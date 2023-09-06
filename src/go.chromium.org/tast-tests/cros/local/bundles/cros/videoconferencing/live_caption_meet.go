// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/bond"
	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const credsVarName = "ui.bond_credentials"

func init() {
	testing.AddTest(&testing.Test{
		Func:         LiveCaptionMeet,
		LacrosStatus: testing.LacrosVariantExists,
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
		SearchFlags: []*testing.StringPair{
			{
				// Live captions display on Meet.
				Key:   "feature_id",
				Value: "screenplay-6bddb622-b203-4c4f-9dec-47df1a280f21",
			},
			{
				// Remove Live captions on Meet.
				Key:   "feature_id",
				Value: "screenplay-25b74f07-6422-4f4d-90e9-193ac21063d2",
			},
			{
				// Retain Live captions on Meet.
				Key:   "feature_id",
				Value: "screenplay-447a9654-a25f-45b6-9b3d-bf95b9864d64",
			},
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
		},
	})
}

func LiveCaptionMeet(ctx context.Context, s *testing.State) {
	const addBotTimeout = 100 * time.Second
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
	meetingCode, err := bondClient.CreateConference(ctx)
	if err != nil {
		s.Fatal("Failed to create conference: ", err)
	}
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

	conn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer closeBrowser(cleanupCtx)
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	var gm *googlemeet.GoogleMeet
	if s.Param().(common.LaunchAppType) == common.LaunchAppInPWA {
		gm, err = googlemeet.JoinMeetingUsingPWA(ctx, cr, br, meetingCode, googlemeet.WithAllPermissions)
	} else {
		// Meet can dynamically switch between different segmentation models.
		// Force the same model the platform effects use with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
		gm, err = googlemeet.JoinMeeting(ctx, cr, br, conn, meetingCode,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			}, googlemeet.WithAllPermissions)
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	ui := uiauto.New(tconn)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "caption")

	liveCaptionBubble := nodewith.ClassName("CaptionBubbleFrameView")
	// Only the text in first row can be validated, as each row is a separate node.
	liveCaptionContent := nodewith.Role(role.StaticText).Ancestor(liveCaptionBubble).First()

	// The expected caption content is from the bot audio input.
	// The input file is hardcoded in bondClient.AddBots options.
	// Since the recognition of the live caption may be inaccurate, the recognition
	// standard was changed from "what color is cheese" to "what color".
	expectedCaptionContain := "what color"

	turnOnLiveCaptionAndCheckBubble := func(checkDLC bool) {
		if err := ossettings.ToggleLiveCaption(cr, tconn, true)(ctx); err != nil {
			s.Fatal("Failed to toggle on live caption: ", err)
		}

		if checkDLC {
			// Wait until dlc libsoda and libsoda-model-en-us are installed.
			if err := testing.Poll(ctx, a11y.VerifySodaInstalled, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
				s.Fatal("Failed to wait for libsoda dlc to be installed: ", err)
			}
		}

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
	}

	// Check DLC mounted for the first time switch on.
	turnOnLiveCaptionAndCheckBubble(true)

	if err := ossettings.ToggleLiveCaption(cr, tconn, false)(ctx); err != nil {
		s.Fatal("Failed to toggle off live caption: ", err)
	}

	// Live caption bubble should disappear after switching off.
	if err := ui.WithTimeout(10 * time.Second).WaitUntilGone(liveCaptionBubble)(ctx); err != nil {
		s.Fatal("Failed to wait for live caption disappear: ", err)
	}

	// Switch on live caption again.
	turnOnLiveCaptionAndCheckBubble(false)
}
