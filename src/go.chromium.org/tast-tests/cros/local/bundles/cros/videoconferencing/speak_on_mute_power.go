// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package videoconferencing

import (
	"context"
	"path/filepath"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/wav"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/common"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/videoconferencing/data"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/vctray"
	"go.chromium.org/tast-tests/cros/local/input/voice"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SpeakOnMutePower,
		LacrosStatus: testing.LacrosVariantUnneeded, // Browser only used to trigger VC UI.
		Desc:         "Checks Speak-On-Mute power usage",
		Contacts: []string{
			"chrome-knowledge-eng@google.com",
			"chenjih@google.com",
			"aaronyu@google.com",
		},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Data:         []string{data.SpeechInputFile},
		BugComponent: "b:187682",
		Timeout:      3*time.Minute + power.RecorderTimeout,
		SoftwareDeps: []string{"chrome", "camera_feature_effects"},
		Fixture:      "powerAshGAIAWithSpeakOnMute",
		Params: []testing.Param{
			{
				Name: "disabled",
				Val: speakOnMutePowerParam{
					enabled: false,
				},
			},
			{
				Name: "enabled",
				Val: speakOnMutePowerParam{
					enabled: true,
				},
			},
		},
	})
}

type speakOnMutePowerParam struct {
	enabled bool
}

func SpeakOnMutePower(ctx context.Context, s *testing.State) {
	param := s.Param().(speakOnMutePowerParam)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(setup.PowerUIFixtureData).Cr

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	// Setup CRAS Aloop for audio test.
	s.Log("Activating Aloop nodes")
	if err := voice.ActivateAloopNodes(ctx, tconn, voice.LoopbackCapture); err != nil {
		s.Fatal("Failed to load Aloop: ", err)
	}

	if err := ossettings.ToggleMuteNudgeWithErrorDump(cr, tconn, param.enabled, s.OutDir())(ctx); err != nil {
		s.Fatal("Failed to toggle on mute nudge: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui")

	vcTray := vctray.New(ctx, tconn)

	defer func(ctx context.Context) error {
		if err := vcTray.ToggleAVDevice(vctray.DevMicrophone, true)(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to toggle on microphone in cleanup: ", err)
		}
		return nil
	}(cleanupCtx)

	ui := uiauto.New(tconn)
	nudgeWaitDuration := 10 * time.Second
	waitForNudge := func(ctx context.Context) error {
		// GoBigSleepLint: For symmetry in the enabled = false test.
		return testing.Sleep(ctx, nudgeWaitDuration)
	}
	if param.enabled {
		waitForNudge = ui.WithTimeout(nudgeWaitDuration).WaitUntilExists(common.SpeakOnMuteNudge)
	}
	muteAndWaitForNudge := uiauto.Combine("mute and speak",
		vcTray.ToggleAVDevice(vctray.DevMicrophone, false),
		waitForNudge,
	)

	const testDuration = time.Minute

	extendedSpeechWav := filepath.Join(s.OutDir(), "speech.wav")
	if err := wav.RepeatForDuration(ctx, s.DataPath(data.SpeechInputFile), extendedSpeechWav, testDuration); err != nil {
		s.Fatal("Cannot prepare wav file: ", err)
	}

	// Open an arbitrary web page that has access to navigator.mediaDevices.
	recorderPage, err := cr.Browser().NewConn(ctx, "chrome://version")
	if err != nil {
		s.Fatal("Cannot open browser: ", err)
	}

	r := power.NewRecorder(ctx, time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)
	if err := r.Cooldown(ctx); err != nil {
		s.Error("Cooldown failed: ", err)
	}
	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Test body. Executes for testDuration.
	{
		var wg sync.WaitGroup
		// Speak in the background.
		wg.Add(1)
		go func(ctx context.Context) {
			defer wg.Done()
			if err := audio.PlayWavToPCM(ctx, extendedSpeechWav, "hw:Loopback,0"); err != nil {
				if s.HasError() && errors.Is(err, context.DeadlineExceeded) {
					return
				}
				s.Error("Failed to play wav to PCM: ", err)
			}
		}(ctx)

		// Record from Chrome.
		s.Log("Recording audio from Chrome")
		if err := recorderPage.Eval(ctx,
			`var stream;
		async function f() {
			stream = await navigator.mediaDevices.getUserMedia({audio: true});
		}
		f()`,
			nil); err != nil {
			s.Fatal("Cannot start capture in Chrome: ", err)
		}

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
			s.Fatal("Failed to input audio and wait for nudge after reset: ", err)
		}

		s.Log("Waiting for playback to complete")
		wg.Wait()
	}
	// End of main test body.

	if err := r.Finish(ctx); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}
