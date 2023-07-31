// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y/dictation"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
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
		Timeout:      3 * time.Minute,
		Attr:         []string{"group:mainline", "informational"},
		// Load audio file used for Dictation.
		Data:         []string{"voice_en_hello.wav"},
		HardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.Microphone(), hwdep.Keyboard()),
		SoftwareDeps: []string{"chrome", "ondevice_speech"},
		Params: []testing.Param{{
			Name: "ash_textarea",
			Val: dictationTestParam{
				browserType: browser.TypeAsh,
				html:        "<textarea class='myEditable'></textarea>",
			},
		}, {
			Name:              "lacros_textarea",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: dictationTestParam{
				browserType: browser.TypeLacros,
				html:        "<textarea class='myEditable'></textarea>",
			},
		}, {
			Name: "ash_input",
			Val: dictationTestParam{
				browserType: browser.TypeAsh,
				html:        "<input class='myEditable'></input>",
			},
		}, {
			Name:              "lacros_input",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: dictationTestParam{
				browserType: browser.TypeLacros,
				html:        "<input class='myEditable'></input>",
			},
		}, {
			Name: "ash_contenteditable",
			Val: dictationTestParam{
				browserType: browser.TypeAsh,
				html:        "<div class='myEditable' contenteditable></div>",
			},
		}, {
			Name:              "lacros_contenteditable",
			ExtraSoftwareDeps: []string{"lacros"},
			Val: dictationTestParam{
				browserType: browser.TypeLacros,
				html:        "<div class='myEditable' contenteditable></div>",
			},
		}},
	})
}

func Dictation(ctx context.Context, s *testing.State) {
	bt := s.Param().(dictationTestParam).browserType
	html := s.Param().(dictationTestParam).html
	const className = "myEditable"
	driver, err := dictation.SetUp(ctx, html, className, bt)
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
