// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

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
		Func:         ChromevoxTTSPitchAndRate,
		LacrosStatus: testing.LacrosVariantUnneeded, // TODO(crbug.com/1159107): Test is disabled in continuous testing. Migrate when enabled.
		Desc:         "A test that verifies the way ChromeVox sets Text-to-Speech pitch and rate",
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

func ChromevoxTTSPitchAndRate(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	bt := browser.TypeAsh
	html := `<p>hi</p>
		<p>high</p>
		<p>normal</p>
		<p>low</p>
		<p>fast</p>
		<p>normal</p>
		<p>slow</p>
		<textarea value="text"></textarea>
		<p>goodbye</p>`

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

	const nextObject = "Search+Right"
	const increasePitch = "Search+]"
	const decreasePitch = "Search+Shift+]"
	const increaseRate = "Search+["
	const decreaseRate = "Search+Shift+["
	const resetTtsSettings = "Search+Shift+Ctrl+\\"
	const lang = "en-US"

	testSteps := []struct {
		keyCommands  []string
		expectations []tts.SpeechExpectation
	}{
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("hi", lang, 1.0, 1.0)},
		},
		// Pitch is lowered for announcements.
		{
			[]string{increasePitch, increasePitch},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("Pitch 50 percent", lang, .85, 1.0),
				tts.NewOptionsExpectation("Pitch 56 percent", lang, .95, 1.0)},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("high", lang, 1.2, 1.0)},
		},
		{
			[]string{decreasePitch, decreasePitch},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("Pitch 50 percent", lang, .85, 1.0),
				tts.NewOptionsExpectation("Pitch 44 percent", lang, .75, 1.0)},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("normal", lang, 1, 1.0)},
		},
		{
			[]string{decreasePitch},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("Pitch 39 percent", lang, .65, 1.0)},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("low", lang, .9, 1.0)},
		},
		{
			[]string{increaseRate, increaseRate},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("Rate 19 percent", lang, .65, 1.1),
				tts.NewOptionsExpectation("Rate 21 percent", lang, .65, 1.2)},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("fast", lang, .9, 1.2)},
		},
		{
			[]string{decreaseRate, decreaseRate},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("Rate 19 percent", lang, .65, 1.1),
				tts.NewOptionsExpectation("Rate 17 percent", lang, .65, 1)},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("normal", lang, .9, 1)},
		},
		{
			[]string{decreaseRate},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("Rate 15 percent", lang, .65, .9)},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("slow", lang, .9, .9)},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("Text area", lang, .7, .9)},
		},
		{
			[]string{"c", "a", "t"},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("C", lang, .9, .9),
				tts.NewOptionsExpectation("A", lang, .9, .9),
				tts.NewOptionsExpectation("T", lang, .9, .9)},
		},
		// Pitch is lowered to delete characters.
		{
			[]string{"Backspace"},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("T", lang, .3, .9)},
		},
		{
			[]string{"Backspace"},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("A", lang, .3, .9)},
		},
		{
			[]string{resetTtsSettings},
			[]tts.SpeechExpectation{tts.NewRegexExpectation("Reset text to speech settings*")},
		},
		{
			[]string{nextObject},
			[]tts.SpeechExpectation{tts.NewOptionsExpectation("goodbye", lang, 1, 1)},
		},
	}
	for _, step := range testSteps {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.keyCommands, step.expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
