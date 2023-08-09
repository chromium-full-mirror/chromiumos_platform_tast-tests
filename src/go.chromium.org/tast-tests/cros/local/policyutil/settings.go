// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policyutil

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/errors"
)

// nodeChecker is used in checking different properties of a node while collecting the error messages.
type nodeChecker struct {
	err  error
	info *uiauto.NodeInfo
}

// openedPage stores information that allows chaining SettingsPage() and SelectNode() together
// without the repetition of some input parameters.
type openedPage struct {
	cr  *chrome.Chrome
	err error
}

// SettingsPage opens a settings page with given link (e.g. "content/location" -> "chrome://settings/content/location").
// The returned openedPage value can be used to select a node from the node tree (not just from the page).
func SettingsPage(ctx context.Context, cr *chrome.Chrome, br ash.ConnSource, shortLink string) *openedPage {
	page := &openedPage{
		cr: cr,
	}

	conn, err := br.NewConn(ctx, "chrome://settings/"+shortLink)
	if err != nil {
		page.err = err
		return page
	}
	defer conn.Close()

	return page
}

// OSSettingsPage opens the OS settings page with given link (e.g. "osAccessibility" -> "chrome://os-settings/osAccessibility").
// The returned openedPage value can be used to select a node from the node tree (not just from the page).
func OSSettingsPage(ctx context.Context, cr *chrome.Chrome, shortLink string) *openedPage {
	page := &openedPage{
		cr: cr,
	}

	conn, err := apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/"+shortLink)
	if err != nil {
		page.err = err
		return page
	}
	defer conn.Close()

	return page
}

// OSSettingsPageWithPassword open the OS settings page with given link (e.g. "osAccessibility" -> "chrome://os-settings/osAccessibility").
// If the opened settings page is password protected, try to authenticate with the given password.
// The returned openedPage value can be used to select a node from the node tree (not just from the page).
func OSSettingsPageWithPassword(ctx context.Context, cr *chrome.Chrome, shortLink, password string) *openedPage {
	page := &openedPage{
		cr: cr,
	}

	conn, err := apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/"+shortLink)
	if err != nil {
		page.err = errors.Wrap(err, "failed to launch OS Settings")
		return page
	}
	defer conn.Close()

	if err := ossettings.ConfirmPassword(ctx, cr, password); err != nil {
		page.err = errors.Wrap(err, "failed to confirm password")
		return page
	}

	return page
}

// CurrentPage return with an openedPage, which can be used to select a node from the node tree (not just from the page).
func CurrentPage(cr *chrome.Chrome) *openedPage {
	page := &openedPage{
		cr: cr,
	}

	return page
}

// SelectNode creates a nodeChecker from the selected node.
// It can be used to verify different properties of the node.
func (page *openedPage) SelectNode(ctx context.Context, finder *nodewith.Finder) *nodeChecker {
	checker := &nodeChecker{}

	if page.err != nil {
		checker.err = errors.Wrap(page.err, "unable to select node as page was not opened properly")
		return checker
	}

	tconn, err := page.cr.TestAPIConn(ctx)
	if err != nil {
		checker.err = errors.Wrap(err, "failed to create Test API connection")
		return checker
	}

	uia := uiauto.New(tconn)
	info, err := uia.Info(ctx, finder)
	if err != nil {
		checker.err = errors.Wrap(err, "failed get the info of the node")
		return checker
	}

	checker.info = info
	return checker
}

// Restriction checks the restriction state of a Settings node given by the SettingsState() function.
func (checker *nodeChecker) Restriction(expectedRestriction restriction.Restriction) *nodeChecker {
	if checker.err != nil || checker.info == nil {
		return checker
	}

	if checker.info.Restriction != expectedRestriction {
		checker.err = errors.Errorf("unexpected node restriction state; want %q, got %q", expectedRestriction, checker.info.Restriction)
	}

	return checker
}

// Checked checks the checked state of a Settings node given by the SettingsState() function.
func (checker *nodeChecker) Checked(expectedChecked checked.Checked) *nodeChecker {
	if checker.err != nil || checker.info == nil {
		return checker
	}

	if checker.info.Checked != expectedChecked {
		checker.err = errors.Errorf("unexpected node checked state; want %q, got %q", expectedChecked, checker.info.Checked)
	}

	return checker
}

// Verify is the last element of the Settings state verifying.
// It returns with the error collected during the process.
func (checker *nodeChecker) Verify() error {
	return checker.err
}

// CheckCertificateVisibleInSystemSettings checks that a certificate with the
// given name is usable for configuring a VPN connection. Fully creating such
// a connection would require a special network environment, so it just tests
// that the certificate is visible and selectable.
func CheckCertificateVisibleInSystemSettings(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome, certName string) error {
	OSSettingsPage(ctx, cr, "network")
	ui := uiauto.New(tconn)

	return uiauto.Combine("use system settings",
		ui.DoDefault(nodewith.Name("Add network connection").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name("Add built-in VPN…").Role(role.Button)),
		ui.DoDefault(nodewith.Name("Add built-in VPN…").Role(role.Button)),
		ui.WaitUntilExists(nodewith.Name("User certificate").ClassName("md-select")),
		ui.DoDefault(nodewith.Name("User certificate").ClassName("md-select")),
		ui.DoDefault(nodewith.Name(certName+" ["+certName+"]").Role(role.ListBoxOption)))(ctx)
}
