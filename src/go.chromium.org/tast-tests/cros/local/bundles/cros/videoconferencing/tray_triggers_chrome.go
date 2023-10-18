// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/zoom"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"
	"go.chromium.org/tast/core/errors"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type triggerTestParam struct {
	triggerType   common.TrayTriggerType
	incognitoMode bool
}

const (
	errMessageMeetingHasEnded    = "This meeting has ended as someone has started a new meeting with this account"
	errMessageSomethingWentWrong = "Something went wrong"
)

var (
	errMeetingHasEnded    = errors.New(errMessageMeetingHasEnded)
	errSomethingWentWrong = errors.New(errMessageSomethingWentWrong)
)

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
		Timeout:      6 * time.Minute,
		Attr: []string{
			"group:video_conference", "video_conference_per_build", "group:external-dependency",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
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
	testParams := s.Param().(triggerTestParam)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	// If there are multiple DUTs that open zoom meeting at the same time with
	// the same account, it is possible that a dialog "This meeting has ended
	// as someone has started a new meeting with this account" or "Something
	// went wrong" will popup. Do retry if it encounters those issues.
	const retryCount = 2
	skipRetry := func(err error) bool {
		return !(errors.Is(err, errMeetingHasEnded) || errors.Is(err, errSomethingWentWrong))
	}
	for i := 1; i <= retryCount; i++ {
		if err := run(ctx, s, cr, testParams); err == nil {
			break
		} else if i == retryCount || skipRetry(err) {
			s.Fatal("Failed to run Zoom: ", err)
		}
		s.Logf("Attempt #%d to run Zoom", i)
	}
}

func run(ctx context.Context, s *testing.State, cr *chrome.Chrome, testParams triggerTestParam) (retErr error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to connect Test API")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	var br *browser.Browser
	var closeBrowser action.Action
	var browserConn *chrome.Conn
	if testParams.incognitoMode {
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get keyboard")
		}
		defer kb.Close(ctx)

		br = cr.Browser()

		if err := kb.Accel(ctx, "Ctrl+Shift+N"); err != nil {
			return errors.Wrap(err, "failed to launch incognito Chrome browser")
		}

		browserConn, err = br.NewConnForTarget(ctx, chrome.MatchTargetURL(chrome.NewTabURL))
		if err != nil {
			return errors.Wrap(err, "failed to setup incognito Chrome browser")
		}
		defer browserConn.Close()
		defer browserConn.CloseTarget(cleanupCtx)

		if err := browserConn.Navigate(ctx, "https://accounts.google.com"); err != nil {
			return errors.Wrap(err, "failed to negavite to account.google.com")
		}

		if err := webutil.LoginGoogleAccount(ctx, cr, cr.Creds().User, cr.Creds().Pass); err != nil {
			return errors.Wrap(err, "failed to login Google account")
		}
	} else {
		browserType := s.FixtValue().(fixture.FixtData).BrowserType()

		browserConn, br, closeBrowser, err = browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
		if err != nil {
			return errors.Wrap(err, "failed to launch browser")
		}
		defer closeBrowser(cleanupCtx)
		defer browserConn.Close()
		defer browserConn.CloseTarget(cleanupCtx)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), func() bool { return retErr != nil }, cr, "ui_zoom_start")

	zm, err := zoom.StartNewMeeting(ctx, cr, br, browserConn, zoom.WithDefaultPermissions)
	if err != nil {
		return errors.Wrap(err, "failed to start meeting")
	}
	defer zm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), func() bool { return retErr != nil }, cr, "ui_zoom")

	vcTray := vctray.New(ctx, tconn)
	if isShown, err := vcTray.Exists(ctx); err != nil {
		return errors.Wrap(err, "failed to check the existence of vcTray")
	} else if isShown {
		return errors.New("vcTray is already shown unexpectedly before trigger")
	}

	// Verifies different triggers.
	switch testParams.triggerType {
	case common.CamTrigger:
		err = verifyCameraTrigger(ctx, s, zm, vcTray, tconn)
	case common.MicTrigger:
		err = verifyMicTrigger(ctx, s, zm, vcTray)
	case common.ScreenTrigger:
		err = verifyScreenTrigger(ctx, s, br, zm, vcTray)
	}
	if err != nil {
		accountErrorText := nodewith.Name(errMessageMeetingHasEnded).Role(role.StaticText)
		if uiauto.New(tconn).Exists(accountErrorText)(ctx) == nil {
			s.Logf("The alert dialog %q pops up", errMessageMeetingHasEnded)
			return errMeetingHasEnded
		}
		// "Something went wrong" dialog can't be detected by ui dump, use udetection instead.
		ud := uidetection.NewDefault(tconn).WithScreenshotStrategy(uidetection.ImmediateScreenshot)
		serverErrorSentence := uidetection.TextBlockFromSentence(errMessageSomethingWentWrong)
		if ud.Exists(serverErrorSentence)(ctx) == nil {
			s.Logf("The alert dialog %q pops up", errMessageSomethingWentWrong)
			return errSomethingWentWrong
		}
		return err
	}
	// Verifies closing app hides vcTray.
	return uiauto.Combine("close Zoom should hide vcTray",
		zm.Close,
		vcTray.WaitUntilGone,
	)(ctx)
}

func verifyScreenTrigger(ctx context.Context, s *testing.State, br *browser.Browser, zm *zoom.Zoom, vcTray *vctray.VCTray) error {
	// Create a new tab for sharing screen.
	const newTabTitle = "New Tab"
	newTabConn, err := br.NewTab(ctx, chrome.NewTabURL, browser.WithBackground())
	if err != nil {
		return errors.Wrap(err, "failed to create new tab")
	}
	defer newTabConn.Close()
	defer newTabConn.CloseTarget(ctx)

	return uiauto.Combine("verify sharing screen triggers vcTray",
		// Activation. Screen in use; Camera available; Mic available.
		zm.ShareScreen(newTabTitle),
		vcTray.WaitUntilExists,
		vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceInUse),
		// Deactivation.
		zm.StopShareScreen(),
		vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
	)(ctx)
}

func verifyCameraTrigger(ctx context.Context, s *testing.State, zm *zoom.Zoom, vcTray *vctray.VCTray, tconn *chrome.TestConn) error {
	// Activation. Camera in use; Mic available; Screen hidden.
	if err := uiauto.Combine("verify camera triggers vcTray",
		zm.SwitchVideo(true),
		vcTray.WaitUntilExists,
		vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceInUse),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify camera triggers vcTray")
	}

	// Verifies return to app via vcTray. It is only validated in camera trigger as it is equivalent to media devices.
	if err := common.VerifyReturnToApp(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to verify return to app via vcTray")
	}

	// Deactivation
	return uiauto.Combine("verify camera deactivation",
		zm.SwitchVideo(false),
		vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
	)(ctx)
}

func verifyMicTrigger(ctx context.Context, s *testing.State, zm *zoom.Zoom, vcTray *vctray.VCTray) error {
	return uiauto.Combine("verify microphone triggers vcTray",
		// Activation. Mic in use; Camera available; Screen hidden.
		zm.SetJoinAudio(true),
		vcTray.WaitUntilExists,
		vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
		// Deactivation.
		zm.SetJoinAudio(false),
		vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceAvailable),
	)(ctx)
}
