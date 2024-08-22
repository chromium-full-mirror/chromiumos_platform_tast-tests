// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ChromevoxRichTextEditing,
		Desc: "A test that verifies the way ChromeVox can be used to edit text in a contenteditable",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"katie@chromium.org",           // Test author
		},
		BugComponent: "b:1272895",
		// TODO(https://crbug.com/1159107): Investigate failures and re-enable this test.
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromePolicyLoggedIn",
	})
}

func ChromevoxRichTextEditing(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	html := `<div role="textbox" contenteditable>
<h2>hello</h2>
<div><br></div>
<p>This is a <a href="%23test">test</a> of rich text</p>
</div>
<div role="textbox" contenteditable>
<p style="font-size:20px; font-family:times"><b style="color:%23ff0000">Move</b>
<i>through</i> <u style="font-family:georgia">text</u> by <strike style="font-size:12px; color:%230000ff">character</strike>
<a href="%23">test</a>!</p>
</div>
<div contenteditable="true" role="textbox">
<p>Start</p>
<span>I </span><span role="suggestion" aria-description="Username">
<span role="insertion">was</span>
<span role="deletion">am</span></span><span> typing</span>
<p>End</p></div>`
	vd := tts.GoogleTTSEnUsVoice()
	ed := tts.GoogleTTSEngine()
	cvData, err := chromevox.SetUp(ctx, cr, vd, ed, html)
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
	const formatPitch = 0.75
	const rolePitch = 0.7
	const classPitch = 0.8

	testSteps := []struct {
		keyCommands  []string
		expectations []tts.SpeechExpectation
	}{
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("hello", lang, 1.0, 1.0)},
		},
		{
			[]string{"Down", "Down"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("new line"),
				tts.NewOptionsExpectation("This is a", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("test", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("Link", lang, rolePitch, 1.0),
				tts.NewOptionsExpectation("of rich text", lang, 1.0, 1.0)},
		},
		{
			[]string{"Up", "Up"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("new line"),
				tts.NewStringExpectation("hello"),
				tts.NewOptionsExpectation("Heading 2", lang, classPitch, 1.0)},
		},
		{
			[]string{"Tab"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("Move through text by character test!"),
				tts.NewOptionsExpectation("Text area", lang, classPitch, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("O"),
				tts.NewOptionsExpectation("Red, 100% opacity.", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Bold", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Font Tinos", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right", "Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("V"),
				tts.NewStringExpectation("E")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("space"),
				tts.NewOptionsExpectation("Black, 100% opacity.", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Not bold", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("T"),
				tts.NewOptionsExpectation("Italic", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right", "Right", "Right", "Right", "Right", "Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("H"),
				tts.NewStringExpectation("R"),
				tts.NewStringExpectation("O"),
				tts.NewStringExpectation("U"),
				tts.NewStringExpectation("G"),
				tts.NewStringExpectation("H")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("space"),
				tts.NewOptionsExpectation("Not italic", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("T"),
				tts.NewOptionsExpectation("Underline", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Font Georgia", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right", "Right", "Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("E"),
				tts.NewStringExpectation("X"),
				tts.NewStringExpectation("T")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("space"),
				tts.NewOptionsExpectation("Not underline", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Font Tinos", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right", "Right", "Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("B"),
				tts.NewStringExpectation("Y"),
				tts.NewStringExpectation("space")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("C"),
				tts.NewOptionsExpectation("Blue, 100% opacity.", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Line through", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right", "Right", "Right", "Right", "Right", "Right", "Right", "Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("H"),
				tts.NewStringExpectation("A"),
				tts.NewStringExpectation("R"),
				tts.NewStringExpectation("A"),
				tts.NewStringExpectation("C"),
				tts.NewStringExpectation("T"),
				tts.NewStringExpectation("E"),
				tts.NewStringExpectation("R")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("space"),
				tts.NewOptionsExpectation("Not line through", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("Link", lang, rolePitch, 1.0),
				tts.NewStringExpectation("T"),
				tts.NewOptionsExpectation("Blue, 100% opacity.", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Link", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Underline", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right", "Right", "Right"},
			[]tts.SpeechExpectation{
				tts.NewStringExpectation("E"),
				tts.NewStringExpectation("S"),
				tts.NewStringExpectation("T")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("exclamation", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("Black, 100% opacity.", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Not link", lang, formatPitch, 1.0),
				tts.NewOptionsExpectation("Not underline", lang, formatPitch, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("End of text", lang, 1.0, 1.0)},
		},
		{
			[]string{"Tab", "Down"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Start")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("space", lang, 1.0, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("Suggest", lang, rolePitch, 1.0),
				tts.NewOptionsExpectation("Username", lang, 1.0, 1.0),
				tts.NewOptionsExpectation("Insert", lang, rolePitch, 1.0),
				tts.NewOptionsExpectation("W", lang, 1.0, 1.0)},
		},
		{
			[]string{"Right", "Right"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("A"), tts.NewStringExpectation("S")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("Exited Insert.", lang, classPitch, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("Delete", lang, rolePitch, 1.0),
				tts.NewOptionsExpectation("A", lang, 1.0, 1.0)},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("M")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{
				tts.NewOptionsExpectation("Exited Delete.", lang, classPitch, 1.0),
				tts.NewOptionsExpectation("Exited Suggest.", lang, classPitch, 1.0)},
		},
	}

	for _, step := range testSteps {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.keyCommands, step.expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
