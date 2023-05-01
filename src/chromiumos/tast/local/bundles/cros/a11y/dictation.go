// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"

	"chromiumos/tast/local/a11y/dictation"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type dictationTestParam struct {
	browserType browser.Type
	html        string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Dictation,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that the Dictation feature can be used to input text using voice",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1272896",
		Attr:         []string{"group:mainline", "informational"},
		// Load audio file used for Dictation.
		Data:         []string{"voice_en_hello.wav"},
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name:    "textarea",
			Fixture: "chromeLoggedIn",
			Val: dictationTestParam{
				browserType: browser.TypeAsh,
				html:        "<textarea class='myEditable'></textarea>",
			},
		}, {
			Name:              "lacros_textarea",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: dictationTestParam{
				browserType: browser.TypeLacros,
				html:        "<textarea class='myEditable'></textarea>",
			},
		}, {
			Name:    "input",
			Fixture: "chromeLoggedIn",
			Val: dictationTestParam{
				browserType: browser.TypeAsh,
				html:        "<input class='myEditable'></input>",
			},
		}, {
			Name:              "lacros_input",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: dictationTestParam{
				browserType: browser.TypeLacros,
				html:        "<input class='myEditable'></input>",
			},
		}, {
			Name:    "contenteditable",
			Fixture: "chromeLoggedIn",
			Val: dictationTestParam{
				browserType: browser.TypeAsh,
				html:        "<div class='myEditable' contenteditable></div>",
			},
		}, {
			Name:              "lacros_contenteditable",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: dictationTestParam{
				browserType: browser.TypeLacros,
				html:        "<div class='myEditable' contenteditable></div>",
			},
		}},
	})
}

func Dictation(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	bt := s.Param().(dictationTestParam).browserType
	html := s.Param().(dictationTestParam).html
	const className = "myEditable"
	driver, err := dictation.SetUp(ctx, cr, html, className, bt)
	if err != nil {
		s.Fatal("Failed to set up Dictation: ", err)
	}

	defer func() {
		if err := driver.TearDown(); err != nil {
			s.Fatal("Failed to tear down Dictation test: ", err)
		}
	}()

	if err := driver.ToggleOn(); err != nil {
		s.Fatal("Failed to toggle Dictation on: ", err)
	}

	audioFile := s.DataPath("voice_en_hello.wav")
	if err := driver.DictateAndWaitForEditableValue(audioFile, "Hello"); err != nil {
		s.Fatal("Failed to dictate and verify editable value: ", err)
	}

	if err := driver.ToggleOff(); err != nil {
		s.Fatal("Failed to toggle Dictation off: ", err)
	}
}
