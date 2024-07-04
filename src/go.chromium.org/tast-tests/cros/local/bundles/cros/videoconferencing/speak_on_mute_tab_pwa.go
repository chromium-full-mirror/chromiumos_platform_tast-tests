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
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/wav"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakepwa"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/fakevctab"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/input/voice"
	"go.chromium.org/tast-tests/cros/local/videoconferencing/fixture"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SpeakOnMuteTabPwa,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks Speak-On-Mute is functional in Google Meet",
		Contacts: []string{
			"cros-video-conference-tast-tests@google.com",
			"xiuwen@google.com",
		},
		Attr: []string{
			"group:external-dependency",
			"group:cbx", "cbx_feature_enabled", "cbx_unstable",
		},
		TestBedDeps: []string{tbdep.Cbx(true)},
		Data: []string{
			data.SpeechInputFile,
			data.VcAppHTML,
			data.VcAppJs,
			data.VcAppIcon,
			data.VcAppManifest,
		},
		BugComponent: "b:187682",
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"chrome"},
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
				Fixture: fixture.LoggedInWithFakeHALAndEffectsDisabled,
				Val:     common.LaunchAppInWeb,
			},
			{
				Name:    "web_lacros",
				Fixture: fixture.LoggedInLacrosWithFakeHALAndEffectsDisabled,
				Val:     common.LaunchAppInWeb,
			},
			{
				Name:    "pwa",
				Fixture: fixture.LoggedInWithFakeHALAndEffectsDisabled,
				Val:     common.LaunchAppInPWA,
			},
			{
				Name:    "pwa_lacros",
				Fixture: fixture.LoggedInLacrosWithFakeHALAndEffectsDisabled,
				Val:     common.LaunchAppInPWA,
			},
		},
	})
}

func SpeakOnMuteTabPwa(cleanupCtx context.Context, s *testing.State) {
	ctx, tconn, cr, br, srvURL, cleanupFunc := common.Setup(cleanupCtx, s)
	defer cleanupFunc()

	vcTray := vctray.New(ctx, tconn)

	// Setup CRAS Aloop for audio test.
	if err := voice.ActivateAloopNodes(ctx, tconn, voice.LoopbackCapture); err != nil {
		s.Fatal("Failed to load Aloop: ", err)
	}

	if err := ossettings.ToggleMuteNudgeWithErrorDump(cr, tconn, true, s.OutDir())(ctx); err != nil {
		s.Fatal("Failed to toggle on mute nudge: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")
	defer func(ctx context.Context) {
		if s.HasError() {
			if err := crastestclient.DumpAudioDiagnostics(ctx, s.OutDir()); err != nil {
				s.Error("Failed to dump audio diagnostics: ", err)
			}
		}
	}(cleanupCtx)

	vcTesterFullURL := srvURL + data.VcAppHTML
	if s.Param().(common.LaunchAppType) == common.LaunchAppInPWA {
		// Install PWA.
		appID, err := fakepwa.InstallPwa(ctx, br, tconn, vcTesterFullURL)
		if err != nil {
			s.Fatal("fail to install pwa: ", err)
		}
		pwaUI, err := fakepwa.LaunchApp(ctx, tconn, br, appID)
		if err != nil {
			s.Fatal("Failed to open pwa: ", err)
		}
		defer pwaUI.Close(ctx)
		if err := uiauto.Combine("activate mic",
			pwaUI.StartAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that pwa triggers vcTray by mic: ", err)
		}
	} else {
		tabUI, err := fakevctab.LaunchTab(ctx, tconn, br, vcTesterFullURL)
		if err != nil {
			s.Fatal("Failed to open tab: ", err)
		}

		if err := uiauto.Combine("activate mic",
			tabUI.StartAudio,
			vcTray.WaitUntilState(vctray.DevMicrophone, vctray.DeviceInUse),
			vcTray.WaitUntilState(vctray.DevCamera, vctray.DeviceAvailable),
			vcTray.WaitUntilState(vctray.DevScreen, vctray.DeviceHidden),
		)(ctx); err != nil {
			s.Fatal("Failed to verify that tab triggers vcTray by mic: ", err)
		}
		defer tabUI.Close(ctx)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_with_meet")

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
