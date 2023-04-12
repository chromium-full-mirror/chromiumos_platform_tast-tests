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
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromevoxPlainTextEditing,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "A test that verifies the way ChromeVox can be used to edit text in plain text fields",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"katie@chromium.org",           // Test author
		},
		BugComponent: "b:1272895", // ChromeOS Public Tracker > Experiences > Accessibility > Features > ChromeVox
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

func ChromevoxPlainTextEditing(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	vd := tts.GoogleTTSEnUsVoice()
	ed := tts.GoogleTTSEngine()
	bt := s.Param().(browser.Type)
	const html = `<label for='singleLine'>singleLine</label>
<input type='text' id='singleLine' value='Single line field'><br>
<label for='textarea'>textArea</label>
<textarea id='textarea'>Line 1
line 2
line 3</textarea>`
	cvData, err := chromevox.SetUp(ctx, cr, vd, ed, bt, html)
	if err != nil {
		s.Fatal("Failed to set up ChromeVox: ", err)
	}
	defer func() {
		if err := cvData.TearDown(); err != nil {
			s.Fatal("Failed to tear down ChromeVox test: ", err)
		}
	}()

	const nextObject = "Search+Right"
	const lang = "en-US"

	testSteps := []struct {
		keyCommands  []string
		expectations []tts.SpeechExpectation
	}{
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("singleLine", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("Single line field", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("Edit text", lang, 1.0, 1.0)},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("textArea", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("Line 1 line 2 line 3", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("Text area", lang, 0.8, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("I")},
		},
		{
			[]string{"Shift+Right"},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("I", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("selected", lang, 1.0, 1.0)},
		},
		{
			[]string{"Down"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("line 2")},
		},
		{
			[]string{"Left"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("L")},
		},
		{
			[]string{"Shift+Ctrl+Right"},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("line", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("selected", lang, 1.0, 1.0)},
		},
		{
			[]string{"Shift+Ctrl+Right"},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("2", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("added to selection", lang, 1.0, 1.0)},
		},
	}

	for _, step := range testSteps {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.keyCommands, step.expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
