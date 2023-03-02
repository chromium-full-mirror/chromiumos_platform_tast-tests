// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/apps/googlemeet"
	"chromiumos/tast/local/bundles/cros/videoconferencing/commontype"
	"chromiumos/tast/local/bundles/cros/videoconferencing/fixture"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayReturnToApp,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray returns to app is functional",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		Attr: []string{
			"group:mainline", "informational", "group:ml_service",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("betty")),
		Params: []testing.Param{
			{
				Name:    "clamshell",
				Fixture: fixture.GAIALoggedInClamshellWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "tablet",
				Fixture: fixture.GAIALoggedInTabletWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "clamshell_lacros",
				Fixture: fixture.GAIALoggedInClamshellWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "tablet_lacros",
				Fixture: fixture.GAIALoggedInTabletWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
		},
	})
}

func TrayReturnToApp(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	br, cleanup, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	gm, err := googlemeet.StartNewMeeting(ctx, cr, br, nil)
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_with_meet")

	if err := uiauto.Combine("configure Meet",
		gm.MuteIfMicAvailable,
		gm.SwitchVideo(true),
	)(ctx); err != nil {
		s.Fatal("Failed to configure Meet: ", err)
	}

	// Assume the meeting window is currently active.
	meetWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
	}

	// Save current meet window state and use it for return verification.
	meetWindowState := meetWindow.State

	if err := ash.SetWindowStateAndWait(ctx, tconn, meetWindow.ID, ash.WindowStateMinimized); err != nil {
		s.Fatal("Failed to minimize meet window: ", err)
	}

	vcTray := vctray.New(ctx, tconn)

	if err := uiauto.Combine("configure effects via mcpanel",
		vcTray.ExpandPanel,
		// TODO(b/267709155): Use window title once vc tray is finalized.
		vcTray.ReturnToApp("meet.google.com"),
		vcTray.CollapsePanel,
	)(ctx); err != nil {
		s.Fatal("Failed to configure effects: ", err)
	}

	newActiveWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
	}

	if newActiveWindow.ID != meetWindow.ID || newActiveWindow.State != meetWindowState {
		s.Fatalf("Failed to restore Meet window(expected: %v, actual: %v)", meetWindow, newActiveWindow)
	}
}
