// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromevoxToggleOnShortcut,
		LacrosStatus: testing.LacrosVariantUnneeded, // TODO(crbug.com/1159107): Test is disabled in continuous testing. Migrate when enabled.
		Desc:         "A test that verifies Ctrl+Alt+Z toggles on Chromevox",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"dtseng@chromium.org",          // Test author
		},
		BugComponent: "b:1272895",
		// TODO(https://crbug.com/1159107): Investigate failures and re-enable this test.
		SoftwareDeps: []string{"chrome"},
		Pre:          chrome.LoggedIn(),
	})
}

func ChromevoxToggleOnShortcut(ctx context.Context, s *testing.State) {
	cr := s.PreValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)

	// Mute the device to avoid noisiness.
	if err := crastestclient.Mute(ctx); err != nil {
		s.Fatal("Failed to mute: ", err)
	}
	ctxCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()
	defer crastestclient.Unmute(ctxCleanup)

	// Get a speech monitor for the Google TTS engine.
	ed := tts.EngineData{
		ExtID:                     tts.GoogleTTSExtensionID,
		UseOnSpeakWithAudioStream: false,
	}

	sm, err := tts.RelevantSpeechMonitor(ctx, cr, tconn, ed)
	if err != nil {
		s.Fatal("Failed to connect to the TTS background page: ", err)
	}
	defer sm.Close()

	ctrlAltZ := []string{"Ctrl+Alt+z"}
	expectedSpeech := []tts.SpeechExpectation{tts.NewRegexExpectation("ChromeVox spoken feedback is ready")}

	// Use the speech monitor to ensure that the spoken announcement was given.
	if err := tts.PressKeysAndConsumeExpectations(ctx, sm, ctrlAltZ, expectedSpeech); err != nil {
		s.Fatal("Failed to verify Chromevox toggled on: ", err)
	}
}
