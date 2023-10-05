// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package spellcheck contains helpers to verify the spellcheck service.
package spellcheck

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome/proxy/mitmproxy"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
)

// AnnotationHashCode is the hashcode of network annotation tag
// spellcheck_lookup.
const AnnotationHashCode = "132553989"

// TestCase defines test expectations based on the policy value.
type TestCase struct {
	// Name is the testcase name.
	Name string
	// Policy is the policy value for this case.
	Policy *policy.SpellCheckServiceEnabled
	// WantRestriction is whether the relevant buttons should be disabled.
	WantRestriction restriction.Restriction
	// WantSettingsCheck is the desired state of the settings toggle.
	WantSettingsCheck checked.Checked
	// WantContextCheck states whether the context menu checkmark should be there.
	WantContextCheck checked.Checked
	// ShouldFindAnnotation states wherher spellcheck_lookup annotation should be found in the net-export log.
	ShouldFindAnnotation bool
}

// TestCases returns the map of policy setting enum to TestCase object
// on which the SpellCheckServiceEnabled policy is tested.
func TestCases() map[networkrequestmonitor.PolicySetting]TestCase {
	return map[networkrequestmonitor.PolicySetting]TestCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:              "disallow",
			Policy:            &policy.SpellCheckServiceEnabled{Val: false},
			WantRestriction:   restriction.Disabled,
			WantSettingsCheck: checked.False,
			// "" means that there is no checkmark.
			WantContextCheck:     "",
			ShouldFindAnnotation: false,
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:                 "allow",
			Policy:               &policy.SpellCheckServiceEnabled{Val: true},
			WantRestriction:      restriction.Disabled,
			WantSettingsCheck:    checked.True,
			WantContextCheck:     checked.True,
			ShouldFindAnnotation: true,
		},
		networkrequestmonitor.PolicyUnset: {
			Name:              "unset",
			Policy:            &policy.SpellCheckServiceEnabled{Stat: policy.StatusUnset},
			WantRestriction:   restriction.None,
			WantSettingsCheck: checked.False,
			// "" means that there is no checkmark.
			WantContextCheck:     "",
			ShouldFindAnnotation: false,
		},
	}
}

// DataFiles returns the list of data files needed to be copied to the dut
// for running tests related to spell check.
func DataFiles() []string {
	return []string{"spell_checking.html", mitmproxy.MitmdumpBinFile}
}

// TriggerSpellCheck attempts to trigger spellcheck and verifies if the policy works as defined in the TestCase param.
func TriggerSpellCheck(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	br := params.Browser
	cr := params.Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}
	server := params.Server
	policyParam := TestCases()[params.PolicySetting]
	// Inside ChromeOS settings, check that the button is restricted and set to the correct value.
	if err := policyutil.OSSettingsPage(ctx, cr, "osSyncSetup").
		SelectNode(ctx, nodewith.
			Role(role.ToggleButton).
			NameStartingWith("Enhanced spell check")).
		Restriction(policyParam.WantRestriction).
		Checked(policyParam.WantSettingsCheck).
		Verify(); err != nil {
		return errors.Wrap(err, "unexpected os settings state")
	}

	if policyParam.WantRestriction == restriction.Disabled {
		// Check for the enterprise icon.
		if err := policyutil.OSSettingsPage(ctx, cr, "osSyncSetup").
			SelectNode(ctx, nodewith.
				Role(role.Image).
				NameStartingWith("Enhanced spell check")).
			Verify(); err != nil {
			return errors.Wrap(err, "unexpected os settings state")
		}
	}

	// Open the browser and navigate to a page that contains an input field with the word "missspelled".
	url := server.URL + "/spell_checking.html"
	conn, err := br.NewConn(ctx, url)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the browser")
	}
	defer conn.Close()

	textfield := nodewith.Role(role.InlineTextBox).Name("missspelled")

	ui := uiauto.New(tconn)
	if err := ui.RightClick(textfield)(ctx); err != nil {
		return errors.Wrap(err, "failed to right click text field")

	}
	if err := ui.LeftClick(nodewith.Role(role.MenuItem).Name("Spell check"))(ctx); err != nil {
		return errors.Wrap(err, "failed to left click spell check button")
	}

	menuItem, err := ui.Info(ctx, nodewith.ClassName("MenuItemView").Name("Use enhanced spell check"))
	if err != nil {
		return errors.Wrap(err, "failed to get info for menuitemcheckBox")
	}

	if policyParam.WantRestriction != menuItem.Restriction {
		return errors.Errorf("menu item in wrong restriction state: want=%s, actual=%s", policyParam.WantRestriction, menuItem.Restriction)
	}

	// If the checkmark is there, menuItem.Checked is checked.True (="true"),
	// otherwise it is "".
	if policyParam.WantContextCheck != menuItem.Checked {
		return errors.Errorf("Menu item in wrong checking state: want=%s, actual=%s", policyParam.WantContextCheck, menuItem.Checked)
	}
	return nil
}
