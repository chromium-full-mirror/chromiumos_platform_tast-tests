// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"time"

	"chromiumos/tast/local/audio"
	"chromiumos/tast/local/bundles/cros/videoconferencing/common"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/apps/thirdparty/googlemeet"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/vctray"
	"chromiumos/tast/local/input/voice"
	"chromiumos/tast/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
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
			"group:video_conference",
			"video_conference_per_build",
		},
		Data:         []string{audioInputFile},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		Params: []testing.Param{
			{
				Name:    "web",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInWeb,
			},
			{
				Name:    "web_lacros",
				Fixture: fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInWeb,
			},
			{
				Name:    "pwa",
				Fixture: fixture.GAIALoggedInWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInPWA,
			},
			{
				Name:    "pwa_lacros",
				Fixture: fixture.GAIALoggedInLacrosWithFakeHALAndEffectsEnabled,
				Val:     common.LaunchAppInPWA,
			},
		},
	})
}

var speakOnMuteToast = nodewith.NameStartingWith("Are you talking?").HasClass("SystemToastInnerLabel")

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

	conn, br, cleanup, err := browserfixt.SetUpWithURL(ctx, cr, browserType, chrome.NewTabURL)
	if err != nil {
		s.Fatal("Failed to launch browser: ", err)
	}
	defer cleanup(cleanupCtx)

	var gm *googlemeet.GoogleMeet

	if s.Param().(common.LaunchAppType) == common.LaunchAppInPWA {
		gm, err = googlemeet.StartNewMeetingUsingPWA(ctx, cr, br, googlemeet.WithAllPermissions)
	} else {
		// Meet can dynamically switch between different segmentation models.
		// Force the same model the platform effects use with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
		gm, err = googlemeet.StartNewMeeting(ctx, cr, br, conn,
			map[string]string{
				"e": "ForceSegmentationModelVariant::GpuMid",
			}, googlemeet.WithAllPermissions)
	}
	if err != nil {
		s.Fatal("Failed to start meeting: ", err)
	}
	defer gm.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_with_meet")

	vcTray := vctray.New(ctx, tconn)

	defer func(ctx context.Context) error {
		if err := vcTray.ToggleAVDevice(vctray.DevMicrophone, true)(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to toggle on microphone in cleanup: ", err)
		}
		return nil
	}(cleanupCtx)

	ui := uiauto.New(tconn)

	speakAndWaitForToast := uiauto.Combine("mute and speak",
		vcTray.ToggleAVDevice(vctray.DevMicrophone, false),
		uiauto.Retry(10, uiauto.Combine("",
			func(ctx context.Context) error {
				return audio.PlayWavToPCM(ctx, s.DataPath(audioInputFile), "hw:Loopback,0")
			},
			ui.WithTimeout(time.Second).WaitUntilExists(speakOnMuteToast),
		)),
	)

	if err := speakAndWaitForToast(ctx); err != nil {
		s.Fatal("Failed to input audio and wait for toast: ", err)
	}

	if err := uiauto.Combine("unmute clears toast",
		vcTray.ToggleAVDevice(vctray.DevMicrophone, true),
		ui.WaitUntilGone(speakOnMuteToast),
	)(ctx); err != nil {
		s.Fatal("Failed to verify unmute: ", err)
	}

	// Toast time frame should reset by unmute.
	// Mute and speak again should trigger the toast.
	if err := speakAndWaitForToast(ctx); err != nil {
		s.Fatal("Failed to input audio and wait for toast after reset: ", err)
	}
}
