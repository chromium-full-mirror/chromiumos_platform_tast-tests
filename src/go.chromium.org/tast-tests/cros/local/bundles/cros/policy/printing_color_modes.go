// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/printingtest"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/dropdown"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintingColorModes,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify behaviour of PrintingAllowedColorModes and PrintingColorDefault Policies",
		Data:         []string{"printing_color_modes_printer_attributes.json"},
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
			"nedol@google.com", // Test author
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Params: []testing.Param{{
			Fixture: "virtualUsbPrinterModulesLoadedWithChromePolicyLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "virtualUsbPrinterModulesLoadedWithLacrosPolicyLoggedIn",
			Val:               browser.TypeLacros,
			Timeout:           4 * time.Minute,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PrintingColorDefault{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PrintingAllowedColorModes{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Test PrintingAllowedColorModes and PrintingColorDefault Policies (COM_FOUND_CUJ9_TASK3_WF1).
				Value: "screenplay-af2592d2-c335-4a0b-8330-a8f494423e58",
			},
		},
	})
}

// fetchColorModesFromPrintPreview assumes that the print preview dialog is open
func fetchColorModesFromPrintPreview(ctx context.Context, s *testing.State, tconn *chrome.TestConn) printingtest.SettingValues {
	ui := uiauto.New(tconn)
	colorSelection := nodewith.Role(role.ComboBoxSelect).Name("Color")
	nodeInfo, err := ui.Info(ctx, colorSelection)
	if err != nil {
		s.Fatal("Failed to check the state of 'Color' selection: ", err)
	}

	defaultColor := nodeInfo.Value

	availableColors := make([]string, 0)
	if nodeInfo.Restriction == restriction.Disabled {
		// Color selection box is disabled, thus the only mode available is the one selected.
		availableColors = append(availableColors, nodeInfo.Value)
	} else if nodeInfo.Restriction == restriction.None {
		if availableColors, err = dropdown.Values(ctx, tconn, colorSelection); err != nil {
			s.Fatal("Failed to fetch available color modes: ", err)
		}
	}

	return printingtest.SettingValues{
		DefaultValue:    defaultColor,
		AvailableValues: availableColors,
	}
}

func PrintingColorModes(ctx context.Context, s *testing.State) {
	subtestcases := []printingtest.SubTestCase{
		{
			TestName:                "default_color_unset_all_colors_allowed",
			ExpectedDefaultValue:    "Color",
			ExpectedAvailableValues: []string{"Color", "Black and white"},
			Policies: []policy.Policy{
				&policy.PrintingColorDefault{Stat: policy.StatusUnset},
				&policy.PrintingAllowedColorModes{Val: "any"},
			},
			FetchValuesFunc: fetchColorModesFromPrintPreview,
		}, {
			TestName:                "default_color_monochrome_allowed_colors_unset",
			ExpectedDefaultValue:    "Black and white",
			ExpectedAvailableValues: []string{"Color", "Black and white"},
			Policies: []policy.Policy{
				&policy.PrintingColorDefault{Val: "monochrome"},
				&policy.PrintingAllowedColorModes{Stat: policy.StatusUnset},
			},
			FetchValuesFunc: fetchColorModesFromPrintPreview,
		}, {
			TestName:                "default_color_color_all_colors_allowed",
			ExpectedDefaultValue:    "Color",
			ExpectedAvailableValues: []string{"Color", "Black and white"},
			Policies: []policy.Policy{
				&policy.PrintingColorDefault{Val: "color"},
				&policy.PrintingAllowedColorModes{Val: "any"},
			},
			FetchValuesFunc: fetchColorModesFromPrintPreview,
		}, {
			TestName:                "default_color_monochrome_is_allowed",
			ExpectedDefaultValue:    "Black and white",
			ExpectedAvailableValues: []string{"Black and white"},
			Policies: []policy.Policy{
				&policy.PrintingColorDefault{Val: "monochrome"},
				&policy.PrintingAllowedColorModes{Val: "monochrome"},
			},
			FetchValuesFunc: fetchColorModesFromPrintPreview,
		}, {
			TestName:                "default_color_monochrome_is_not_allowed",
			ExpectedDefaultValue:    "Color",
			ExpectedAvailableValues: []string{"Color"},
			Policies: []policy.Policy{
				&policy.PrintingColorDefault{Val: "monochrome"},
				&policy.PrintingAllowedColorModes{Val: "color"},
			},
			FetchValuesFunc: fetchColorModesFromPrintPreview,
		},
	}

	printerAttributesFilePath := s.DataPath("printing_color_modes_printer_attributes.json")
	printingtest.RunFeatureRestrictionTest(ctx, s, subtestcases, &printerAttributesFilePath)
}
