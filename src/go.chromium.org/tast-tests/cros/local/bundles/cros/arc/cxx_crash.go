// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"io/ioutil"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/arccrash"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CxxCrash,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test handling of a C++ binary crash",
		Contacts:     []string{"arc-core@google.com", "matvore@chromium.org"},
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		// ChromeOS > Software > ARC++ > Core
		BugComponent: "b:488493",
		SoftwareDeps: []string{"arc_android_data_cros_access", "chrome"},
		Fixture:      "arcBooted",
		Params: []testing.Param{{
			Name:              "real_consent",
			ExtraSoftwareDeps: []string{"android_container", "metrics_consent"},
			Val:               crash.RealConsent,
		}, {
			Name:              "mock_consent",
			ExtraSoftwareDeps: []string{"android_container"},
			Val:               crash.MockConsent,
		}, {
			Name:              "real_consent_vm",
			ExtraSoftwareDeps: []string{"android_vm", "metrics_consent"},
			Val:               crash.RealConsent,
		}, {
			Name:              "mock_consent_vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val:               crash.MockConsent,
		}},
	})
}

func CxxCrash(ctx context.Context, s *testing.State) {
	const (
		temporaryCrashDirInAndroid = "/data/vendor/arc_native_crash_reports"
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome

	opt := crash.WithMockConsent()
	if s.Param().(crash.ConsentType) == crash.RealConsent {
		opt = crash.WithConsent(cr)
	}

	if err := crash.SetUpCrashTest(ctx, opt); err != nil {
		s.Fatal("Failed to set up crash test: ", err)
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	s.Log("Making crash")
	cmd := a.Command(ctx, "/system/bin/sh", "-c", "kill -SEGV $$")
	if err := cmd.Run(); err != nil {
		// The shell returns 139 (= 128 + 11) when it's terminated by SIGSEGV (= 11).
		if cmd.ProcessState.ExitCode() != 139 {
			s.Fatal("Failed to crash: ", err)
		}
	} else {
		s.Fatal("Failed to crash: the process has successfully finished without crashing")
	}

	s.Log("Waiting for crash files to become present")
	// Wait files like sh.20200420.204845.12345.664107.dmp in the daemon-store directory.
	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	const stem = `sh\.\d{8}\.\d{6}\.\d+\.\d+`
	metaFileName := stem + crash.MetadataExt
	files, err := crash.WaitForCrashFiles(ctx, crashDirs, []string{
		stem + crash.MinidumpExt, metaFileName,
	})
	if err != nil {
		s.Fatal("Failed to find files: ", err)
	}
	defer crash.RemoveAllFiles(cleanupCtx, files)

	metaFiles := files[metaFileName]
	if len(metaFiles) > 1 {
		s.Errorf("Unexpectedly saw %d crashes. Saving for debugging", len(metaFiles))
		crash.MoveFilesToOut(ctx, s.OutDir(), metaFiles...)
	}
	// WaitForCrashFiles guarantees that there will be a match for all regexes if it succeeds,
	// so this must exist.
	metaFile := metaFiles[0]

	s.Log("Validating the meta file")
	bp, err := arccrash.GetBuildProp(ctx, a)
	if err != nil {
		if err := arccrash.UploadSystemBuildProp(ctx, a, s.OutDir()); err != nil {
			s.Error("Failed to get build.prop: ", err)
		}
		s.Fatal("Failed to get BuildProperty: ", err)
	}
	isValid, err := arccrash.ValidateBuildProp(ctx, metaFile, bp)
	if err != nil {
		s.Fatal("Failed to validate meta file: ", err)
	}
	if !isValid {
		s.Error("validateBuildProp failed. Saving meta file")
		crash.MoveFilesToOut(ctx, s.OutDir(), metaFile)
	}

	// On ARC++ container, the Linux kernel is shared with ARC and ChromeOS. The kernel can
	// directly receive ARC's C++ binary crashes and no temporary files are created. On ARCVM,
	// some temporary files are created as part of the crash handling and we want to clean it
	// up.
	vmEnabled, err := arc.VMEnabled()
	if err != nil {
		s.Fatal("Failed to check whether ARCVM is enabled: ", err)
	}
	if vmEnabled {
		s.Log("Getting the dir path for temporary dump files")
		androidDataDir, err := arc.AndroidDataDir(ctx, cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to get android-data dir: ", err)
		}
		temporaryCrashDir := filepath.Join(androidDataDir, temporaryCrashDirInAndroid)

		s.Log("Checking that temporary dump files are deleted")
		// The time to wait for removal of temporary files. Typically they are removed in a few seconds.
		const pollingTimeout = 10 * time.Second
		if err := arc.PollWithReadOnlyAndroidData(ctx, cr.NormalizedUser(), func(c context.Context) error {
			files, err := ioutil.ReadDir(temporaryCrashDir)
			if err != nil {
				return arc.PollBreakIfNotEUCLEANOnVirtioBlkData(ctx, err)
			}

			if len(files) != 0 {
				var filePaths []string
				for _, fi := range files {
					filePaths = append(filePaths, filepath.Join(temporaryCrashDir, fi.Name()))
				}
				return errors.Errorf("temporary files found: %s", strings.Join(filePaths, ", "))
			}
			return nil
		}, &testing.PollOptions{Timeout: pollingTimeout}); err != nil {
			s.Fatal("Temporary files are not deleted: ", err)
		}
	}
}
