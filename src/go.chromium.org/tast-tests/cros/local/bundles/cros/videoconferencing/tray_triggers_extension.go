// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakeextension"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayTriggersExtension,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray can be triggered by Chrome extension",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"xiuwen@google.com",
			"charleszhao@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:mainline", "group:cbx", "cbx_feature_enabled", "cbx_unstable",
			"informational",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: fixture.LoggedInWithFakeVCExtension,
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.LoggedInLacrosWithFakeVCExtension,
			},
		},
		SearchFlags: []*testing.StringPair{
			{
				// Trigger VC tray with Camera on Chrome Extension.
				Key:   "feature_id",
				Value: "screenplay-a2a23547-4fab-4b65-a21a-a88daecd16aa",
			},
			{
				// Trigger VC tray with Mic on Chrome Extension.
				Key:   "feature_id",
				Value: "screenplay-2864041b-956e-4583-901c-96881f63ab71",
			},
			{
				// Trigger VC tray with sharing screen on Chrome Extension.
				Key:   "feature_id",
				Value: "screenplay-e2ae14f5-3dec-44de-aed4-b2975c62f17f",
			},
			{
				// Use VC tray to return to an Chrome extension app.
				Key:   "feature_id",
				Value: "screenplay-82edd651-0483-49c3-b6bb-58af7acb5a75",
			},
			{
				// Trigger VC tray with Camera on Lacros Extension.
				Key:   "feature_id",
				Value: "screenplay-dc04650a-6dc8-42cf-a75c-f107a80b231c",
			},
			{
				// Trigger VC tray with Mic on Lacros Extension.
				Key:   "feature_id",
				Value: "screenplay-8547de4e-4932-4ac7-9570-d0e73d0b1118",
			},
			{
				// Trigger VC tray with sharing screen on Lacros Extension.
				Key:   "feature_id",
				Value: "screenplay-201cb73e-9071-44e0-a251-188c3937a841",
			},
			{
				// Use VC tray to return to an Lacros extension app.
				Key:   "feature_id",
				Value: "screenplay-09f693df-f573-4880-a4cf-707b6f78aa03",
			},
		},
	})
}

// TrayTriggersExtension checks VC tray can be triggered by Chrome extension.
func TrayTriggersExtension(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, _, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	vcTray := vctray.New(ctx, tconn)

	if err := fakeextension.GrantAVPermissions(ctx, br); err != nil {
		s.Fatal("Failed to grant AV permissions: ", err)
	}
	extUI, err := fakeextension.Launch(ctx, tconn, br)
	if err != nil {
		s.Fatal("Failed to launch extension: ", err)
	}

	// Verify extension triggers vcTray on camera.
	s.Run(ctx, "cam_only", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_cam_only")

		if err := uiauto.Combine("activate camera",
			extUI.StartVideo,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceHidden),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that extension triggers vcTray by camera: ", err)
		}

		if err := uiauto.Combine("deactivate camera",
			extUI.StopVideo,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that vcTray is gone after deactivating camera: ", err)
		}
	})

	// Verify extension triggers vcTray on mic.
	s.Run(ctx, "mic_only", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_mic_only")

		if err := uiauto.Combine("activate microphone",
			extUI.StartAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceHidden),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify activting microphone: ", err)
		}

		// Toggle off microphone will hide vcTray as no other media devices are used.
		if err := uiauto.Combine("deactivate microphone",
			extUI.StopAudio,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify deactivating microphone: ", err)
		}
	})

	// Verify extension triggers vcTray on sharing screen.
	s.Run(ctx, "screen_only", func(ctx context.Context, s *testing.State) {
		defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_screen_only")

		if err := uiauto.Combine("share screen only",
			extUI.StartScreenCapture,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceHidden),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceHidden),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceInUse),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that extension triggers vcTray by sharing screen: ", err)
		}

		if err := uiauto.Combine("stop screen share",
			extUI.StopScreenCapture,
			vcTray.WaitUntilGone,
		)(ctx); err != nil {
			s.Fatal("Failed to verify that vcTray is gone after stopping sharing screen: ", err)
		}
	})
}
