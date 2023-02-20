// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	printinghelpers "chromiumos/tast/local/bundles/cros/policy/printing_helpers"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/restriction"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintingPINModes,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify behaviour of PrintingAllowedPinModes and PrintingPinDefault Policies",
		Data:         []string{"printing_pin_modes_printer_attributes.json"},
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
			"nedol@google.com", // Test author
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:mainline",
			"informational",
		},
		Timeout: 4 * time.Minute,
		// PIN printing is enabled only on enrolled devices, thus we use fixtures with enrollment.
		Params: []testing.Param{{
			Fixture: "virtualUSBPrinterModulesLoadedWithChromeEnrolledLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "virtualUSBPrinterModulesLoadedWithLacrosEnrolledLoggedIn",
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PrintingPinDefault{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PrintingAllowedPinModes{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Test PrintingPinDefault and PrintingAllowedPinModes Policies (COM_FOUND_CUJ9_TASK3_WF1).
				Value: "screenplay-af2592d2-c335-4a0b-8330-a8f494423e58",
			},
		},
	})
}

func fetchPINModesValuesFromPrintPreview(ctx context.Context, s *testing.State, tconn *chrome.TestConn) printinghelpers.SettingValues {
	enableUserPINCheckBox := nodewith.Role(role.CheckBox).Name("Enable user pin")
	ui := uiauto.New(tconn)
	nodeInfo, err := ui.Info(ctx, enableUserPINCheckBox)
	if err != nil {
		s.Fatal("Failed to check the state of 'Enable user pin' checkbox: ", err)
	}

	defaultEnabledUserPINValue := string(nodeInfo.Checked)

	var availableEnabledUserPINValue []string
	if nodeInfo.Restriction == restriction.None {
		// Both checkbox states are available if the checkbox is enabled.
		availableEnabledUserPINValue = []string{"false", "true"}
	} else {
		// Only the default checkbox value is available otherwise.
		availableEnabledUserPINValue = []string{defaultEnabledUserPINValue}
	}

	// Enter some valid pin so that it's possible to switch back to "Print as PDF" as a default destination.
	if defaultEnabledUserPINValue == "true" {
		// Find a keyboard input source.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			s.Fatal("Failed to get the keyboard: ", err)
		}
		defer kb.Close()

		pinTextField := nodewith.Role("textField").NameContaining("Enter 4 digit pin")
		if err := uiauto.Combine("set pin",
			ui.DoDefault(pinTextField),
			ui.EnsureFocused(pinTextField),
			kb.TypeAction("1234"),
		)(ctx); err != nil {
			s.Fatal("Failed to type sample pin: ", err)
		}
	}

	return printinghelpers.SettingValues{
		DefaultValue:    defaultEnabledUserPINValue,
		AvailableValues: availableEnabledUserPINValue,
	}

}

func PrintingPINModes(ctx context.Context, s *testing.State) {
	subtestcases := []printinghelpers.SubTestCase{
		{
			TestName:                "default_pin_allowed_pin_x1",
			ExpectedDefaultValue:    "true",
			ExpectedAvailableValues: []string{"true"},
			Policies: []policy.Policy{
				&policy.PrintingPinDefault{Val: "pin"},
				&policy.PrintingAllowedPinModes{Val: "pin"},
			},
		},
		{
			TestName:                "default_unset_allowed_unset",
			ExpectedDefaultValue:    "false",
			ExpectedAvailableValues: []string{"false", "true"},
			Policies: []policy.Policy{
				&policy.PrintingPinDefault{Stat: policy.StatusUnset},
				&policy.PrintingAllowedPinModes{Stat: policy.StatusUnset},
			},
		},
		{
			TestName:                "default_pin_allowed_unset",
			ExpectedDefaultValue:    "true",
			ExpectedAvailableValues: []string{"false", "true"},
			Policies: []policy.Policy{
				&policy.PrintingPinDefault{Val: "pin"},
				&policy.PrintingAllowedPinModes{Stat: policy.StatusUnset},
			},
		},
		{
			TestName:                "default_unset_allowed_any",
			ExpectedDefaultValue:    "false",
			ExpectedAvailableValues: []string{"false", "true"},
			Policies: []policy.Policy{
				&policy.PrintingPinDefault{Stat: policy.StatusUnset},
				&policy.PrintingAllowedPinModes{Val: "any"},
			},
		},
		{
			TestName:                "default_no_pin_allowed_any",
			ExpectedDefaultValue:    "false",
			ExpectedAvailableValues: []string{"false", "true"},
			Policies: []policy.Policy{
				&policy.PrintingPinDefault{Val: "no_pin"},
				&policy.PrintingAllowedPinModes{Val: "any"},
			},
		},
		{
			TestName:                "default_pin_allowed_pin",
			ExpectedDefaultValue:    "true",
			ExpectedAvailableValues: []string{"true"},
			Policies: []policy.Policy{
				&policy.PrintingPinDefault{Val: "pin"},
				&policy.PrintingAllowedPinModes{Val: "pin"},
			},
		},
		{
			TestName:                "default_no_pin_allowed_pin",
			ExpectedDefaultValue:    "true",
			ExpectedAvailableValues: []string{"true"},
			Policies: []policy.Policy{
				&policy.PrintingPinDefault{Val: "no_pin"},
				&policy.PrintingAllowedPinModes{Val: "pin"},
			},
		},
	}

	printerAttributesFilePath := s.DataPath("printing_pin_modes_printer_attributes.json")
	printinghelpers.RunFeatureRestrictionTest(
		ctx, s, subtestcases, &printerAttributesFilePath, fetchPINModesValuesFromPrintPreview)
}
