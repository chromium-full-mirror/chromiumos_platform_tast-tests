// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/videoconferencing/common"
	"chromiumos/tast/local/camera/arcapp"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/videoconferencing/fixture"

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
			"video_conference_per_build",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Data:         []string{"ArcCameraTest.apk"},
		Fixture:      fixture.GAIALoggedInARCWithInternalCameraAndEffectsEnabled,
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
	defer cleanupFunc(cleanupCtx, tconn)

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
			return arcapp.StopRecording(ctx, cr, a)
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
