// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package telemetryextension

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/telemetryextension/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlatformAPIEventsKeyboardDiagnostic,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests keyboard diagnostics part of the chrome.os.events Chrome Extension API exposed to Telemetry Extension",
		Contacts:     []string{"chromeos-oem-services@google.com"},
		// ChromeOS > Software > Commercial (Enterprise) > OEM Services.
		BugComponent: "b:1256717",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				Name:              "ash_keyboard",
				Fixture:           fixture.TelemetryExtensionSkipOEMNameCheck,
				ExtraHardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
				Val:               true, // has keyboard
			},
			{
				Name:              "ash_no_keyboard",
				Fixture:           fixture.TelemetryExtensionSkipOEMNameCheck,
				ExtraHardwareDeps: hwdep.D(hwdep.NoInternalKeyboard()),
				Val:               false, // has keyboard
			},
			{
				Name:              "lacros_keyboard",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.TelemetryExtensionSkipOEMNameCheckLacros,
				ExtraHardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
				Val:               true, // has keyboard
			},
			{
				Name:              "lacros_no_keyboard",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.TelemetryExtensionSkipOEMNameCheckLacros,
				ExtraHardwareDeps: hwdep.D(hwdep.NoInternalKeyboard()),
				Val:               false, // has keyboard
			},
		},
	})
}

// PlatformAPIEventsKeyboardDiagnostic tests that a call to
// chrome.os.events.startCapturingEvents(keyboard_diagnostic)
// opens the first party diagnostic application.
func PlatformAPIEventsKeyboardDiagnostic(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(*fixture.Value)

	if err := v.ExtConn.Call(ctx, nil,
		"tast.promisify(chrome.os.events.startCapturingEvents)", "keyboard_diagnostic"); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}

	// Check that the Diagnostics Application was opened.
	diagnosticsApp := nodewith.Name("Diagnostics").Role(role.Application)
	if err := uiauto.New(v.TConn).WaitUntilExists(diagnosticsApp)(ctx); err != nil {
		s.Fatal("Failed to load diagnostics app: ", err)
	}

	hasKeyboard, ok := s.Param().(bool)
	if !ok {
		s.Fatal("Unable to convert param")
	}

	if hasKeyboard {
		// Check that the keyboard page is shown.
		keyboardHeading := nodewith.NameContaining("Keyboard").Role(role.Heading)
		if err := uiauto.New(v.TConn).WaitUntilExists(keyboardHeading)(ctx); err != nil {
			s.Fatal("Failed to open on keyboard tab: ", err)
		}
	}

	// Stop observing to reset state.
	if err := v.ExtConn.Call(ctx, nil,
		"tast.promisify(chrome.os.events.stopCapturingEvents)", "keyboard_diagnostic"); err != nil {
		s.Fatal("Failed to get response from Telemetry extension service worker: ", err)
	}
}
