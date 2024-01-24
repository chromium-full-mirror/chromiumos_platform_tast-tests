// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"io/ioutil"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelKfence,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify kernel kfence errors are logged as expected",
		Contacts:     []string{"cros-telemetry@google.com", "mutexlox@google.com", "swboyd@chromium.org"},
		BugComponent: "b:1032705",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		// TODO(b/201790026): The lkdtm resides on the debugfs,
		// which is not accessible when integrity mode is
		// enabled.
		//
		// We either need to refactor the test not to use lkdtm
		// to trigger crashes or we need to modify the kernel to
		// allow access to the required path. Skip on reven for
		// now, since reven uses integrity mode.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("reven")),
		SoftwareDeps: []string{"kfence_enabled"},
		Params: []testing.Param{{
			Name:              "real_consent",
			ExtraSoftwareDeps: []string{"chrome", "metrics_consent"},
			Val:               crash.RealConsent,
			Fixture:           crash.LoggedInRealConsent,
		}, {
			Name:    "mock_consent",
			Val:     crash.MockConsent,
			Fixture: crash.MockConsentFixture,
		}},
	})
}

func KernelKfence(ctx context.Context, s *testing.State) {
	consentType := s.Param().(crash.ConsentType)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	if err := crash.SetUpCrashTest(ctx, crash.FilterCrashes("kernel_kfence")); err != nil {
		s.Fatal("SetUpCrashTest failed: ", err)
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	if err := crash.RestartAnomalyDetectorWithSendAll(ctx, true); err != nil {
		s.Fatal("Failed to restart anomaly detector: ", err)
	}
	// Restart anomaly detector to clear its --testonly-send-all flag at the end of execution.
	defer crash.RestartAnomalyDetector(cleanupCtx)

	s.Log("Inducing artificial kfence error")
	const lkdtm = "/sys/kernel/debug/provoke-crash/DIRECT"
	if err := ioutil.WriteFile(lkdtm, []byte("KFENCE_READ_AFTER_FREE"), 0); err != nil {
		s.Fatal("Failed to induce kfence error in lkdtm: ", err)
	}

	s.Log("Waiting for files")
	const (
		funcName = `lkdtm[a-zA-Z0-9_]*`
		baseName = `kernel_kfence_` + funcName + `\.\d{8}\.\d{6}\.\d+\.0`
		metaName = baseName + `\.meta`
	)
	expectedRegexes := []string{baseName + `\.kcrash`,
		baseName + `\.log\.gz`,
		metaName}
	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	if consentType == crash.MockConsent {
		// We might not be logged in, so also allow system crash dir.
		crashDirs = append(crashDirs, crash.SystemCrashDir)
	}
	files, err := crash.WaitForCrashFiles(ctx, crashDirs, expectedRegexes)
	defer func() {
		if err := crash.RemoveAllFiles(cleanupCtx, files); err != nil {
			s.Log("Couldn't clean up files: ", err)
		}
	}()
	// For the other consentTypes, expect to find files.
	if err != nil {
		s.Fatal("Couldn't find expected files: ", err)
	}

	if len(files[metaName]) == 1 {
		metaFile := files[metaName][0]
		metaFileParametersToTest := map[string]string{
			"in_progress_integration_test": "crash.KernelKfence",
			"client_computed_severity":     "ERROR",
			"client_computed_product":      "Platform",
		}
		if contents, err := ioutil.ReadFile(metaFile); err != nil {
			s.Errorf("Couldn't read meta file %s contents: %v", metaFile, err)
		} else {
			missing := false
			for key, value := range metaFileParametersToTest {
				if !strings.Contains(string(contents), ("upload_var_" + key + "=" + value)) {
					s.Error(".meta file did not contain expected " + key)
					missing = true
				}
			}
			if missing {
				if err := crash.MoveFilesToOut(ctx, s.OutDir(), metaFile); err != nil {
					s.Error("Failed to save the meta file: ", err)
				}
			}
		}
	} else {
		s.Errorf("Unexpectedly found multiple meta files: %q", files[metaName])
		if err := crash.MoveFilesToOut(ctx, s.OutDir(), files[metaName]...); err != nil {
			s.Error("Failed to save additional meta files: ", err)
		}
	}
}
