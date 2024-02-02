// Copyright 2021 The ChromiumOS Authors
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

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromevoxNumberReadingStyle,
		LacrosStatus: testing.LacrosVariantExists, // TODO(b:268196299): The ChromeVox options page will migrate to ChromeOS settings, so this test will need to be updated and re-enabled when the above bug is closed.
		Desc:         "Verifies ChromeVox honors its setting to read numbers as words or as digits",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",
		},
		BugComponent: "b:1272895",
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
			Val:               browser.TypeLacros,
		}},
	})
}

func ChromevoxNumberReadingStyle(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	vd := tts.GoogleTTSEnUsVoice()
	ed := tts.GoogleTTSEngine()
	bt := s.Param().(browser.Type)
	html := "<p>123</p>"
	cvData, err := chromevox.SetUp(ctx, cr, vd, ed, bt, html)
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
			[]tts.SpeechExpectation{tts.NewStringExpectation("123")},
		},
		{
			chromevox.OpenOptionsPage,
			[]tts.SpeechExpectation{tts.NewStringExpectation("Settings - ChromeVox")},
		},
		{
			[]string{chromevox.Find},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Find")},
		},
		{
			[]string{"R", "E", "A", "D", chromevox.Space, "N", "U", "M", "B", "E", "R", "S", chromevox.Escape},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Read numbers as:")},
		},
		{
			[]string{chromevox.NextObject},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Words")},
		},
		{
			[]string{chromevox.Activate},
			[]tts.SpeechExpectation{tts.NewStringExpectation("has pop up")},
		},
		{
			[]string{chromevox.ArrowDown},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Digits")},
		},
		{
			[]string{chromevox.Activate, chromevox.CloseWindow},
			[]tts.SpeechExpectation{tts.NewStringExpectation("1 2 3")},
		},
	}

	for _, step := range testSteps {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.KeyCommands, step.Expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
