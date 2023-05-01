// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/common/bond"
	"chromiumos/tast/local/a11y"
	"chromiumos/tast/local/bundles/cros/videoconferencing/common"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/googlemeet"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const credsVarName = "ui.bond_credentials"

func init() {
	testing.AddTest(&testing.Test{
		Func:         MeetLiveCaption,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks on-device live caption works in Google Meet",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		BugComponent: "b:187682",
		Timeout:      10 * time.Minute,
		Attr: []string{
			"group:camera_dependent",
			"group:external-dependency",
			"group:video_conference",
			"video_conference_per_build",
		},
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		VarDeps: []string{
			credsVarName,
		},
		Params: []testing.Param{
			{
				Name:    "pwa",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInPWA,
			},
			{
				Name:    "web",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInWeb,
			},
			{
				Name:    "pwa_lacros",
				Fixture: fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInPWA,
			},
			{
				Name:    "web_lacros",
				Fixture: fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInWeb,
			},
		},
	})
}

func MeetLiveCaption(ctx context.Context, s *testing.State) {
	const (
		createConfTimeout = 30 * time.Second
		addBotTimeout     = 100 * time.Second
	)
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	// Initialize a bond client.
	creds := s.RequiredVar(credsVarName)
	bondClient, err := bond.NewClient(ctx, bond.WithCredsJSON([]byte(creds)))
	if err != nil {
		s.Fatal("Failed to create a bond client: ", err)
	}
	defer bondClient.Close()

	// Create a meeting via bond API.
	var meetingCode string
	func(ctx context.Context) {
		// createConfTimeout(30s) would allow 3 bond.defaultSendTimeout(8s)
		// attempts to request the bond server to create conference.
		sctx, cancel := context.WithTimeout(ctx, createConfTimeout)
		defer cancel()
		meetingCode, err = bondClient.CreateConference(sctx)
		if err != nil {
			s.Fatal("Failed to create conference: ", err)
		}
	}(ctx)
	s.Log("Created a room with the code: ", meetingCode)

	// Register bots cleanup since the AddBots can partially fail.
	defer func(ctx context.Context) {
		s.Log("Removing all bots from the call")
		if _, _, err := bondClient.RemoveAllBots(ctx, meetingCode); err != nil {
			testing.ContextLog(ctx, "Failed to remove all bots: ", err)
		}
	}(cleanupCtx)

	// Add bots to meeting.
	numBots := 1                    // The amount of bots to add.
	botsDuration := 5 * time.Minute // 5 mins long by default.
	// addBotTimeout(100s) would allow 3 bond.longerSendTimeout(30s) attempts
	// to request the bond server to add bots.
	sctx, cancel := context.WithTimeout(ctx, addBotTimeout)
	defer cancel()
	_, nFailures, err := bondClient.AddBots(sctx, meetingCode, numBots, botsDuration, bond.WithAudio("what_color_is_cheese_32bit_48k_stereo.raw"))
	if err != nil || nFailures > 0 {
		s.Fatalf("Failed to add bots: %d bots are not added: %v", nFailures, err)
	}

	conn, _, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browserType, "")
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	var gm *googlemeet.GoogleMeet
	if s.Param().(common.LaunchAppType) == common.LaunchAppInPWA {
		gm, err = googlemeet.JoinMeetingUsingPWA(ctx, cr, meetingCode, googlemeet.WithAllPermissions)
	} else {
		// Meet can dynamically switch between different segmentation models.
		// Force the same model the platform effects use with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
		gm, err = googlemeet.JoinMeeting(ctx, cr, conn, meetingCode,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			}, googlemeet.WithAllPermissions)
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	vcTray := vctray.New(ctx, tconn)

	if err := vcTray.ChangeSettingsInPanel(vcTray.SetLiveCaption(true))(ctx); err != nil {
		s.Fatal("Failed to turn on live caption: ", err)
	}

	// Wait until dlc libsoda and libsoda-model-en-us are installed.
	if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to wait for libsoda dlc to be installed: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "caption")

	liveCaptionBubble := nodewith.ClassName("CaptionBubbleFrameView")
	// Only the text in first row can be validated, as each row is a separate node.
	liveCaptionContent := nodewith.Role(role.StaticText).Ancestor(liveCaptionBubble).First()

	ui := uiauto.New(tconn)

	// The expected caption content is from the bot audio input.
	// The input file is hardcoded in bondClient.AddBots options.
	expectedCaptionContain := "what color is cheese"
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		nodeInfo, err := ui.Info(ctx, liveCaptionContent)
		if err != nil {
			return err
		}

		if !strings.Contains(strings.ToLower(nodeInfo.Name), expectedCaptionContain) {
			return errors.Errorf("failed to validate caption content: expected contain %q, got %q", expectedCaptionContain, nodeInfo.Name)
		}
		return nil
	}, &testing.PollOptions{Timeout: 60 * time.Second}); err != nil {
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
