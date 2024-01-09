// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/dropdown"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrintingDuplexModes,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify behaviour of PrintingAllowedDuplexModes and PrintingDuplexDefault Policies",
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
		}},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PrintingAllowedDuplexModes{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.PrintingDuplexDefault{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Test PrintingAllowedDuplexModes and PrintingDuplexDefault Policies (COM_FOUND_CUJ9_TASK3_WF1).
				Value: "screenplay-af2592d2-c335-4a0b-8330-a8f494423e58",
			},
		},
		Timeout: 4 * time.Minute,
	})
}

func PrintingDuplexModes(ctx context.Context, s *testing.State) {
	subtestcases := []printingtest.SubTestCase{
		{
			TestName:                "default_unset_allowed_any",
			ExpectedDefaultValue:    "Simplex",
			ExpectedAvailableValues: []string{"Simplex", "Flip on long edge", "Flip on short edge"},
			Policies: []policy.Policy{
				&policy.PrintingAllowedDuplexModes{Val: "any"},
				&policy.PrintingDuplexDefault{Stat: policy.StatusUnset},
			},
			FetchValuesFunc: fetchDuplexModesFromPrintPreview,
		}, {
			TestName:                "default_unset_allowed_duplex",
			ExpectedDefaultValue:    "Flip on long edge",
			ExpectedAvailableValues: []string{"Flip on long edge", "Flip on short edge"},
			Policies: []policy.Policy{
				&policy.PrintingAllowedDuplexModes{Val: "duplex"},
				&policy.PrintingDuplexDefault{Stat: policy.StatusUnset},
			},
			FetchValuesFunc: fetchDuplexModesFromPrintPreview,
		}, {
			TestName:                "default_short_edge_allowed_any",
			ExpectedDefaultValue:    "Flip on short edge",
			ExpectedAvailableValues: []string{"Simplex", "Flip on long edge", "Flip on short edge"},
			Policies: []policy.Policy{
				&policy.PrintingAllowedDuplexModes{Val: "any"},
				&policy.PrintingDuplexDefault{Val: "short-edge"},
			},
			FetchValuesFunc: fetchDuplexModesFromPrintPreview,
		}, {
			TestName:                "default_long_edge_allowed_simplex",
			ExpectedDefaultValue:    "Simplex",
			ExpectedAvailableValues: []string{"Simplex"},
			Policies: []policy.Policy{
				&policy.PrintingAllowedDuplexModes{Val: "simplex"},
				&policy.PrintingDuplexDefault{Val: "long-edge"},
			},
			FetchValuesFunc: fetchDuplexModesFromPrintPreview,
		}, {
			TestName:                "default_short_edge_allowed_duplex",
			ExpectedDefaultValue:    "Flip on short edge",
			ExpectedAvailableValues: []string{"Flip on short edge", "Flip on long edge"},
			Policies: []policy.Policy{
				&policy.PrintingAllowedDuplexModes{Val: "duplex"},
				&policy.PrintingDuplexDefault{Val: "short-edge"},
			},
			FetchValuesFunc: fetchDuplexModesFromPrintPreview,
		}, {
			TestName:                "default_long_edge_allowed_duplex",
			ExpectedDefaultValue:    "Flip on long edge",
			ExpectedAvailableValues: []string{"Flip on short edge", "Flip on long edge"},
			Policies: []policy.Policy{
				&policy.PrintingAllowedDuplexModes{Val: "duplex"},
				&policy.PrintingDuplexDefault{Val: "long-edge"},
			},
			FetchValuesFunc: fetchDuplexModesFromPrintPreview,
		},
	}

	printingtest.RunFeatureRestrictionTest(ctx, s, subtestcases, nil /* printerAttributesFilePath */)
}

// fetchTwoSidedDuplexModesDropdown is a helper function for fetching the two
// sided duplex modes nodefinder. The problem is that this dropdown box is
// nameless, thus it is not possible to find it just by describing this node.
// This function checks the duplex checkbox if it's not checked already and
// returns the described nodefinder. It never unchecks the duplex checkbox,
// otherwise the two sided duplex dropdown box disappears.
// TODO(nedol): Simply find a node by its name after crbug.com/1422961 is complete.
func fetchTwoSidedDuplexModesDropdown(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, duplexCheckboxInfo *uiauto.NodeInfo) (*nodewith.Finder, error) {
	// Set two-sided printing checkbox so that the dropdown is available.
	if duplexCheckboxInfo.Checked == checked.False {
		if duplexCheckboxInfo.Restriction != restriction.None {
			return nil, errors.New("can't fetch two sided duplex modes dropdown - the duplex mode is unavailable")
		}
		if err := printpreview.SetCheckboxState(ctx, tconn, "Print on both sides", true); err != nil {
			return nil, errors.Wrap(err, "failed to set the value of 'Print on both sides' checkbox")
		}
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the keyboard")
	}
	defer kb.Close(ctx)

	// First focus on the previous checkbox and then move focus to the dropdown we need.
	twoSidedCheckboxNode := nodewith.Role(role.CheckBox).Name("Print on both sides")
	focusTwoSidedDuplexModesDropdownActions := []uiauto.Action{
		ui.EnsureFocused(twoSidedCheckboxNode),
		kb.AccelAction("Tab"),
	}
	if err := uiauto.Combine("focus two sided duplex modes dropdown",
		focusTwoSidedDuplexModesDropdownActions...)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to focus two sided duplex modes dropdown")
	}

	// Now the only focused dropdown is the one we need.
	return nodewith.Role(role.ComboBoxSelect).Focused(), nil
}

func fetchDefaultDuplexModeChecked(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, duplexCheckboxInfo *uiauto.NodeInfo) (string, error) {
	twoSidedDuplexModesDropdown, err := fetchTwoSidedDuplexModesDropdown(ctx, tconn, ui, duplexCheckboxInfo)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch two sided duplex modes dropdown")
	}
	twoSidedDuplexModesDropdownInfo, err := ui.Info(ctx, twoSidedDuplexModesDropdown)
	if err != nil {
		return "", errors.Wrap(err, "failed to fetch info about two sided duplex modes dropdown")
	}
	return twoSidedDuplexModesDropdownInfo.Value, nil
}

func fetchDefaultDuplexModeUnchecked() string {
	return "Simplex"
}

func fetchAvailableDuplexModesChecked(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, duplexCheckboxInfo *uiauto.NodeInfo) ([]string, error) {
	twoSidedDuplexModesDropdown, err := fetchTwoSidedDuplexModesDropdown(ctx, tconn, ui, duplexCheckboxInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch two sided duplex modes dropdown")
	}
	twoSidedDuplexTypes, err := dropdown.Values(ctx, tconn, twoSidedDuplexModesDropdown)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch dropdown values")
	}
	return twoSidedDuplexTypes, nil
}

func fetchAvailableDuplexModesUnchecked() []string {
	return []string{"Simplex"}
}

func fetchAvailableDuplexModesNoRestriction(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, duplexCheckboxInfo *uiauto.NodeInfo) ([]string, error) {
	var availableDuplexModes []string
	availableDuplexModes = append(availableDuplexModes, fetchAvailableDuplexModesUnchecked()...)
	availableDuplexModesChecked, err := fetchAvailableDuplexModesChecked(ctx, tconn, ui, duplexCheckboxInfo)
	if err != nil {
		return nil, errors.Wrap(err, "failed to fetch available duplex modes checked")
	}
	availableDuplexModes = append(availableDuplexModes, availableDuplexModesChecked...)
	return availableDuplexModes, nil
}

// fetchDuplexModesFromPrintPreview assumes that the print preview dialog is open.
// The print preview can be opened in multiple ways, probably the easiest one is to use the Ctrl+P shortcut.
func fetchDuplexModesFromPrintPreview(ctx context.Context, s *testing.State, tconn *chrome.TestConn) printingtest.SettingValues {
	ui := uiauto.New(tconn)
	if err := printpreview.ExpandMoreSettings(ctx, tconn); err != nil {
		s.Fatal("Failed to expand more settings: ", err)
	}

	duplexCheckbox := nodewith.Role(role.CheckBox).Name("Print on both sides")
	duplexCheckboxInfo, err := ui.Info(ctx, duplexCheckbox)
	if err != nil {
		s.Fatal("Failed to check the state of 'Print on both sides' checkbox: ", err)
	}

	var defaultDuplexMode string
	var availableDuplexModes []string
	// Fetch the default value.
	if duplexCheckboxInfo.Checked == checked.True {
		defaultDuplexMode, err = fetchDefaultDuplexModeChecked(ctx, tconn, ui, duplexCheckboxInfo)
		if err != nil {
			s.Fatal("Failed to fetch default duplex mode checked: ", err)
		}
	} else if duplexCheckboxInfo.Checked == checked.False {
		defaultDuplexMode = fetchDefaultDuplexModeUnchecked()
	} else {
		s.Fatal("Unknown duplexCheckbox checked state: ", string(duplexCheckboxInfo.Checked))
	}

	// Fetch the available values.
	if duplexCheckboxInfo.Restriction == restriction.None {
		availableDuplexModes, err = fetchAvailableDuplexModesNoRestriction(ctx, tconn, ui, duplexCheckboxInfo)
		if err != nil {
			s.Fatal("Failed to fetch available duplex modes with no restriction: ", err)
		}
	} else if duplexCheckboxInfo.Checked == checked.False {
		availableDuplexModes = fetchAvailableDuplexModesUnchecked()
	} else if duplexCheckboxInfo.Checked == checked.True {
		availableDuplexModes, err = fetchAvailableDuplexModesChecked(ctx, tconn, ui, duplexCheckboxInfo)
		if err != nil {
			s.Fatal("Failed to fetch available duplex modes checked: ", err)
		}
	} else {
		s.Fatal("Unknown duplexCheckbox checked state: ", string(duplexCheckboxInfo.Checked))
	}

	return printingtest.SettingValues{
		DefaultValue:    defaultDuplexMode,
		AvailableValues: availableDuplexModes,
	}
}
