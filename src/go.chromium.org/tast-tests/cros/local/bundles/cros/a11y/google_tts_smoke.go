// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/audionode"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GoogleTtsSmoke,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A test that verifies that Google TTS can speak content through the device's internal speakers",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1272895",
		Attr:         []string{"group:mainline", "informational"},
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{{
			Name: "with_audio_verification",
			Val:  true,
		}, {
			Name: "without_audio_verification",
			Val:  false,
		}},
	})
}

func GoogleTtsSmoke(ctx context.Context, s *testing.State) {
	ctxCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.NoLogin(), chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctxCleanup)

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute device: ", err)
	}
	defer crastestclient.Unmute(ctxCleanup)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	// Wait for the welcome screen to be shown.
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the welcome screen to be visible: ", err)
	}

	ed := tts.GoogleTTSEngine()
	sm, err := tts.RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		s.Fatal("Failed to connect to the TTS background page: ", err)
	}
	defer sm.Close()

	verifyAudio := s.Param().(bool)
	if !verifyAudio {
		// Ask Google TTS to speak "Hello world".
		text := "Hello world"
		if err := sm.SendSpeechRequest(ctx, text); err != nil {
			s.Fatal("Failed to invoke speech: ", err)
		}

		// Verify that Google TTS spoke "Hello world".
		if err := sm.Consume(ctx, []tts.SpeechExpectation{tts.NewStringExpectation(text)}); err != nil {
			s.Fatal("Failed to consume speech: ", err)
		}

		return
	}

	// Ensure there is no audio playing from the speakers.
	if err := crastestclient.WaitForNoStream(ctx, 20*time.Second); err != nil {
		s.Fatal("Timeout waiting for all streams to stop: ", err)
	}

	// Set expected audio output device to "internal speaker".
	expectedDeviceName, err := audionode.SetAudioNode(ctx, "INTERNAL_SPEAKER")
	if err != nil {
		s.Fatal("Failed to set the Audio node: ", err)
	}

	// Ask Google TTS to speak "Hello world".
	text := "Hello world"
	if err := sm.SendSpeechRequest(ctx, text); err != nil {
		s.Fatal("Failed to invoke speech: ", err)
	}

	// Verify that audio was played from the speakers. Use a poll to increase
	// stability, since the timeout for FirstRunningDevice is three seconds.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		actualDeviceName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
		if err != nil {
			return errors.Wrap(err, "failed to detect running output device")
		}

		if expectedDeviceName != actualDeviceName {
			return errors.Wrapf(err, "failed to route the audio through expected audio node: got %q; want %q", actualDeviceName, expectedDeviceName)
		}

		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		s.Fatal("Failed to verify that audio was played through the speakers: ", err)
	}

	// Verify that Google TTS spoke "Hello world".
	if err := sm.Consume(ctx, []tts.SpeechExpectation{tts.NewStringExpectation(text)}); err != nil {
		s.Fatal("Failed to consume speech: ", err)
	}
}
