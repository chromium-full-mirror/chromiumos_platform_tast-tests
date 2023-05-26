// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/videoconferencing/common"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/zoom"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/videoconferencing/fixture"
	"go.chromium.org/tast-tests/cros/common/action"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type triggerTestParam struct {
	triggerType   common.TrayTriggerType
	incognitoMode bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TrayTriggersChrome,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks VC tray can be triggered on Chrome apps",
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
		SearchFlags: []*testing.StringPair{
			// Test coverage on Chrome apps.
			{
				// Trigger VC tray with Camera on Chrome apps.
				Key:   "feature_id",
				Value: "screenplay-eb95a7e3-db7d-4856-b12f-c72206223f6a",
			},
			{
				// Trigger VC tray with Mic on Chrome apps.
				Key:   "feature_id",
				Value: "screenplay-9bee2da7-d1b3-4c75-ba10-b598d8c93b8e",
			},
			{
				// Trigger VC tray with sharing screen on Chrome apps.
				Key:   "feature_id",
				Value: "screenplay-9423c5ea-5050-4828-96fa-52ba838449e1",
			},
			{
				// Use VC tray to return to a Chrome app.
				Key:   "feature_id",
				Value: "screenplay-1be20f28-70a4-44c0-9124-81ad373b68a9",
			},

			// Test coverage on Lacros apps.
			{
				// Trigger VC tray with Camera on Lacros apps.
				Key:   "feature_id",
				Value: "screenplay-897ef5fb-a9c4-4ae5-85f2-f1f82bc396a0",
			},
			{
				// Trigger VC tray with Mic on Lacros apps.
				Key:   "feature_id",
				Value: "screenplay-cac94449-3699-45a0-adff-98708f2da826",
			},
			{
				// Trigger VC tray with sharing screen on Lacros apps.
				Key:   "feature_id",
				Value: "screenplay-09d4f0df-d171-40d5-9eab-486fa54710ab",
			},
			{
				// Use VC tray to return to a Lacros app.
				Key:   "feature_id",
				Value: "screenplay-4a47131d-4a9f-450d-b256-f6796f0c26ef",
			},

			// Test coverage on Chrome apps in incognito mode.
			{
				// Trigger VC tray with Camera on Chrome Apps in incognito mode.
				Key:   "feature_id",
				Value: "screenplay-54e7cbef-5790-4678-9514-6ef3c0c10b94",
			},
			{
				// Trigger VC tray with Mic on Chrome Apps in incognito mode.
				Key:   "feature_id",
				Value: "screenplay-97909041-2ae0-4e2b-b4bf-219042215bce",
			},
			{
				// Use VC tray to return to a Chrome app in incognito mode.
				Key:   "feature_id",
				Value: "screenplay-9cdbd9c7-34b8-479d-a6d5-5d6b42092bc0",
			},
		},
		Params: []testing.Param{
			{
				Name: "screen",
				Val: triggerTestParam{
					triggerType:   common.ScreenTrigger,
					incognitoMode: false,
				},
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name: "screen_lacros",
				Val: triggerTestParam{
					triggerType:   common.ScreenTrigger,
					incognitoMode: false,
				},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
			// TODO(b/281657351): Support screen sharing test in incognito mode.
			// br.NewTab() does not support incognito mode.
			// So it requires extra effort to support screen sharing in incongito mode.
			{
				Name: "mic",
				Val: triggerTestParam{
					triggerType:   common.MicTrigger,
					incognitoMode: false,
				},
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name: "mic_lacros",
				Val: triggerTestParam{
					triggerType:   common.MicTrigger,
					incognitoMode: false,
				},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
			{
				Name: "mic_incognito",
				Val: triggerTestParam{
					triggerType:   common.MicTrigger,
					incognitoMode: true,
				},
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name: "cam",
				Val: triggerTestParam{
					triggerType:   common.CamTrigger,
					incognitoMode: false,
				},
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
			{
				Name: "cam_lacros",
				Val: triggerTestParam{
					triggerType:   common.CamTrigger,
					incognitoMode: false,
				},
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
			},
			{
				Name: "cam_incognito",
				Val: triggerTestParam{
					triggerType:   common.CamTrigger,
					incognitoMode: true,
				},
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
			},
		},
	})
}

func TrayTriggersChrome(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	testParams := s.Param().(triggerTestParam)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	var br *browser.Browser
	var cleanup action.Action
	var browserConn *chrome.Conn
	if testParams.incognitoMode {
		kb, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to get keyboard: ", err)
		}
		defer kb.Close(ctx)

		br = cr.Browser()

		if err := kb.Accel(ctx, "Ctrl+Shift+N"); err != nil {
			s.Fatal("Failed to launch incognito Chrome browser: ", err)
		}

		browserConn, err = br.NewConnForTarget(ctx, chrome.MatchTargetURL(chrome.NewTabURL))
		if err != nil {
			s.Fatal("Failed to setup incognito Chrome browser: ", err)
		}
		defer browserConn.Close()
		defer browserConn.CloseTarget(cleanupCtx)

		if err := browserConn.Navigate(ctx, "https://accounts.google.com"); err != nil {
			s.Fatal("Failed to negavite to account.google.com: ", err)
		}

		if err := webutil.LoginGoogleAccount(ctx, cr, cr.Creds().User, cr.Creds().Pass); err != nil {
			s.Fatal("Failed to login Google account: ", err)
		}
	} else {
		browserType := s.FixtValue().(fixture.FixtData).BrowserType()

		browserConn, br, cleanup, err = browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
		if err != nil {
			s.Fatal("Failed to launch browser: ", err)
		}
		defer cleanup(cleanupCtx)
		defer browserConn.Close()
		defer browserConn.CloseTarget(cleanupCtx)
	}

	zm, err := zoom.StartNewMeeting(ctx, cr, br, browserConn, zoom.WithDefaultPermissions)
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer zm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_zoom")

	vcTray := vctray.New(ctx, tconn)
	if isShown, err := vcTray.Exists(ctx); err != nil {
		s.Fatal("Failed to check the existence of vcTray: ", err)
	} else if isShown {
		s.Fatal("vcTray is already shown unexpectedly before trigger")
	}

	// Verifies different triggers.
	switch testParams.triggerType {
	case common.CamTrigger:
		verifyCameraTrigger(ctx, s, zm, vcTray, tconn)
	case common.MicTrigger:
		verifyMicTrigger(ctx, s, zm, vcTray)
	case common.ScreenTrigger:
		verifyScreenTrigger(ctx, s, br, zm, vcTray)
	}

	// Verifies closing app hides vcTray.
	if err := uiauto.Combine("close Zoom should hide vcTray",
		zm.Close,
		vcTray.WaitUntilGone,
	)(ctx); err != nil {
		s.Fatal("Failed to close the app to hide vcTray: ", err)
	}
}

func verifyScreenTrigger(ctx context.Context, s *testing.State, br *browser.Browser, zm *zoom.Zoom, vcTray *vctray.VCTray) {
	// Create a new tab for sharing screen.
	const newTabTitle = "New Tab"
	newTabConn, err := br.NewTab(ctx, chrome.NewTabURL, browser.WithBackground())
	if err != nil {
		s.Fatal("Failed to create new tab: ", err)
	}
	defer newTabConn.Close()
	defer newTabConn.CloseTarget(ctx)

	if err := uiauto.Combine("verify sharing screen triggers vcTray",
		// Activation. Screen in use; Camera available; Mic available.
		zm.ShareScreen(newTabTitle),
		vcTray.WaitUntilExists,
		vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceInUse),
		// Deactivation.
		zm.StopShareScreen(),
		vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
	)(ctx); err != nil {
		s.Fatal("Failed to verify sharing screen triggers vcTray: ", err)
	}
}

func verifyCameraTrigger(ctx context.Context, s *testing.State, zm *zoom.Zoom, vcTray *vctray.VCTray, tconn *chrome.TestConn) {
	// Activation. Camera in use; Mic available; Screen hidden.
	if err := uiauto.Combine("verify camera triggers vcTray",
		zm.SwitchVideo(true),
		vcTray.WaitUntilExists,
		vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
	)(ctx); err != nil {
		s.Fatal("Failed to verify camera triggers vcTray: ", err)
	}

	// Verifies return to app via vcTray. It is only validated in camera trigger as it is equivalent to media devices.
	if err := common.VerifyReturnToApp(ctx, tconn); err != nil {
		s.Fatal("Failed to verify return to app via vcTray: ", err)
	}

	// Deactivation
	if err := uiauto.Combine("verify camera deactivation",
		zm.SwitchVideo(false),
		vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
	)(ctx); err != nil {
		s.Fatal("Failed to verify camera deactivation: ", err)
	}
}

func verifyMicTrigger(ctx context.Context, s *testing.State, zm *zoom.Zoom, vcTray *vctray.VCTray) {
	if err := uiauto.Combine("verify microphone triggers vcTray",
		// Activation. Mic in use; Camera available; Screen hidden.
		zm.SetJoinAudio(true),
		vcTray.WaitUntilExists,
		vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
		// Deactivation.
		zm.SetJoinAudio(false),
		vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
	)(ctx); err != nil {
		s.Fatal("Failed to verify microphone triggers vcTray: ", err)
	}
}
