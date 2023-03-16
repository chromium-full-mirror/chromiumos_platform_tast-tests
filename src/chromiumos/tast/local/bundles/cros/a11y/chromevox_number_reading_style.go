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

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromevoxNumberReadingStyle,
		LacrosStatus: testing.LacrosVariantNeeded, // TODO(crbug.com/1358282): Migrate when Chromevox options page opens in Lacros.
		Desc:         "Verifies ChromeVox honors its setting to read numbers as words or as digits",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"josiahk@chromium.org",         // Test author
		},
		BugComponent: "b:1272895",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromePolicyLoggedIn",
	})
}

func ChromevoxNumberReadingStyle(ctx context.Context, s *testing.State) {
	ctxCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	vd := a11y.VoiceData{
		ExtID:  a11y.GoogleTTSExtensionID,
		Locale: "en-US",
	}
	ed := a11y.TTSEngineData{
		ExtID:                     a11y.GoogleTTSExtensionID,
		UseOnSpeakWithAudioStream: false,
	}
	bt := browser.TypeAsh
	html := "<p>123</p>"
	cvData, err := chromevox.SetUpChromeVox(ctx, ctxCleanup, cr, vd, ed, bt, html)
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
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("123")},
		},
		{
			chromevox.OpenOptionsPage,
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("ChromeVox Options")},
		},
		{
			[]string{chromevox.Find},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("Find")},
		},
		{
			[]string{"R", "E", "A", "D", chromevox.Space, "N", "U", "M", "B", "E", "R", "S", chromevox.Escape},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("Read numbers as:")},
		},
		{
			[]string{chromevox.NextObject},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("Words")},
		},
		{
			[]string{chromevox.Activate},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("has pop up")},
		},
		{
			[]string{chromevox.ArrowDown},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("Digits")},
		},
		{
			[]string{chromevox.Activate, chromevox.PreviousTab, chromevox.NextObject, chromevox.NextObject},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("1 2 3")},
		},
	}

	for _, step := range testSteps {
		if err := a11y.PressKeysAndConsumeExpectations(ctx, cvData.SM, step.KeyCommands, step.Expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
