// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/videoconferencing/common"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/screencastify"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayTriggersExtension,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray can be triggered by Chrome extension",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:video_conference", "video_conference_per_build", "group:external-dependency",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Params: []testing.Param{
			{
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
		},
	})
}

func TrayTriggersExtension(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	br, cleanup, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	if screencastify.InstallExtension(ctx, tconn, br); err != nil {
		s.Fatal("Failed to install Screencastify: ", err)
	}
	defer screencastify.UninstallExtension(cleanupCtx, tconn, br)

	if err := screencastify.Login(ctx, cr, tconn, br); err != nil {
		s.Fatal("Failed to login Screencastify: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	// By default, only audio is activated by launching extension popup.
	s.Run(ctx, "mic_only", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_mic_only")
		sc, err := screencastify.Launch(ctx, cr, tconn)
		if err != nil {
			s.Fatal("Failed to launch Screencastify extension: ", err)
		}

		if err := uiauto.Combine("verify default status of media device usage",
			vcTray.WaitUntilExists,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceHidden),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify the default status of media device usage: ", err)
		}

		// Toggle off microphone will hide vcTray as no other media devices are used.
		if err := uiauto.Combine("toggle off microphone to hide vcTray",
			sc.ToggleMicrophone(false),
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that extension triggers vcTray by microphone: ", err)
		}
	})

	// Verify extension triggers vcTray on camera.
	s.Run(ctx, "cam_only", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_cam_only")
		sc, err := screencastify.Launch(ctx, cr, tconn)
		if err != nil {
			s.Fatal("Failed to launch Screencastify extension: ", err)
		}

		if err := uiauto.Combine("record webcam only",
			sc.SetRecordType(screencastify.RecordTypeCameraOnly),
			// Mic is automatically enabled when camera is selected.
			sc.ToggleMicrophone(false),
			sc.StartRecording,
			sc.WaitUntilCameraRecordingStarted,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceHidden),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that extension triggers vcTray by camera: ", err)
		}

		// Return to app only works on camera record.
		// Verify returnToApp via vcTray.
		if err := common.VerifyReturnToApp(ctx, tconn); err != nil {
			s.Fatal("Failed to verify returnToApp: ", err)
		}

		if err := uiauto.Combine("stop camera record",
			sc.StopRecordingOnPreviewPage,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that vcTray is gone after stopping sharing screen: ", err)
		}
	})

	// Verify extension triggers vcTray on sharing screen.
	s.Run(ctx, "screen_only", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_screen_only")
		sc, err := screencastify.Launch(ctx, cr, tconn)
		if err != nil {
			s.Fatal("Failed to launch Screencastify extension: ", err)
		}

		if err := uiauto.Combine("share screen only",
			sc.ShareEntireScreen(false, false),
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceHidden),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceHidden),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceInUse),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that extension triggers vcTray by sharing screen: ", err)
		}

		// Share screen can only be stopped from extension popup.
		sc, err = screencastify.Launch(ctx, cr, tconn)
		if err != nil {
			s.Fatal("Failed to launch Screencastify extension: ", err)
		}

		if err := uiauto.Combine("stop screen share",
			sc.StopRecordingOnPopupWindow,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that vcTray is gone after stopping sharing screen: ", err)
		}
	})
}
