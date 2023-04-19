// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package a11y provides functions to assist with interacting with accessibility
// features and settings.
package a11y

import (
	"context"

	"chromiumos/tast/local/a11y/dictation"
	"chromiumos/tast/local/policyutil/fixtures"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Dictation,
		LacrosStatus: testing.LacrosVariantUnneeded, // TODO(crbug.com/1159107): Test is disabled in continuous testing. Migrate when enabled.
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
		Fixture:      "chromePolicyLoggedIn",
	})
}

func Dictation(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*fixtures.FixtData).Chrome()
	const html = "<textarea class='myTextArea'></textarea>"
	const className = "myTextArea"
	driver, err := dictation.SetUp(ctx, cr, html, className)
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
