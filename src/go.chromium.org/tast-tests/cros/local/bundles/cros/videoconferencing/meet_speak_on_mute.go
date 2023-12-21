// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"path/filepath"
	"strings"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/wav"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/data"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/input/voice"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

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
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps:  []string{tbdep.Cbx(false)},
		Data:         []string{data.SpeechInputFile},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		SearchFlags: []*testing.StringPair{
			{
				// Mute Mic and speak more.
				Key:   "feature_id",
				Value: "screenplay-d03cca4a-3789-4003-bcfe-67e8595601fd",
			},
			{
				// Notification timeframe reset by unmute Mic.
				Key:   "feature_id",
				Value: "screenplay-6d4f36ee-fac1-4112-8593-190e813bbd43",
			},
		},
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

	if err := ossettings.ToggleMuteNudgeWithErrorDump(cr, tconn, true, s.OutDir())(ctx); err != nil {
		s.Fatal("Failed to toggle on mute nudge: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	browserType := s.FixtValue().(fixture.FixtData).BrowserType()

	var gm *googlemeet.GoogleMeet

	if s.Param().(common.LaunchAppType) == common.LaunchAppInPWA {
		gm, err = googlemeet.StartNewMeetingUsingPWA(ctx, cr, browserType, googlemeet.WithAllPermissions)
	} else {
		// Meet can dynamically switch between different segmentation models.
		// Force the same model the platform effects use with the experiment ?e=ForceSegmentationModelVariant::GpuMid.
		gm, err = googlemeet.StartNewMeetingUsingBrowser(ctx, cr, browserType,
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

	// Nudge should appear immediately after mute while speaking.
	nudgeWaitDuration := 10 * time.Second
	waitForNudge := ui.WithTimeout(nudgeWaitDuration).WaitUntilExists(common.SpeakOnMuteNudge)

	muteAndWaitForNudge := uiauto.Combine("mute and speak",
		vcTray.ToggleAVDevice(vctray.DevMicrophone, false),
		waitForNudge,
	)

	playDuration := 1 * time.Minute
	extendedSpeechWav := filepath.Join(s.OutDir(), "speech.wav")
	if err := wav.RepeatForDuration(ctx, s.DataPath(data.SpeechInputFile), extendedSpeechWav, playDuration); err != nil {
		s.Fatal("Cannot prepare wav file: ", err)
	}

	var wg sync.WaitGroup
	// Speak in the background.
	wg.Add(1)
	go func(ctx context.Context) {
		if err := audio.PlayWavToPCM(ctx, extendedSpeechWav, "hw:Loopback,0"); err != nil {
			if strings.Contains(err.Error(), "context canceled") {
				return
			}
			s.Error("Failed to play wav to PCM: ", err)
		}
	}(ctx)

	if err := muteAndWaitForNudge(ctx); err != nil {
		s.Fatal("Failed to wait for nudge: ", err)
	}

	if err := uiauto.Combine("unmute clears nudge",
		vcTray.ToggleAVDevice(vctray.DevMicrophone, true),
		ui.WaitUntilGone(common.SpeakOnMuteNudge),
	)(ctx); err != nil {
		s.Fatal("Failed to verify unmute: ", err)
	}

	// Nudge time frame should reset by unmute.
	// Mute and speak again should trigger the Nudge.
	if err := muteAndWaitForNudge(ctx); err != nil {
		s.Fatal("Failed to wait for nudge after reset: ", err)
	}

	s.Log("End the playback")
	wg.Done()
}
