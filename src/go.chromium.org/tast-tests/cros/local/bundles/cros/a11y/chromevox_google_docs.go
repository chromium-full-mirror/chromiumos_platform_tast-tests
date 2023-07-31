// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/chromevox"
	"go.chromium.org/tast-tests/cros/local/a11y/tts"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromevoxGoogleDocs,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verifies turning on ChromeVox enables screen reader support in Google Docs and ChromeVox can read contents of a Google Doc",
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
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "lacros",
			Val:               browser.TypeLacros,
		}},
	})
}

func ChromevoxGoogleDocs(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	vd := tts.GoogleTTSEnUsVoice()
	ed := tts.GoogleTTSEngine()
	bt := s.Param().(browser.Type)
	url := "https://docs.google.com/document/d/1qpu3koSIHpBzQbxeEE-dofSKXCIgdc4yJLI-o1LpCPs/view"
	cvData, err := chromevox.SetUpWithURLWithoutFocusWaiter(ctx, cr, vd, ed, bt, url)
	if err != nil {
		s.Fatal("Failed to set up ChromeVox: ", err)
	}
	defer func() {
		if err := cvData.TearDown(); err != nil {
			s.Fatal("Failed to tear down ChromeVox test: ", err)
		}
	}()

	ui := uiauto.New(cvData.TTSData.TConn).WithTimeout(10 * time.Second)
	docsA11yMenu := nodewith.Name("Accessibility").Role(role.MenuItem)
	if err := ui.WaitUntilExists(docsA11yMenu)(ctx); err != nil {
		s.Fatal("Failed to wait for the accessibility menu to be shown: ", err)
	}

	testSteps := []struct {
		KeyCommands  []string
		Expectations []tts.SpeechExpectation
	}{
		{
			[]string{"Esc", chromevox.NextObject},
			[]tts.SpeechExpectation{tts.NewRegexExpectation("Screen reader support enabled.*")},
		},
		{
			[]string{"Esc"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("Document content")},
		},
		{
			[]string{chromevox.Activate, "Right"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("L")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("O")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("N")},
		},
		{
			[]string{"Right"},
			[]tts.SpeechExpectation{tts.NewStringExpectation("G")},
		},
	}

	for _, step := range testSteps {
		if err := tts.PressKeysAndConsumeExpectations(cvData.Context(), cvData.SpeechMonitor(), step.KeyCommands, step.Expectations); err != nil {
			s.Error("Error when pressing keys and expecting speech: ", err)
		}
	}
}
