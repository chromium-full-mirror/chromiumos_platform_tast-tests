// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/a11y/dictation"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DictationWithPumpkin,
		Desc: "Tests that the Dictation feature can use the Pumpkin semantic parser to input text",
		Contacts: []string{
			"chromeos-a11y-eng@google.com", // Mailing list
			"akihiroota@chromium.org",      // Test author
		},
		BugComponent: "b:1272896",
		Timeout:      3 * time.Minute,
		Attr:         []string{"group:mainline", "informational"},
		// Load audio file used for Dictation.
		Data:         []string{"voice_en_dictate_hello.wav"},
		HardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.Microphone(), hwdep.Keyboard()),
		SoftwareDeps: []string{"chrome", "ondevice_speech"},
		Fixture:      a11y.SodaDLCInstalled,
	})
}

func DictationWithPumpkin(ctx context.Context, s *testing.State) {
	html := "<textarea class='myEditable'></textarea>"
	const className = "myEditable"
	driver, err := dictation.SetUp(ctx, html, className)
	if err != nil {
		s.Fatal("Failed to set up Dictation: ", err)
	}

	defer func() {
		if err := driver.TearDown(); err != nil {
			s.Fatal("Failed to tear down Dictation test: ", err)
		}
	}()

	if err := driver.WaitForPumpkinTaggerReady(); err != nil {
		s.Fatal("Failed to wait for Pumpkin to setup: ", err)
	}

	if err := driver.ToggleOn(); err != nil {
		s.Fatal("Failed to toggle Dictation on: ", err)
	}

	// The audio file will play "Dictate hello" - if Pumpkin is working correctly,
	// Dictation should just enter "Hello" into the text field.
	audioFile := s.DataPath("voice_en_dictate_hello.wav")
	if err := driver.DictateAndWaitForEditableValue(audioFile, 2 /*audioDurationSec*/, "Hello"); err != nil {
		s.Fatal("Failed to dictate and verify editable value: ", err)
	}

	if err := driver.ToggleOff(); err != nil {
		s.Fatal("Failed to toggle Dictation off: ", err)
	}
}
