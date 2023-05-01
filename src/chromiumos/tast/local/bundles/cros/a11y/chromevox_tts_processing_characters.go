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
		Func:         ChromevoxTTSProcessingCharacters,
		LacrosStatus: testing.LacrosVariantUnneeded, // TODO(crbug.com/1159107): Test is disabled in continuous testing. Migrate when enabled.
		Desc:         "A test that verifies the way ChromeVox processes some characters for speech",
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

func ChromevoxTTSProcessingCharacters(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	bt := browser.TypeAsh
	html := `<p>.</p>
		<p>x.</p>
		<p>=========</p>
		<p>&bull; &bull;&bull;</p>
		<p>&bull;&bull;&bull;</p>
		<p>C++</p><p>C+++</p>
		<p>&pound; and %23 symbol</p>
		<p>&pound;&pound;&pound;</p>
		<p>C--</p>
		<p>x, y.</p>
		<p>"That's all, folks!"</p>
		<p>"$1,234.56 (plus tax) for 78&percnt; of your %232 pencils?", they mused</p>
		<p>x, y.</p>
		<p>"That's all, folks!"</p>
		<p>"$1,234.56 (plus tax) for 78&percnt; of your %232 pencils?", they mused</p>
		<p>x, y.</p>
		<p>"That's all, folks!"</p>
		<p>"$1,234.56 (plus tax) for 78&percnt; of your %232 pencils?", they mused</p>`
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

	nextObject := []string{"Search+Right"}
	cyclePunctuationMode := []string{"Search+A", "P"}

	testSteps := []struct {
		keyCommands  []string
		expectations []tts.SpeechExpectation
	}{
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("dot")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("x.")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("9 equal signs")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("bullet bullet bullet")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("3 bullets")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("C plus plus")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("C 3 plus signs")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("pound sterling and pound symbol")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("pound sterling pound sterling pound sterling")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("C--")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("x, y.")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("quote That's all, folks! quote")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("quote dollar 1,234.56 (plus tax) for 78 percent of your pound 2 pencils? quote , they mused")},
		},
		{
			cyclePunctuationMode,
			[]tts.SpeechExpectation{tts.NewStringExpectation("All punctuation")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("x comma y dot")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("quote That apostrophe' s all comma folks exclamation! quote")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("quote dollar 1 comma 234 dot 56 open paren plus tax close paren for 78 percent of your pound 2 pencils question mark? quote comma they mused")},
		},
		{
			cyclePunctuationMode,
			[]tts.SpeechExpectation{tts.NewStringExpectation("No punctuation")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("x, y.")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("That's all, folks!")},
		},
		{
			nextObject,
			[]tts.SpeechExpectation{tts.NewStringExpectation("1,234.56 plus tax for 78% of your 2 pencils? , they mused")},
		},
		{
			cyclePunctuationMode,
			[]tts.SpeechExpectation{tts.NewStringExpectation("Some punctuation")},
		},
	}
	for _, step := range testSteps {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.keyCommands, step.expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
