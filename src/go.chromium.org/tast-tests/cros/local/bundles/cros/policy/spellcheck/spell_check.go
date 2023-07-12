// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package spellcheck contains helpers to verify the spellcheck service.
package spellcheck

import (
	"context"
	"net/http/httptest"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
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
	// Value is the policy value for this case.
	Value *policy.SpellCheckServiceEnabled
	// WantRestriction is whether the relevant buttons should be disabled.
	WantRestriction restriction.Restriction
	// WantSettingsCheck is the desired state of the settings toggle.
	WantSettingsCheck checked.Checked
	// WantContextCheck states whether the context menu checkmark should be there.
	WantContextCheck checked.Checked
	// ShouldFindAnnotation states wherher spellcheck_lookup annotation should be found in the net-export log.
	ShouldFindAnnotation bool
}

// GetTestCases returns the list of TestCase objects on which
// SpellCheckServiceEnabled policy is tested.
func GetTestCases() []TestCase {
	// Reordering the TestCase objects in the returned list may break tests.
	return []TestCase{
		{
			Name:              "disallow",
			Value:             &policy.SpellCheckServiceEnabled{Val: false},
			WantRestriction:   restriction.Disabled,
			WantSettingsCheck: checked.False,
			// "" means that there is no checkmark.
			WantContextCheck:     "",
			ShouldFindAnnotation: false,
		},
		{
			Name:                 "allow",
			Value:                &policy.SpellCheckServiceEnabled{Val: true},
			WantRestriction:      restriction.Disabled,
			WantSettingsCheck:    checked.True,
			WantContextCheck:     checked.True,
			ShouldFindAnnotation: true,
		},
		{
			Name:              "unset",
			Value:             &policy.SpellCheckServiceEnabled{Stat: policy.StatusUnset},
			WantRestriction:   restriction.None,
			WantSettingsCheck: checked.False,
			// "" means that there is no checkmark.
			WantContextCheck:     "",
			ShouldFindAnnotation: false,
		},
	}
}

// GetDataFiles returns the list of data files needed to be copied to the dut
// for running tests related to spell check.
func GetDataFiles() []string {
	return []string{"spell_checking.html"}
}

// TriggerSpellCheck attempts to trigger spellcheck and verifies if the policy works as defined in the TestCase param.
func TriggerSpellCheck(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, server *httptest.Server, tconn *chrome.TestConn, paramIndex int) (err error) {
	param := GetTestCases()[paramIndex]

	// Inside ChromeOS settings, check that the button is restricted and set to the correct value.
	if err := policyutil.OSSettingsPage(ctx, cr, "osSyncSetup").
		SelectNode(ctx, nodewith.
			Role(role.ToggleButton).
			NameStartingWith("Enhanced spell check")).
		Restriction(param.WantRestriction).
		Checked(param.WantSettingsCheck).
		Verify(); err != nil {
		return errors.Wrap(err, "unexpected os settings state")
	}

	if param.WantRestriction == restriction.Disabled {
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

	if param.WantRestriction != menuItem.Restriction {
		return errors.Errorf("menu item in wrong restriction state: want=%s, actual=%s", param.WantRestriction, menuItem.Restriction)
	}

	// If the checkmark is there, menuItem.Checked is checked.True (="true"),
	// otherwise it is "".
	if param.WantContextCheck != menuItem.Checked {
		return errors.Errorf("Menu item in wrong checking state: want=%s, actual=%s", param.WantContextCheck, menuItem.Checked)
	}
	return nil
}
