// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/audio"
	"chromiumos/tast/local/bundles/cros/videoconferencing/commontype"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/googlemeet"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/input/voice"
	"chromiumos/tast/local/videoconferencing/fixture"
	"chromiumos/tast/testing"
)

const audioInputFile = "voice_en_hello.wav"

func init() {
	testing.AddTest(&testing.Test{
		Func:         MeetSpeakOnMute,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Speak-On-Mute is functional in Google Meet",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"shengjun@chromium.org",
		},
		Attr: []string{
			"group:camera_dependent",
			"group:external-dependency",
		},
		Data:         []string{audioInputFile},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		// TODO(b/276998230): Add attributes to enable this test in CI.
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		Params: []testing.Param{
			{
				Name:    "web",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "web_lacros",
				Fixture: fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInWeb,
			},
			{
				Name:    "pwa",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
			{
				Name:    "pwa_lacros",
				Fixture: fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:     commontype.LaunchAppInPWA,
			},
		},
	})
}

var speakOnMuteToast = nodewith.NameStartingWith("Are you speaking? You are on mute").HasClass("SystemToastInnerLabel")

func MeetSpeakOnMute(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Setup CRAS Aloop for audio test.
	if err := voice.ActivateAloopNodes(ctx, tconn, voice.LoopbackCapture); err != nil {
		s.Fatal("Failed to load Aloop: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	br, cleanup, err := browserfixt.SetUp(ctx, cr, browserType)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	var gm *googlemeet.GoogleMeet

	if s.Param().(commontype.LaunchAppType) == commontype.LaunchAppInPWA {
		gm, err = googlemeet.StartNewMeetingUsingPWA(ctx, cr, br, googlemeet.WithAllPermissions)
	} else {
		// Meet can dynamically switch between different segmentation models.
		// Force the same model the platform effects use with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
		gm, err = googlemeet.StartNewMeeting(ctx, cr, br,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			}, googlemeet.WithAllPermissions)
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_with_meet")

	// TODO(b/276998230): Use vcTray to mute and unmute
	// Note: This test is only semi-automated due to the blocking bug.
	s.Log("Please mute via vctray on DUT manually within 20s")
	testing.Sleep(ctx, 20*time.Second) // GoBigSleepLint
	s.Log("Finished waiting")

	defer func(ctx context.Context) {
		s.Log("Please unmute via vctray on DUT manually within 20s")
		testing.Sleep(ctx, 20*time.Second) // GoBigSleepLint
		s.Log("Finished waiting")
	}(cleanupCtx)

	if err := audio.PlayWavToPCM(ctx, s.DataPath(audioInputFile), "hw:Loopback,0"); err != nil {
		s.Fatal("Failed to input audio: ", err)
	}

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("speakOnMuteToast should appear and auto clear",
		ui.WaitUntilExists(speakOnMuteToast),
		ui.WaitUntilGone(speakOnMuteToast),
	)(ctx); err != nil {
		s.Fatal("Failed to verify speak on mute: ", err)
	}
}
