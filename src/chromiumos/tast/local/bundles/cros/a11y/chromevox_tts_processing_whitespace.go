// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	bt := s.Param().(browser.Type)
	html := "<textarea></textarea>"
	vd := a11y.VoiceData{
		ExtID:  a11y.GoogleTTSExtensionID,
		Locale: "en-US",
	}
	ed := a11y.TTSEngineData{
		ExtID:                     a11y.GoogleTTSExtensionID,
		UseOnSpeakWithAudioStream: false,
	}
	cvData, err := chromevox.SetUpChromeVox(ctx, cleanupCtx, cr, vd, ed, bt, html)
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
		expectations []a11y.SpeechExpectation
	}{
		{
			[]string{"Search+Right"},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("Text area")},
		},
		{
			[]string{"Space"},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("space")},
		},
		{
			[]string{"Enter"},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("new line")},
		},
		{
			[]string{"Backspace", "Backspace"},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation(", deleted", "en-US", .4, 1.0)},
		},
	}

	for _, step := range testSteps {
		if err := a11y.PressKeysAndConsumeExpectations(ctx, cvData.SM, step.keyCommands, step.expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
