// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"chromiumos/tast/common/pci"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/local/bundles/cros/policy/printingtest"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/printpreview"
	"chromiumos/tast/local/chrome/uiauto/restriction"
	"chromiumos/tast/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintingBackgroundGraphicsModes,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify behaviour of PrintingAllowedBackgroundGraphicsModes and PrintingBackgroundGraphicsDefault Policies",
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
		},
		Params: []testing.Param{{
			Fixture: "virtualUsbPrinterModulesLoadedWithChromePolicyLoggedIn",
			Val:     browser.TypeAsh,
			Timeout: 4 * time.Minute,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "virtualUsbPrinterModulesLoadedWithLacrosPolicyLoggedIn",
			Val:               browser.TypeLacros,
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PrintingBackgroundGraphicsDefault{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PrintingAllowedBackgroundGraphicsModes{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Test PrintingBackgroundGraphicsDefault and PrintingAllowedBackgroundGraphicsModes Policies (COM_FOUND_CUJ9_TASK3_WF1).
				Value: "screenplay-af2592d2-c335-4a0b-8330-a8f494423e58",
			},
		},
	})
}

func fetchBackgroundGraphicsValuesFromPrintPreview(ctx context.Context, s *testing.State, tconn *chrome.TestConn) printingtest.SettingValues {
	ui := uiauto.New(tconn)
	if err := printpreview.ExpandMoreSettings(ctx, tconn); err != nil {
		s.Error("Failed to expand more settings: ", err)
	}

	backgroundGraphicsCheckbox := nodewith.Role(role.CheckBox).Name("Background graphics")
	nodeInfo, err := ui.Info(ctx, backgroundGraphicsCheckbox)
	if err != nil {
		s.Error("Failed to check the state of 'Background Graphics' checkbox: ", err)
	}

	defaultBackgroundGraphicsValue := string(nodeInfo.Checked)

	var availableBackgroundGraphicsValue []string
	if nodeInfo.Restriction == restriction.None {
		// Both checkbox states are available if the checkbox is enabled
		availableBackgroundGraphicsValue = []string{"false", "true"}
	} else {
		// Only the default checkbox value is available otherwise
		availableBackgroundGraphicsValue = []string{defaultBackgroundGraphicsValue}
	}

	return printingtest.SettingValues{
		DefaultValue:    defaultBackgroundGraphicsValue,
		AvailableValues: availableBackgroundGraphicsValue,
	}
}

func PrintingBackgroundGraphicsModes(ctx context.Context, s *testing.State) {
	subtestcases := []printingtest.SubTestCase{
		{
			TestName:                "default_unset_allowed_unset",
			ExpectedDefaultValue:    "false",
			ExpectedAvailableValues: []string{"false", "true"},
			Policies: []policy.Policy{
				&policy.PrintingBackgroundGraphicsDefault{Stat: policy.StatusUnset},
				&policy.PrintingAllowedBackgroundGraphicsModes{Stat: policy.StatusUnset},
			},
			FetchValuesFunc: fetchBackgroundGraphicsValuesFromPrintPreview,
		},
		{
			TestName:                "default_unset_allowed_any",
			ExpectedDefaultValue:    "false",
			ExpectedAvailableValues: []string{"false", "true"},
			Policies: []policy.Policy{
				&policy.PrintingBackgroundGraphicsDefault{Stat: policy.StatusUnset},
				&policy.PrintingAllowedBackgroundGraphicsModes{Val: "any"},
			},
			FetchValuesFunc: fetchBackgroundGraphicsValuesFromPrintPreview,
		},
		{
			TestName:                "default_unset_allowed_enabled",
			ExpectedDefaultValue:    "true",
			ExpectedAvailableValues: []string{"true"},
			Policies: []policy.Policy{
				&policy.PrintingBackgroundGraphicsDefault{Stat: policy.StatusUnset},
				&policy.PrintingAllowedBackgroundGraphicsModes{Val: "enabled"},
			},
			FetchValuesFunc: fetchBackgroundGraphicsValuesFromPrintPreview,
		},
		{
			TestName:                "default_enabled_allowed_unset",
			ExpectedDefaultValue:    "true",
			ExpectedAvailableValues: []string{"false", "true"},
			Policies: []policy.Policy{
				&policy.PrintingBackgroundGraphicsDefault{Val: "enabled"},
				&policy.PrintingAllowedBackgroundGraphicsModes{Stat: policy.StatusUnset},
			},
			FetchValuesFunc: fetchBackgroundGraphicsValuesFromPrintPreview,
		},
		{
			TestName:                "default_enabled_allowed_enabled",
			ExpectedDefaultValue:    "true",
			ExpectedAvailableValues: []string{"true"},
			Policies: []policy.Policy{
				&policy.PrintingBackgroundGraphicsDefault{Val: "enabled"},
				&policy.PrintingAllowedBackgroundGraphicsModes{Val: "enabled"},
			},
			FetchValuesFunc: fetchBackgroundGraphicsValuesFromPrintPreview,
		},
		{
			TestName:                "default_enabled_allowed_disabled",
			ExpectedDefaultValue:    "false",
			ExpectedAvailableValues: []string{"false"},
			Policies: []policy.Policy{
				&policy.PrintingBackgroundGraphicsDefault{Val: "enabled"},
				&policy.PrintingAllowedBackgroundGraphicsModes{Val: "disabled"},
			},
			FetchValuesFunc: fetchBackgroundGraphicsValuesFromPrintPreview,
		},
	}

	printingtest.RunFeatureRestrictionTest(ctx, s, subtestcases, nil /* printerAttributesFilePath */)
}
