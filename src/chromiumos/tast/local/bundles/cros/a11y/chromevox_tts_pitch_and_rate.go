// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"

	"chromiumos/tast/local/a11y"
	"chromiumos/tast/local/a11y/chromevox"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/testing"
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

	vd := a11y.GoogleTTSEnUsVoice()
	ed := a11y.GoogleTTSEngine()
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
		expectations []a11y.SpeechExpectation
	}{
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("hi", lang, 1.0, 1.0)},
		},
		// Pitch is lowered for announcements.
		{
			[]string{increasePitch, increasePitch},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("Pitch 50 percent", lang, .85, 1.0),
				a11y.NewOptionsExpectation("Pitch 56 percent", lang, .95, 1.0)},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("high", lang, 1.2, 1.0)},
		},
		{
			[]string{decreasePitch, decreasePitch},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("Pitch 50 percent", lang, .85, 1.0),
				a11y.NewOptionsExpectation("Pitch 44 percent", lang, .75, 1.0)},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("normal", lang, 1, 1.0)},
		},
		{
			[]string{decreasePitch},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("Pitch 39 percent", lang, .65, 1.0)},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("low", lang, .9, 1.0)},
		},
		{
			[]string{increaseRate, increaseRate},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("Rate 19 percent", lang, .65, 1.1),
				a11y.NewOptionsExpectation("Rate 21 percent", lang, .65, 1.2)},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("fast", lang, .9, 1.2)},
		},
		{
			[]string{decreaseRate, decreaseRate},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("Rate 19 percent", lang, .65, 1.1),
				a11y.NewOptionsExpectation("Rate 17 percent", lang, .65, 1)},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("normal", lang, .9, 1)},
		},
		{
			[]string{decreaseRate},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("Rate 15 percent", lang, .65, .9)},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("slow", lang, .9, .9)},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("Text area", lang, .7, .9)},
		},
		{
			[]string{"c", "a", "t"},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("C", lang, .9, .9),
				a11y.NewOptionsExpectation("A", lang, .9, .9),
				a11y.NewOptionsExpectation("T", lang, .9, .9)},
		},
		// Pitch is lowered to delete characters.
		{
			[]string{"Backspace"},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("T", lang, .3, .9)},
		},
		{
			[]string{"Backspace"},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("A", lang, .3, .9)},
		},
		{
			[]string{resetTtsSettings},
			[]a11y.SpeechExpectation{a11y.NewRegexExpectation("Reset text to speech settings*")},
		},
		{
			[]string{nextObject},
			[]a11y.SpeechExpectation{a11y.NewOptionsExpectation("goodbye", lang, 1, 1)},
		},
	}
	for _, step := range testSteps {
		if err := a11y.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.keyCommands, step.expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
