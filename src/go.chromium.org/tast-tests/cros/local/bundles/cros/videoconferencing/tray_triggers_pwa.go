// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakepwa"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayTriggersPwa,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray can be triggered by Chrome tab",
		Contacts: []string{
			"cros-video-conference-tast-tests@google.com",
			"charleszhao@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			data.VcAppHTML,
			data.VcAppJs,
			data.VcAppIcon,
			data.VcAppManifest,
		},

		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: fixture.LoggedInWithFakeHALAndEffectsDisabled,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LoggedInLacrosWithFakeHALAndEffectsDisabled,
			},
		},
		SearchFlags: []*testing.StringPair{
			// Test coverage on Chrome tabs.
			{
				// Trigger VC tray with Camera on Chrome tabs.
				Key:   "feature_id",
				Value: "screenplay-eb95a7e3-db7d-4856-b12f-c72206223f6a",
			},
			{
				// Trigger VC tray with Mic on Chrome tabs.
				Key:   "feature_id",
				Value: "screenplay-9bee2da7-d1b3-4c75-ba10-b598d8c93b8e",
			},
			{
				// Trigger VC tray with sharing screen on Chrome tabs.
				Key:   "feature_id",
				Value: "screenplay-9423c5ea-5050-4828-96fa-52ba838449e1",
			},
			{
				// Use VC tray to return to a Chrome tab.
				Key:   "feature_id",
				Value: "screenplay-1be20f28-70a4-44c0-9124-81ad373b68a9",
			},

			// Test coverage on Lacros tabs.
			{
				// Trigger VC tray with Camera on Lacros tabs.
				Key:   "feature_id",
				Value: "screenplay-897ef5fb-a9c4-4ae5-85f2-f1f82bc396a0",
			},
			{
				// Trigger VC tray with Mic on Lacros tabs.
				Key:   "feature_id",
				Value: "screenplay-cac94449-3699-45a0-adff-98708f2da826",
			},
			{
				// Trigger VC tray with sharing screen on Lacros tabs.
				Key:   "feature_id",
				Value: "screenplay-09d4f0df-d171-40d5-9eab-486fa54710ab",
			},
			{
				// Use VC tray to return to a Lacros tab.
				Key:   "feature_id",
				Value: "screenplay-4a47131d-4a9f-450d-b256-f6796f0c26ef",
			},
		},
	})
}

// TrayTriggersPwa checks VC tray can be triggered by Chrome tabs.
func TrayTriggersPwa(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	vcTray := vctray.New(ctx, tconn)

	// Install PWA.
	appID, err := fakepwa.InstallPwa(ctx, br, tconn, srvURL+data.VcAppHTML)
	if err != nil {
		s.Fatal("fail to install pwa: ", err)
	}

	// Verify pwa triggers vcTray on camera.
	s.Run(ctx, "cam_only", func(ctx context.Context, s *testing.State) {
		pwaUI, err := fakepwa.LaunchApp(ctx, tconn, br, appID)
		if err != nil {
			s.Fatal("Failed to open pwa: ", err)
		}
		defer pwaUI.Close(ctx)

		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_cam_only")

		if err := uiauto.Combine("activate camera",
			pwaUI.StartVideo,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that tab triggers vcTray by camera: ", err)
		}

		if err := uiauto.Combine("deactivate camera",
			pwaUI.StopVideo,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that stop using camera resets tray state: ", err)
		}

		if err := uiauto.Combine("close tab",
			pwaUI.Close,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that close tab hides VcTray: ", err)
		}
	})

	// Verify pwa triggers vcTray on mic.
	s.Run(ctx, "mic_only", func(ctx context.Context, s *testing.State) {
		pwaUI, err := fakepwa.LaunchApp(ctx, tconn, br, appID)
		if err != nil {
			s.Fatal("Failed to open tab: ", err)
		}
		defer pwaUI.Close(ctx)

		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_mic_only")

		if err := uiauto.Combine("activate mic",
			pwaUI.StartAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that tab triggers vcTray by mic: ", err)
		}

		if err := uiauto.Combine("deactivate mic",
			pwaUI.StopAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that stop using camera resets tray state: ", err)
		}

		if err := uiauto.Combine("close tab",
			pwaUI.Close,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that close tab hides VcTray: ", err)
		}
	})

	// Verify pwa triggers vcTray on screen-share.
	s.Run(ctx, "screen_only", func(ctx context.Context, s *testing.State) {
		pwaUI, err := fakepwa.LaunchApp(ctx, tconn, br, appID)
		if err != nil {
			s.Fatal("Failed to open tab: ", err)
		}
		defer pwaUI.Close(ctx)

		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_screen_only")

		if err := uiauto.Combine("share screen only",
			pwaUI.StartScreenCapture,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceInUse),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that extension triggers vcTray by sharing screen: ", err)
		}

		if err := uiauto.Combine("stop screen share",
			pwaUI.StopScreenCapture,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that stop screen share resets tray state: ", err)
		}

		if err := uiauto.Combine("close tab",
			pwaUI.Close,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that close tab hides VcTray: ", err)
		}
	})

	// Verify pwa works on return to app.
	s.Run(ctx, "return_to_app", func(ctx context.Context, s *testing.State) {
		pwaUI, err := fakepwa.LaunchApp(ctx, tconn, br, appID)
		if err != nil {
			s.Fatal("Failed to open tab: ", err)
		}
		defer pwaUI.Close(ctx)

		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_return_to_app")

		if err := uiauto.Combine("activate camera",
			pwaUI.StartVideo,
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that tab triggers vcTray by camera: ", err)
		}

		if err := vcTray.ReturnToAppForWindow(common.VcAppName, tconn)(ctx); err != nil {
			s.Fatal("Failed to verify return to app: ", err)
		}
	})
}
