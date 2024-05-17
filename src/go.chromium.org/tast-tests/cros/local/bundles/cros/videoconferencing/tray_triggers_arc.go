// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakearc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayTriggersARC,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray returns to app is functional",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"xiuwen@google.com",
			"charleszhao@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:camera_dependent",
			"group:external-dependency",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome"},
		Data:         []string{fakearc.AppNameArc},
		Fixture:      fixture.LoggedInARCWithInternalCameraAndEffectsDisabled,
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

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create the keyboard: ", err)
	}
	defer kb.Close(ctx)

	fakearc.Install(ctx, s)

	vcTray := vctray.New(ctx, tconn)

	// Verify arc triggers vcTray on camera.
	s.Run(ctx, "cam_only", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_cam_only")

		arcApp := fakearc.Launch(ctx, s, tconn, cr, kb)

		if err := uiauto.Combine("activate camera",
			arcApp.StartVideo,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that arc triggers vcTray by camera: ", err)
		}

		// GoBigSleepLint: wait for 6 seconds; arc only notify os every a few seconds.
		uiauto.Sleep(6 * time.Second)

		if err := uiauto.Combine("deactivate camera",
			arcApp.StopVideo,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that stop using camera resets tray state: ", err)
		}

		if err := uiauto.Combine("close app",
			arcApp.Close,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that close app hides VcTray: ", err)
		}
	})

	// Verify arc triggers vcTray on mic.
	s.Run(ctx, "mic_only", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_mic_only")

		arcApp := fakearc.Launch(ctx, s, tconn, cr, kb)

		if err := uiauto.Combine("activate mic",
			arcApp.StartAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that arc triggers vcTray by mic: ", err)
		}

		// GoBigSleepLint: wait for 6 seconds; arc only notify os every a few seconds.
		uiauto.Sleep(6 * time.Second)

		if err := uiauto.Combine("deactivate mic",
			arcApp.StopAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that stop using camera resets tray state: ", err)
		}

		if err := uiauto.Combine("close app",
			arcApp.Close,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that close app hides VcTray: ", err)
		}
	})

	// Verify tab works on return to app.
	s.Run(ctx, "return_to_app", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_return_to_app")

		arcApp := fakearc.Launch(ctx, s, tconn, cr, kb)

		if err := uiauto.Combine("activate microphone",
			arcApp.StartAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that arc triggers vcTray by camera: ", err)
		}

		if err := vcTray.ReturnToAppForWindow(common.VcAppName, tconn)(ctx); err != nil {
			s.Fatal("Failed to verify return to app: ", err)
		}

		if err := arcApp.Close(cleanupCtx); err != nil {
			s.Fatal("Failed to close tab: ", err)
		}
	})
}
