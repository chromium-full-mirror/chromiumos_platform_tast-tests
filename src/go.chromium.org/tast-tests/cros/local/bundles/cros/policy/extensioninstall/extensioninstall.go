// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package extensioninstall contains helpers to verify
// extension install enabled policy.
package extensioninstall

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestCase definition.
type TestCase struct {
	Name                 string          // name is the subtest name.
	AllowInstall         bool            // whether the extension should be allowed to be installed or not.
	Policies             []policy.Policy // policies is a list of ExtensionInstallAllowlist, ExtensionInstallBlocklist policies.
	ShouldFindAnnotation bool            // Whether network annotations should be present or not
}

// Hash codes for NetworkTrafficAnnotationTags.
const extensionCrxFetcher = "21145003"
const extensionManifestFetcher = "5151071"
const extensionInstallSigner = "50464499"
const webstoreInstaller = "18764319"
const chromeAppsSocketapi = "8591273"
const downloadsAPIRunAsync = "121068967"
const pepperTCPSocket = "120623198"
const blinkExtensionResourceLoader = "84165821"

// ExtensionID of the Google keep chrome extension.
const ExtensionID = "lpcaedmchfhocbbapmcbpinfpgnhiddi"
const extensionURL = "https://chrome.google.com/webstore/detail/" + ExtensionID

// GetAnnotations gives annotations that should/shouldn't be present when test is invoked.
func GetAnnotations() []string {
	return []string{extensionCrxFetcher, extensionManifestFetcher, extensionInstallSigner, webstoreInstaller, chromeAppsSocketapi, downloadsAPIRunAsync,
		pepperTCPSocket, blinkExtensionResourceLoader}
}

// GetAnnotationsForUmbrellaTest gives annotations that should or shouldn't
// be present when the umbrella test is invoked.
// Note: The existing umbrella test invokes some of the extensions leading to 2
// annotations being already present.
func GetAnnotationsForUmbrellaTest() []string {
	return []string{extensionInstallSigner, webstoreInstaller, chromeAppsSocketapi, downloadsAPIRunAsync, pepperTCPSocket, blinkExtensionResourceLoader}
}

// TriggerExtensionInstall invokes the extension install scenario.
func TriggerExtensionInstall(ctx context.Context, testCase TestCase, tconn *chrome.TestConn, br *browser.Browser) (err error) {
	// Run actual test.
	if allowInstall, err := triggerExtensionInstallReturnStatus(ctx, tconn, br); err != nil {
		return errors.Wrap(err, "failed to check if extension can be installed")
	} else if allowInstall != testCase.AllowInstall {
		return errors.Wrapf(err, "unexpected result when trying to install extension: got %t; want %t", allowInstall, testCase.AllowInstall)
	}
	return err
}

// triggerExtensionInstallReturnStatus invokes extension install and returns whether the extension install succeeded or failed.
func triggerExtensionInstallReturnStatus(ctx context.Context, tconn *chrome.TestConn, br *browser.Browser) (bool, error) {
	// Ensure google cookies are accepted, it appears when we open the extension link.
	if err := policyutil.EnsureGoogleCookiesAccepted(ctx, br); err != nil {
		return false, errors.Wrap(err, "failed to accept cookies")
	}

	addfinder := nodewith.Role(role.Button).Name("Add to Chrome")
	blockedfinder := nodewith.Role(role.Button).Name("Blocked by admin")

	// Open the Chrome Web Store page of the extension.
	conn, err := br.NewConn(ctx, extensionURL)
	if err != nil {
		return false, errors.Wrap(err, "failed to connect to chrome")
	}
	defer conn.Close()

	var allowInstall bool
	ui := uiauto.New(tconn)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// For blocked extensions, there should be a blocked button.
		if blocked, err := ui.IsNodeFound(ctx, blockedfinder.First()); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to check Blocked by admin button"))
		} else if blocked {
			allowInstall = false
			return nil
		}

		// For allowed extensions, there should be a button to add them.
		if allowed, err := ui.IsNodeFound(ctx, addfinder.First()); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to check Add to chrome button"))
		} else if allowed {
			allowInstall = true
			return nil
		}

		return errors.New("failed to determine the outcome")
	}, &testing.PollOptions{Timeout: 20 * time.Second}); err != nil {
		return false, err
	}

	// The new Chrome Web Store In Preview experience adds a toast message
	// so check for that before concluding the extension is allowed.
	blockedDivXPathSelector := fmt.Sprintf(`//div[text()="Your admin has blocked this item (ID: %s)"]`, ExtensionID)
	if allowInstall {
		blockedDiv := fmt.Sprintf("document.evaluate('%s', document, null, XPathResult.FIRST_ORDERED_NODE_TYPE).singleNodeValue", blockedDivXPathSelector)
		if err := conn.WaitForExprWithTimeout(ctx, fmt.Sprintf("%s != null", blockedDiv), time.Second); err == nil {
			allowInstall = false
		}
	}

	return allowInstall, nil
}
