// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"

	"chromiumos/tast/local/a11y/chromevox"
	"chromiumos/tast/local/a11y/tts"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromevoxTTSProcessingWhitespace,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A test that verifies the way ChromeVox processes whitespace for speech in text areas",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"katie@chromium.org",           // Test author
		},
		BugComponent: "b:1272895",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

func ChromevoxTTSProcessingWhitespace(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	bt := s.Param().(browser.Type)
	html := "<textarea></textarea>"
	vd := tts.GoogleTTSEnUsVoice()
	ed := tts.GoogleTTSEngine()
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
		keyCommands  []string
		expectations []tts.SpeechExpectation
	}{
		{
			[]string{"Search+Right"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Text area")},
		},
		{
			[]string{"Space"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("space")},
		},
		{
			[]string{"Enter"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("new line")},
		},
		{
			[]string{"Backspace", "Backspace"},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation(", deleted", "en-US", .4, 1.0)},
		},
	}

	for _, step := range testSteps {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.keyCommands, step.expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
