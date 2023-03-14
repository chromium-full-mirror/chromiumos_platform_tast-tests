// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/a11y"
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
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
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
	bt := s.Param().(browser.Type)
	const html = `<label for='singleLine'>singleLine</label>
<input type='text' id='singleLine' value='Single line field'><br>
<label for='textarea'>textArea</label>
<textarea id='textarea'>Line 1
line 2
line 3</textarea>`
	cvData, err := a11y.SetUpChromeVox(ctx, cleanupCtx, cr, vd, ed, bt, html)
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
		expectations []a11y.SpeechExpectation
	}{
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{
				a11y.NewOptionsExpectation("singleLine", lang, 1.0, 1.0),
				a11y.NewOptionsExpectation("Single line field", lang, 1.0, 1.0),
				a11y.NewOptionsExpectation("Edit text", lang, 1.0, 1.0)},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{
				a11y.NewOptionsExpectation("textArea", lang, 1.0, 1.0),
				a11y.NewOptionsExpectation("Line 1 line 2 line 3", lang, 1.0, 1.0),
				a11y.NewOptionsExpectation("Text area", lang, 0.8, 1.0)},
		},
		{
			[]string{"Right"},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("I")},
		},
		{
			[]string{"Shift+Right"},
			[]a11y.SpeechExpectation{
				a11y.NewOptionsExpectation("I", lang, 1.0, 1.0),
				a11y.NewOptionsExpectation("selected", lang, 1.0, 1.0)},
		},
		{
			[]string{"Down"},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("line 2")},
		},
		{
			[]string{"Left"},
			[]a11y.SpeechExpectation{a11y.NewStringExpectation("L")},
		},
		{
			[]string{"Shift+Ctrl+Right"},
			[]a11y.SpeechExpectation{
				a11y.NewOptionsExpectation("line", lang, 1.0, 1.0),
				a11y.NewOptionsExpectation("selected", lang, 1.0, 1.0)},
		},
		{
			[]string{"Shift+Ctrl+Right"},
			[]a11y.SpeechExpectation{
				a11y.NewOptionsExpectation("2", lang, 1.0, 1.0),
				a11y.NewOptionsExpectation("added to selection", lang, 1.0, 1.0)},
		},
	}

	for _, step := range testSteps {
		if err := a11y.PressKeysAndConsumeExpectations(ctx, cvData.SM, step.keyCommands, step.expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
