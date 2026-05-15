// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/arccrash"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast/core/testing"
)

func init() {
	// Disabled by TORA. See: b/349914087
	// testing.AddTest(&testing.Test{
	// 	Func: AppCrash,
	// 	// Disabled by TORA. See: b/349914087
	// 	LifeCycleStage: testing.LifeCycleOwnerMonitored,
	// 	Desc:           "Test handling of a local app crash",
	// 	Contacts: []string{
	// 		// ARC
	// 		"arc-core@google.com",
	// 		"jhorwich@google.com",
	// 		// Data team
	// 		"troywang@google.com",
	// 		"chromeos-data-eng@google.com",
	// 	},
	// 	BugComponent: "b:153255",
	// 	Attr:         []string{"group:mainline", "informational"},
	// 	SoftwareDeps: []string{"chrome"},
	// 	Fixture:      "arcBooted",
	// 	Params: []testing.Param{
	// 		{
	// 			Name:              "vm_mock_consent",
	// 			ExtraAttr:         []string{"group:hw_agnostic"},
	// 			ExtraSoftwareDeps: []string{"android_vm"},
	// 			Val:               crash.MockConsent,
	// 		},
	// 		{
	// 			Name:              "vm_real_consent",
	// 			ExtraAttr:         []string{"group:hw_agnostic"},
	// 			ExtraSoftwareDeps: []string{"android_vm", "metrics_consent"},
	// 			Val:               crash.RealConsent,
	// 		},
	// 	},
	// })
}

func AppCrash(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome

	opt := crash.WithMockConsent()
	useConsent := s.Param().(crash.ConsentType)
	if useConsent == crash.RealConsent {
		opt = crash.WithConsent(cr)
	}

	if err := crash.SetUpCrashTest(ctx, opt); err != nil {
		s.Fatal("Couldn't set up crash test: ", err)
	}
	defer crash.TearDownCrashTest(ctx)

	s.Log("Starting app")
	const exampleApp = "com.android.settings"
	if err := a.Command(ctx, "am", "start", "-W", exampleApp).Run(); err != nil {
		s.Fatal("Failed to run an app to be crashed: ", err)
	}

	s.Log("Making crash")
	if err := a.Command(ctx, "am", "crash", exampleApp).Run(); err != nil {
		s.Fatal("Failed to crash: ", err)
	}

	s.Log("Waiting for crash files to become present")
	// Wait files like com_android_settings_foo_bar.20200420.204845.12345.664107.log in the daemon-store directory
	base := strings.ReplaceAll(exampleApp, ".", "_") + `(?:_[[:alnum:]]+)*.\d{8}\.\d{6}\.\d+\.\d+`
	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	metaFileName := base + crash.MetadataExt
	files, err := crash.WaitForCrashFiles(ctx, crashDirs, []string{
		base + crash.LogExt, metaFileName, base + crash.InfoExt,
	})
	if err != nil {
		s.Fatal("Didn't find files: ", err)
	}
	defer crash.RemoveAllFiles(ctx, files)

	metaFiles := files[metaFileName]
	if len(metaFiles) > 1 {
		s.Errorf("Unexpectedly saw %d crashes of appcrash. Saving for debugging", len(metaFiles))
		if err := crash.MoveFilesToOut(ctx, s.OutDir(), metaFiles...); err != nil {
			s.Error("Failed to save meta file: ", err)
		}
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
		if err := crash.MoveFilesToOut(ctx, s.OutDir(), metaFile); err != nil {
			s.Error("Failed to save meta file: ", err)
		}
	}
	isValidSeverity, err := arccrash.ValidateComputedSeverity(ctx, metaFile)
	if err != nil {
		s.Fatal("Failed to validate meta file severity: ", err)
	}
	if !isValidSeverity {
		s.Error("validateComputedSeverity failed. Saving meta file")
		if err := crash.MoveFilesToOut(ctx, s.OutDir(), metaFile); err != nil {
			s.Error("Failed to save meta file: ", err)
		}
	}
}
