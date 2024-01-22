// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/testing"
)

type builtInVoicesTestParam struct {
	testData          chromevox.VoiceData
	expectedLocale    string
	expectedVoiceName string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromevoxBuiltInVoices,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that ChromeVox works with all built-in Google TTS voices",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1272895",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "english",
			Fixture: "chromeLoggedIn",
			Val: builtInVoicesTestParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.GoogleTTSEnUsVoice(),
					EngineData: tts.GoogleTTSEngine(),
				},
				expectedLocale:    "en-US",
				expectedVoiceName: "Chrome OS US English 1",
			},
		}, {
			Name:    "spanish",
			Fixture: "chromeLoggedIn",
			Val: builtInVoicesTestParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.GoogleTTSEsEsVoice(),
					EngineData: tts.GoogleTTSEngine(),
				},
				expectedLocale:    "es-ES",
				expectedVoiceName: "Chrome OS español 1",
			},
		}, {
			Name:    "french",
			Fixture: "chromeLoggedIn",
			Val: builtInVoicesTestParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.GoogleTTSFrFrVoice(),
					EngineData: tts.GoogleTTSEngine(),
				},
				expectedLocale:    "fr-FR",
				expectedVoiceName: "Chrome OS français 1",
			},
		}, {
			Name:    "hindi",
			Fixture: "chromeLoggedIn",
			Val: builtInVoicesTestParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.GoogleTTSHiInVoice(),
					EngineData: tts.GoogleTTSEngine(),
				},
				expectedLocale:    "hi-IN",
				expectedVoiceName: "Chrome OS हिन्दी 1",
			},
		}, {
			Name:    "japanese",
			Fixture: "chromeLoggedIn",
			Val: builtInVoicesTestParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.GoogleTTSJaJpVoice(),
					EngineData: tts.GoogleTTSEngine(),
				},
				expectedLocale:    "ja-JP",
				expectedVoiceName: "Chrome OS 日本語 1",
			},
		}, {
			Name:    "dutch",
			Fixture: "chromeLoggedIn",
			Val: builtInVoicesTestParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.GoogleTTSNlNlVoice(),
					EngineData: tts.GoogleTTSEngine(),
				},
				expectedLocale:    "nl-NL",
				expectedVoiceName: "Chrome OS Nederlands 1",
			},
		}},
	})
}

func ChromevoxBuiltInVoices(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	td := s.Param().(builtInVoicesTestParam).testData
	bt := browser.TypeAsh
	locale := s.Param().(builtInVoicesTestParam).expectedLocale
	voiceName := s.Param().(builtInVoicesTestParam).expectedVoiceName
	const html = "<p>Test</p>"
	cvData, err := chromevox.SetUp(ctx, cr, td.VoiceData, td.EngineData, bt, html)
	if err != nil {
		s.Fatal("Failed to set up ChromeVox: ", err)
	}
	defer func() {
		if err := cvData.TearDown(); err != nil {
			s.Fatal("Failed to tear down ChromeVox test: ", err)
		}
	}()

	command := []string{chromevox.NextObject}
	expectation := []tts.SpeechExpectation{tts.NewOptionsVoiceExpectation("Test", locale, voiceName)}

	if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), command, expectation); err != nil {
		s.Error("Error when pressing keys and expecting speech: ", err)
	}
}
