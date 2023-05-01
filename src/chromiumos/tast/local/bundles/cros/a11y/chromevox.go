// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"

	"chromiumos/tast/local/a11y/chromevox"
	"chromiumos/tast/local/a11y/tts"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"go.chromium.org/tast/core/testing"
)

type testParam struct {
	testData    chromevox.VoiceData
	browserType browser.Type
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Chromevox,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A spoken feedback test that executes ChromeVox commands and keyboard shortcuts, and verifies that correct speech is given by the Google and eSpeak TTS engines",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1272895",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "google_tts",
			Fixture: "chromeLoggedIn",
			Val: testParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.GoogleTTSEnUsVoice(),
					EngineData: tts.GoogleTTSEngine(),
				},
				browserType: browser.TypeAsh,
			},
		}, {
			Name:    "espeak",
			Fixture: "chromeLoggedIn",
			Val: testParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.EspeakElVoice(),
					EngineData: tts.EspeakEngine(),
				},
				browserType: browser.TypeAsh,
			},
		}, {
			Name:              "lacros_google_tts",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: testParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.GoogleTTSEnUsVoice(),
					EngineData: tts.GoogleTTSEngine(),
				},
				browserType: browser.TypeLacros,
			},
		}, {
			Name:              "lacros_espeak",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: testParam{
				testData: chromevox.VoiceData{
					VoiceData:  tts.EspeakElVoice(),
					EngineData: tts.EspeakEngine(),
				},
				browserType: browser.TypeLacros,
			},
		}},
	})
}

func Chromevox(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	td := s.Param().(testParam).testData
	bt := s.Param().(testParam).browserType
	const html = "<p>Start</p><p>This is a ChromeVox test</p><p>End</p>"
	cvData, err := chromevox.SetUp(ctx, cr, td.VoiceData, td.EngineData, bt, html)
	if err != nil {
		s.Fatal("Failed to set up ChromeVox: ", err)
	}
	defer func() {
		if err := cvData.TearDown(); err != nil {
			s.Fatal("Failed to tear down ChromeVox test: ", err)
		}
	}()

	testSteps := []struct {
		KeyCommands  []string
		Expectations []tts.SpeechExpectation
	}{
		{
			[]string{chromevox.NextObject},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Start")},
		},
		{
			[]string{chromevox.NextObject},
			[]tts.SpeechExpectation{tts.NewStringExpectation("This is a ChromeVox test")},
		},
		{
			[]string{chromevox.NextObject},
			[]tts.SpeechExpectation{tts.NewStringExpectation("End")},
		},
		{
			[]string{chromevox.PreviousObject},
			[]tts.SpeechExpectation{tts.NewStringExpectation("This is a ChromeVox test")},
		},
		{
			[]string{chromevox.PreviousObject},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Start")},
		},
		{
			[]string{chromevox.JumpToLauncher},
			[]tts.SpeechExpectation{tts.NewRegexExpectation("(Launcher|Back)")},
		},
		{
			[]string{chromevox.JumpToStatusTray},
			[]tts.SpeechExpectation{tts.NewRegexExpectation("Quick Settings*")},
		},
	}

	for _, step := range testSteps {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.KeyCommands, step.Expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
