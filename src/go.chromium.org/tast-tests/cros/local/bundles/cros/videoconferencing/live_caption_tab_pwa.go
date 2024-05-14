// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakepwa"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakevctab"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	liveCaptionBubble  = nodewith.ClassName("CaptionBubbleFrameView")
	liveCaptionContent = nodewith.Name("Hello").Role(role.StaticText)
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LiveCaptionTabPwa,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks on-device live caption works in tabs and pwas",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:external-dependency",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(true)},
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			"vc_tester/voice_en_hello.wav",
			"vc_tester/popup.html",
			"vc_tester/popup.js",
			"vc_tester/camera.png",
			"vc_tester/manifest.json",
		},
		SearchFlags: []*testing.StringPair{
			{
				// Live captions display on tabs and pwas.
				Key:   "feature_id",
				Value: "screenplay-6bddb622-b203-4c4f-9dec-47df1a280f21",
			},
			{
				// Remove Live captions on tabs and pwas.
				Key:   "feature_id",
				Value: "screenplay-25b74f07-6422-4f4d-90e9-193ac21063d2",
			},
			{
				// Retain Live captions on tabs and pwas.
				Key:   "feature_id",
				Value: "screenplay-447a9654-a25f-45b6-9b3d-bf95b9864d64",
			},
		},
		Params: []testing.Param{
			{
				Name:    "pwa",
				Fixture: fixture.LoggedInWithFakeHALAndEffectsDisabled,
				Val:     common.LaunchAppInPWA,
			},
			{
				Name:    "web",
				Fixture: fixture.LoggedInWithFakeHALAndEffectsDisabled,
				Val:     common.LaunchAppInWeb,
			},
			{
				Name:    "pwa_lacros",
				Fixture: fixture.LoggedInLacrosWithFakeHALAndEffectsDisabled,
				Val:     common.LaunchAppInPWA,
			},
			{
				Name:    "web_lacros",
				Fixture: fixture.LoggedInLacrosWithFakeHALAndEffectsDisabled,
				Val:     common.LaunchAppInWeb,
			},
		},
	})
}

func LiveCaptionTabPwa(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	vcTray := vctray.New(ctx, tconn)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "caption")

	// Open an empty chrome tab.
	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	ui := uiauto.New(tconn)

	playAudioAction := ui.DoDefault(nil)

	// Launch either the tab or the pwa.
	if s.Param().(common.LaunchAppType) == common.LaunchAppInPWA {
		appID := fakepwa.SetupServerAndPermission(ctx, br, s, tconn)
		pwaUI, err := fakepwa.LaunchApp(ctx, tconn, br, appID)
		if err != nil {
			s.Fatal("Failed to open pwa: ", err)
		}

		if err := uiauto.Combine("activate mic",
			pwaUI.StartAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that pwa triggers vcTray by mic: ", err)
		}

		playAudioAction = pwaUI.PlayAudio

	} else {
		vcTabFullURL := fakevctab.SetupServerAndPermission(ctx, br, s)
		tabUI, err := fakevctab.LaunchTab(ctx, tconn, br, vcTabFullURL)
		if err != nil {
			s.Fatal("Failed to open tab: ", err)
		}

		if err := uiauto.Combine("activate mic",
			tabUI.StartAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that pwa triggers vcTray by mic: ", err)
		}

		playAudioAction = tabUI.PlayAudio
	}

	// Enable LiveCaption from the vcTray.
	if err := vcTray.ChangeSettingsInPanel(vcTray.SetLiveCaption(true))(ctx); err != nil {
		s.Fatal("Failed to turn on live caption: ", err)
	}

	// Wait until dlc libsoda and libsoda-model-en-us are installed.
	if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to wait for libsoda dlc to be installed: ", err)
	}

	// Play audio and wait for the caption.
	// This need to be triggered multiple times because of its flakiness.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// Play audio.
		if err := playAudioAction(ctx); err != nil {
			return errors.Wrap(err, "failed to play audio")
		}

		// Wait for the buble to show.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return uiauto.Combine("Wait for the bubble and content",
				ui.WaitUntilExists(liveCaptionBubble),
				ui.WaitUntilExists(liveCaptionContent),
			)(ctx)
		}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: 1 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to wait for the caption buble and content")
		}
		return nil

	}, &testing.PollOptions{Timeout: 60 * time.Second, Interval: 10 * time.Second}); err != nil {
		s.Fatal("Failed to validate live caption: ", err)
	}

	if err := vcTray.ChangeSettingsInPanel(vcTray.SetLiveCaption(false))(ctx); err != nil {
		s.Fatal("Failed to turn off live caption: ", err)
	}

	// Live caption bubble should disappear after switching off.
	if err := ui.WithTimeout(10 * time.Second).WaitUntilGone(liveCaptionBubble)(ctx); err != nil {
		s.Fatal("Failed to wait for live caption disappear: ", err)
	}
}
