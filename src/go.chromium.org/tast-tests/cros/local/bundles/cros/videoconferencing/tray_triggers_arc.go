// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/camera/arcapp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayTriggersARC,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray returns to app is functional",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:camera_dependent",
			"group:external-dependency",
			"group:video_conference",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(false)},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Data:         []string{"ArcCameraTest.apk"},
		Fixture:      fixture.GAIALoggedInARCWithInternalCameraAndEffectsEnabled,
		SearchFlags: []*testing.StringPair{
			{
				// Trigger VC tray with Camera on ARC++ apps.
				Key:   "feature_id",
				Value: "screenplay-766f6f86-dfff-4725-89e7-a45c24592135",
			},
			{
				// Trigger VC tray with Mic on ARC++ apps.
				Key:   "feature_id",
				Value: "screenplay-4a6a19bd-9555-43f9-ba9f-66adae46e406",
			},
			{
				// Use VC tray to return to an ARC++ app.
				Key:   "feature_id",
				Value: "screenplay-eee4ab4b-b263-4c9c-8499-0266ae241a41",
			},
		},
	})
}

func TrayTriggersARC(ctx context.Context, s *testing.State) {

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(fixture.FixtData).ARC()

	if err := a.Install(ctx, s.DataPath(arcapp.CameraAppApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	cleanupFunc, err := arcapp.LaunchARCCameraApp(ctx, a, tconn)
	if err != nil {
		s.Fatal("Failed to launch ARC camera app: ", err)
	}
	defer func() {
		if s.HasError() {
			cleanupFunc(cleanupCtx, tconn)
		}
	}()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	vcTray := vctray.New(ctx, tconn)
	if err := uiauto.Combine("verify camera triggers vcTray",
		vcTray.WaitUntilExists,
		vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
	)(ctx); err != nil {
		s.Fatal("Failed to verify camera triggers vcTray: ", err)
	}

	if err := common.VerifyReturnToApp(ctx, tconn); err != nil {
		s.Fatal("Failed to verify returnToApp: ", err)
	}

	if err := uiauto.Combine("verify audio activation",
		func(ctx context.Context) error {
			return arcapp.StartRecording(ctx, cr, a)
		},
		vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
		vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
	)(ctx); err != nil {
		s.Fatal("Failed to verify audio activation: ", err)
	}

	if err := uiauto.Combine("verify audio deactivation",
		// GoBigSleepLint: Record the video for 3 seconds before stop. Otherwise it fails with
		// `could not send intent: broadcast of "chromeos.camera.app.arccameratest.ACTION_STOP_RECORDING" failed, status = 0, data = "8"`
		uiauto.Sleep(3*time.Second),
		func(ctx context.Context) error {
			return arcapp.StopRecordingAndCheckFile(ctx, cr, a, false)
		},
		vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
		vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
	)(ctx); err != nil {
		s.Fatal("Failed to verify audio deactivation: ", err)
	}

	// Close app should hide vcTray.
	if err := uiauto.Combine("verify closing app hides vcTray",
		func(ctx context.Context) error {
			cleanupFunc(cleanupCtx, tconn)
			return nil
		},
		vcTray.WaitUntilGone,
	)(ctx); err != nil {
		s.Fatal("Failed to verify audio deactivation: ", err)
	}
}
