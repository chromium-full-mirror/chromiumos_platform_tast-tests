// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package iwa

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	ssoOffloadingIWAExampleUpdateManifestURL = "https://github.com/google/sso-offloading/releases/latest/download/iwa-update-manifest.json"
	ssoOffloadingIWAExampleWebBundleID       = "v5uvfpi6dtpf7xhj3swcaoxfmgui645rc47uib23a5jtt477yhyaaaic"
	ssoOffloadingIWAExampleURL               = "isolated-app://v5uvfpi6dtpf7xhj3swcaoxfmgui645rc47uib23a5jtt477yhyaaaic"
	extensionID                              = "jmdcfpeebneidlbnldlhcifibpkidhkn"
	extensionName                            = "SSO Offloading Handler"
	extensionUpdateXMLFileURL                = "https://github.com/google/sso-offloading/releases/latest/download/extension-update-manifest.xml"
	gaiaURLPrefix                            = "https://accounts.google.com/"
	expectedSsoCfSrcPrefix                   = "https://developers.google.com/oauthplayground/?code="
)

func init() {
	testing.AddTest(&testing.Test{
		Func:          ControlledFrameSSOOffloading,
		Desc:         "Checks installing an IWA and a Chrome extension via policies, and tests SSO offloading flow",
		Contacts:     []string{"iwa-team@google.com"},
		BugComponent: "b:1168200",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Timeout:      2*chrome.LoginTimeout + 5*time.Minute,
		Fixture:       fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.IsolatedWebAppInstallForceList{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ExtensionInstallForcelist{}, pci.VerifiedFunctionalityOS),
		},
	})
}

// ControlledFrameSSOOffloading installs an IWA and an Extension via policy
// and tests the end-to-end SSO Offloading flow.
func  ControlledFrameSSOOffloading(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	gaiaCreds, err := credconfig.PickRandomCreds(
		dma.CredsFromPool(policy.ManagedUserAccountPoolVarName))
	if err != nil {
		s.Fatal("Failed to parse managed user creds: ", err)
	}

	if err := setupPolicies(fdms, gaiaCreds.User); err != nil {
		s.Fatal("Failed to setup policies: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr, err := chrome.New(
		ctx,
		chrome.DMSPolicy(fdms.URL),
		chrome.GAIALogin(gaiaCreds),
		chrome.KeepEnrollment(),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_dump")

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	ui := uiauto.New(tconn)

	if err := waitForExtensionInstalled(ctx, cr, ui, cleanupCtx); err != nil {
		s.Fatal("Failed to verify extension installation: ", err)
	}

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(cleanupCtx)

	conn, err := connectToIWA(ctx, cr, cleanupCtx)
	if err != nil {
		s.Fatal("Failed to connect to IWA: ", err)
	}
	defer conn.Close()
	defer conn.CloseTarget(cleanupCtx)

	if err := waitForSendMessageAPI(ctx, conn); err != nil {
		s.Error("chrome.runtime.sendMessage check failed: ", err)
	}

	if err := performIWASetupFlow(ctx, ui); err != nil {
		s.Fatal("Failed to perform IWA setup flow: ", err)
	}

	gaiaConn, err := triggerAndHandleGaiaFlow(ctx, cr, ui, kb, gaiaCreds.User)
	if err != nil {
		s.Fatal("Failed to trigger and handle GAIA flow: ", err)
	}
	if gaiaConn != nil {
		defer gaiaConn.Close()
		defer gaiaConn.CloseTarget(cleanupCtx)
	}

	if err := pollForSsoCfSrcUpdate(ctx, conn, s); err != nil {
		s.Fatal("Failed to poll for ssoCf src update: ", err)
	}

	if err := checkIWAForSSOErrors(ctx, conn, s); err != nil {
		s.Error("IWA SSO error check failed: ", err)
	}
	s.Log("Test finished, GAIA tab will be closed if it wasn't already")
}

func setupPolicies(fdms *fakedms.FakeDMS, policyUser string) error {
	policyBlob := policy.NewBlob()
	policyBlob.PolicyUser = policyUser

	policies := []policy.Policy{
		&policy.IsolatedWebAppInstallForceList{
			Val: []*policy.IsolatedWebAppInstallForceListValue{
				{
					UpdateManifestUrl: ssoOffloadingIWAExampleUpdateManifestURL,
					WebBundleId:       ssoOffloadingIWAExampleWebBundleID,
				},
			},
		},
		&policy.ExtensionInstallForcelist{
			Val: []string{extensionID + ";" + extensionUpdateXMLFileURL},
		},
	}

	if err := policyBlob.AddPolicies(policies); err != nil {
		return errors.Wrap(err, "failed to add policies")
	}

	if err := fdms.WritePolicyBlob(policyBlob); err != nil {
		return errors.Wrap(err, "failed to write policies to FakeDMS")
	}
	return nil
}

func waitForExtensionInstalled(ctx context.Context, cr *chrome.Chrome, ui *uiauto.Context, cleanupCtx context.Context) error {
	testing.ContextLog(ctx, "Navigating to chrome://extensions to verify UI")
	extPageConn, err := cr.NewConn(ctx, "chrome://extensions")
	if err != nil {
		return errors.Wrap(err, "failed to navigate to chrome://extensions")
	}
	defer extPageConn.Close()
	defer extPageConn.CloseTarget(cleanupCtx)

	extNameNode := nodewith.Name(extensionName).Role(role.Heading)

	testing.ContextLogf(ctx, "Waiting for extension %q (ID: %s) to appear on chrome://extensions", extensionName, extensionID)
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(extNameNode)(ctx); err != nil {
		return errors.Wrapf(err, "extension %q (ID: %s) did not appear on chrome://extensions within the timeout", extensionName, extensionID)
	}
	testing.ContextLogf(ctx, "Extension %q (ID: %s) is visible on chrome://extensions", extensionName, extensionID)
	return nil
}

func connectToIWA(ctx context.Context, cr *chrome.Chrome, cleanupCtx context.Context) (*chrome.Conn, error) {
	var conn *chrome.Conn
	testing.ContextLogf(ctx, "Waiting for IWA to be available at %s", ssoOffloadingIWAExampleURL)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var pollErr error
		conn, pollErr = cr.NewConn(ctx, ssoOffloadingIWAExampleURL)
		if pollErr != nil {
			return errors.Wrapf(pollErr, "failed to connect to IWA at %s (will retry)", ssoOffloadingIWAExampleURL)
		}
		return nil
	}, &testing.PollOptions{Timeout: 60 * time.Second, Interval: 5 * time.Second}); err != nil {
		return nil, errors.Wrapf(err, "failed to connect to IWA at %s after multiple retries", ssoOffloadingIWAExampleURL)
	}
	testing.ContextLogf(ctx, "Successfully connected to IWA at %s", ssoOffloadingIWAExampleURL)
	return conn, nil
}

func waitForSendMessageAPI(ctx context.Context, conn *chrome.Conn) error {
	testing.ContextLog(ctx, "Polling for chrome.runtime.sendMessage availability in IWA")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		jsExpr := "typeof chrome !== 'undefined' && typeof chrome.runtime !== 'undefined' && typeof chrome.runtime.sendMessage === 'function'"
		available := false
		if err := conn.Eval(ctx, jsExpr, &available); err != nil {
			return errors.Wrap(err, "failed to evaluate JS for chrome.runtime.sendMessage check")
		}
		if !available {
			return errors.New("chrome.runtime.sendMessage is not yet available")
		}
		return nil
	}, &testing.PollOptions{Timeout: 15 * time.Second, Interval: time.Second}); err != nil {
		return errors.Wrap(err, "chrome.runtime.sendMessage did not become available within the timeout")
	}
	testing.ContextLog(ctx, "chrome.runtime.sendMessage IS available in the IWA")
	return nil
}

func performIWASetupFlow(ctx context.Context, ui *uiauto.Context) error {
	submitButton := nodewith.Name("Submit").Role(role.Button).ClassName("ssoFormElement")
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(submitButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to find Submit button in IWA")
	}

	setupSSOButton := nodewith.Name("Setup SSO offloading").Role(role.Button)
	successText := nodewith.Name("SSO connector started successfully.").Role(role.StaticText)

	if err := uiauto.Combine("Perform SSO Offloading flow",
		ui.LeftClick(submitButton),
		ui.WithTimeout(10*time.Second).WaitUntilExists(setupSSOButton),
		ui.LeftClick(setupSSOButton),
		ui.WithTimeout(10*time.Second).WaitUntilExists(successText),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to complete SSO Offloading flow in IWA")
	}
	testing.ContextLog(ctx, "SSO Offloading flow completed successfully")
	return nil
}

func triggerAndHandleGaiaFlow(ctx context.Context, cr *chrome.Chrome, ui *uiauto.Context, kb *input.KeyboardEventWriter, userEmail string) (*chrome.Conn, error) {
	authorizeAPIButton := nodewith.Name("Click `Authorize APIs` button within Controlled Frame below.").First()
	if err := ui.LeftClick(authorizeAPIButton)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to click Authorize APIs button")
	}

	testing.ContextLogf(ctx, "Waiting for a tab with URL prefix %q to open", gaiaURLPrefix)
	waitCtx, cancelWait := context.WithTimeout(ctx, 20*time.Second)
	defer cancelWait()

	matchGaiaURL := func(t *chrome.Target) bool {
		return t.Type == "page" && strings.HasPrefix(t.URL, gaiaURLPrefix)
	}

	gaiaConn, err := cr.NewConnForTarget(waitCtx, matchGaiaURL)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find and connect to tab with URL prefix %q", gaiaURLPrefix)
	}
	testing.ContextLog(ctx, "Successfully connected to GAIA login tab")

	emailElement := nodewith.NameContaining(userEmail).Role(role.Link)
	testing.ContextLogf(ctx, "Searching for element containing email: %s", userEmail)
	if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(emailElement)(ctx); err != nil {
		gaiaConn.Close()
		gaiaConn.CloseTarget(context.Background())
		return nil, errors.Wrapf(err, "failed to find element with email %q on the GAIA page", userEmail)
	}

	testing.ContextLogf(ctx, "Clicking element containing email: %s", userEmail)
	if err := ui.LeftClick(emailElement)(ctx); err != nil {
		gaiaConn.Close()
		gaiaConn.CloseTarget(context.Background())
		return nil, errors.Wrapf(err, "failed to click element with email %q", userEmail)
	}
	testing.ContextLog(ctx, "Successfully clicked the email element")

	continueButton := nodewith.Name("Continue").Role(role.Button).First()
	testing.ContextLog(ctx, "Waiting for Continue button to appear")
	if err := ui.WithTimeout(15 * time.Second).WaitUntilExists(continueButton)(ctx); err != nil {
		gaiaConn.Close()
		gaiaConn.CloseTarget(context.Background())
		return nil, errors.Wrap(err, "continue button did not appear within the timeout")
	}

	testing.ContextLog(ctx, "Clicking Continue button")
	if err := ui.LeftClick(continueButton)(ctx); err != nil {
		gaiaConn.Close()
		gaiaConn.CloseTarget(context.Background())
		return nil, errors.Wrap(err, "failed to click Continue button")
	}
	testing.ContextLog(ctx, "Successfully clicked Continue button")

	return gaiaConn, nil
}

func pollForSsoCfSrcUpdate(ctx context.Context, conn *chrome.Conn, s *testing.State) error {
	s.Logf("Polling for ssoCf src attribute to start with: %s", expectedSsoCfSrcPrefix)
	return testing.Poll(ctx, func(ctx context.Context) error {
		var ssoCfSrc string
		ssoCfExpr := `(function() { const el = document.getElementById('ssoCf'); return el ? el.getAttribute('src') : ''; })()`
		if err := conn.Eval(ctx, ssoCfExpr, &ssoCfSrc); err != nil {
			return errors.Wrap(err, "failed to evaluate JS to get ssoCf src attribute")
		}

		if strings.HasPrefix(ssoCfSrc, expectedSsoCfSrcPrefix) {
			s.Logf("ssoCf src attribute updated to: %s", ssoCfSrc)
			return nil
		}
		return errors.Errorf("ssoCf src is still not matching: %s", ssoCfSrc)
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 2 * time.Second})
}

func checkIWAForSSOErrors(ctx context.Context, conn *chrome.Conn, s *testing.State) error {
	s.Log("Checking for errors in formValidationMessage on IWA page")
	var formMessage string
	jsExpr := `(function() { const el = document.getElementById('formValidationMessage'); return el ? el.textContent : ''; })()`
	if err := conn.Eval(ctx, jsExpr, &formMessage); err != nil {
		return errors.Wrap(err, "failed to evaluate JS to get formValidationMessage text")
	}

	trimmedMessage := strings.TrimSpace(formMessage)
	if trimmedMessage != "" {
		if strings.HasPrefix(trimmedMessage, "SSO Error:") {
			return errors.Errorf("SSO Error found in formValidationMessage: %s", formMessage)
		}
		s.Logf("formValidationMessage content: %s", formMessage)
	} else {
		s.Log("formValidationMessage element not found or is empty")
	}
	return nil
}
