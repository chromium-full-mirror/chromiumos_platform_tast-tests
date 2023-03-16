// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/a11y"
	"chromiumos/tast/local/a11y/chromevox"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/testing"
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
					VoiceData: a11y.VoiceData{
						ExtID:  a11y.GoogleTTSExtensionID,
						Locale: "en-US",
					},
					EngineData: a11y.TTSEngineData{
						ExtID:                     a11y.GoogleTTSExtensionID,
						UseOnSpeakWithAudioStream: false,
					},
				},
				browserType: browser.TypeAsh,
			},
		}, {
			Name:    "espeak",
			Fixture: "chromeLoggedIn",
			Val: testParam{
				testData: chromevox.VoiceData{
					VoiceData: a11y.VoiceData{
						// eSpeak does not come with an English voice built-in, so we need to
						// use another language. We use Greek here since the voice is built-in
						// and capable of speaking English words.
						ExtID:  a11y.ESpeakExtensionID,
						Locale: "el",
					},
					EngineData: a11y.TTSEngineData{
						ExtID:                     a11y.ESpeakExtensionID,
						UseOnSpeakWithAudioStream: true,
					},
				},
				browserType: browser.TypeAsh,
			},
		}, {
			Name:              "lacros_google_tts",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: testParam{
				testData: chromevox.VoiceData{
					VoiceData: a11y.VoiceData{
						ExtID:  a11y.GoogleTTSExtensionID,
						Locale: "en-US",
					},
					EngineData: a11y.TTSEngineData{
						ExtID:                     a11y.GoogleTTSExtensionID,
						UseOnSpeakWithAudioStream: false,
					},
				},
				browserType: browser.TypeLacros,
			},
		}, {
			Name:              "lacros_espeak",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: testParam{
				testData: chromevox.VoiceData{
					VoiceData: a11y.VoiceData{
						// eSpeak does not come with an English voice built-in, so we need to
						// use another language. We use Greek here since the voice is built-in
						// and capable of speaking English words.
						ExtID:  a11y.ESpeakExtensionID,
						Locale: "el",
					},
					EngineData: a11y.TTSEngineData{
						ExtID:                     a11y.ESpeakExtensionID,
						UseOnSpeakWithAudioStream: true,
					},
				},
				browserType: browser.TypeLacros,
			},
		}},
	})
}

func Chromevox(ctx context.Context, s *testing.State) {
	ctxCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	td := s.Param().(testParam).testData
	bt := s.Param().(testParam).browserType
	const html = "<p>Start</p><p>This is a ChromeVox test</p><p>End</p>"
	cvData, err := a11y.SetUpChromeVox(ctx, ctxCleanup, cr, td.VoiceData, td.EngineData, bt, html)
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
		Expectations []a11y.SpeechExpectation
	}{
		{
			[]string{chromevox.NextObject},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("Start")},
		},
		{
			[]string{chromevox.NextObject},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("This is a ChromeVox test")},
		},
		{
			[]string{chromevox.NextObject},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("End")},
		},
		{
			[]string{chromevox.PreviousObject},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("This is a ChromeVox test")},
		},
		{
			[]string{chromevox.PreviousObject},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("Start")},
		},
		{
			[]string{chromevox.JumpToLauncher},
			[]a11y.SpeechExpectation{a11y.NewRegexExpectation("(Launcher|Back)")},
		},
		{
			[]string{chromevox.JumpToStatusTray},
			[]a11y.SpeechExpectation{a11y.NewRegexExpectation("Quick Settings*")},
		},
	}

	for _, step := range testSteps {
		if err := a11y.PressKeysAndConsumeExpectations(ctx, cvData.SM, step.KeyCommands, step.Expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
