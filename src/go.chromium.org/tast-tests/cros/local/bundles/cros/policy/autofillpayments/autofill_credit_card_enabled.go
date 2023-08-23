// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package autofillpayments contains helpers to trigger autofill for credit card details
package autofillpayments

import (
	"context"
	"crypto/tls"
	"fmt"
	"net"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/annotations"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome"

	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/https"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
)

const (
	autofillCreditCardCertFile   = "certificate.pem"
	autofillCreditCardKeyFile    = "key.pem"
	autofillCreditCardCaCertFile = "ca-cert.pem"
	autofillCreditCardHTMLFile   = "autofill_credit_card_enabled.html"
	// AutofillCreditCardAnnotationHash is the hashcode of network annotation
	// autofill_credit_card_enabled.
	AutofillCreditCardAnnotationHash = "88863520"
)

// DataFiles returns the list of data files needed to be copied to the dut
// for running tests related to Autofill Payments
func DataFiles() []string {
	return []string{autofillCreditCardCertFile, autofillCreditCardKeyFile, autofillCreditCardCaCertFile, autofillCreditCardHTMLFile}
}

// testCase defines test expectations based on the policy value.
type testCase struct {
	Name                 string
	ShouldFindAnnotation bool
	wantRestriction      restriction.Restriction
	wantChecked          checked.Checked
	Policy               *policy.AutofillCreditCardEnabled
}

// GetTestCases returns the list of TestCase objects for each policy value.
func GetTestCases() map[networkrequestmonitor.PolicySetting]testCase {
	return map[networkrequestmonitor.PolicySetting]testCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:                 "deny",
			wantRestriction:      restriction.Disabled,
			wantChecked:          checked.False,
			ShouldFindAnnotation: false,
			Policy:               &policy.AutofillCreditCardEnabled{Val: false},
		},
		networkrequestmonitor.PolicyUnset: {
			Name:                 "unset",
			wantRestriction:      restriction.None,
			wantChecked:          checked.True,
			ShouldFindAnnotation: true,
			Policy:               &policy.AutofillCreditCardEnabled{Stat: policy.StatusUnset},
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:                 "allow",
			wantRestriction:      restriction.None,
			wantChecked:          checked.True,
			ShouldFindAnnotation: true,
			Policy:               &policy.AutofillCreditCardEnabled{Val: true},
		},
	}
}

// newLocalHTTPSTestServer (htmlFile, certFile, keyFile string) (*httptest.Server, error) {
func newLocalHTTPSTestServer(htmlFile, certFile, keyFile string) (*httptest.Server, error) {
	server := httptest.NewUnstartedServer(http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		switch r.Method {
		case "GET":
			http.ServeFile(w, r, htmlFile)
		case "POST":
			fmt.Fprintf(w, "Thanks for filling in your credit card details.")
		}
	}))

	cert, err := tls.LoadX509KeyPair(certFile, keyFile)
	if err != nil {
		return nil, err
	}
	server.TLS = &tls.Config{Certificates: []tls.Certificate{cert}}
	server.StartTLS()
	return server, nil
}

// TriggerAutofillCreditCardEnabled triggers autofill for credit card
func TriggerAutofillCreditCardEnabled(ctx context.Context, params networkrequestmonitor.OptionalServiceParams) (err error) {
	param := GetTestCases()[params.PolicySetting]
	cr := params.Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		errors.Wrap(err, "failed to create Test API connection")
	}
	server := params.Server
	br := params.Browser
	creditCardFields := []struct {
		fieldValue  string
		htmlFieldID string
	}{
		{
			fieldValue:  "4111111111111111", // A fake Visa fulfilling the Luhn algorithm.
			htmlFieldID: "cc-number",
		},
		{
			fieldValue:  "Tester",
			htmlFieldID: "cc-name",
		},
		{
			fieldValue:  time.Now().AddDate( /*year=*/ 1, 0, 0).Format("01/2006"),
			htmlFieldID: "cc-exp",
		},
	}
	// Open the net-export page and start logging.
	if err := annotations.StartLogging(ctx, cr, br, false); err != nil {
		return errors.Wrap(err, "failed to start logging")
	}

	// Ensure saving payment methods toggle is accordingly enabled/disabled.
	if err := policyutil.SettingsPage(ctx, cr, br, "payments").
		SelectNode(ctx, nodewith.
			Name("Save and fill payment methods").
			Role(role.ToggleButton)).
		Restriction(param.wantRestriction).
		Checked(param.wantChecked).
		Verify(); err != nil {
		return errors.Wrap(err, "unexpected settings state")
	}

	// If autofilling credit card details policy is enabled, let's ensure that we can add
	// a credit card in the settings and autofill it into a credit card form.
	if param.wantChecked == checked.True {
		ui := uiauto.New(tconn)
		visaNode := nodewith.NameContaining("Visa").NameContaining("1111").Role(role.StaticText)

		isSavedAlready, err := ui.IsNodeFound(ctx, visaNode)
		if err != nil {
			return errors.Wrap(err, "failed to check if credit card node is already found")
		}

		// TODO(crbug.com/1298550): Don't rely on all files being in same directory.
		baseDirectory := params.BaseDirectory
		serverConfiguration := https.ServerConfiguration{
			ServerKeyPath:         baseDirectory + "/" + autofillCreditCardKeyFile,
			ServerCertificatePath: baseDirectory + "/" + autofillCreditCardCertFile,
			CaCertificatePath:     baseDirectory + "/" + autofillCreditCardCaCertFile,
			HostedFilesBasePath:   baseDirectory,
		}

		// Save the certificate in chrome's certificate settings.
		if err := https.ConfigureChromeToAcceptCertificate(ctx, serverConfiguration, cr, br, tconn); err != nil {
			return errors.Wrap(err, "failed to set the certificate in Chrome's settings")
		}

		// The certificate doesn't work in 127.0.0.1 and it needs to be replaced with localhost.
		port := server.Listener.Addr().(*net.TCPAddr).Port
		urlToOpen := fmt.Sprintf("https://localhost:%d/%v", port, autofillCreditCardHTMLFile)

		// Open the website with the credit card form.
		conn, err := openCreditCardPage(ctx, br, tconn, urlToOpen)
		if err != nil {
			errors.Wrap(err, "failed to open website")
		}
		defer conn.Close()

		// If the credit card has already been saved, saving the same card again is not possible.
		if !isSavedAlready {
			kb, err := input.Keyboard(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to use keyboard")
			}
			defer kb.Close(ctx)

			// Fill in the credit card details and click on the save button.
			jsScript := "(htmlFieldID, fieldValue) => { document.getElementById(htmlFieldID).value = fieldValue; }"
			for _, creditCardField := range creditCardFields {
				if err := conn.Call(ctx, nil, jsScript, creditCardField.htmlFieldID, creditCardField.fieldValue); err != nil {
					return errors.Wrap(err, "failed to set the field value")
				}
			}

			if err := uiauto.Combine("trigger and handle the save prompt for the credit card",
				ui.DoDefaultUntil(nodewith.Name("OK").Role(role.Button).ClassName("test-target-button"), ui.Exists(visaNode)),
				ui.LeftClickUntil(nodewith.Role(role.Button).Name("Save").ClassName("MdTextButton"), ui.Exists(nodewith.NameContaining("Card saved").Role(role.StaticText))),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to save credit card")
			}

			// Re-open the website with the credit card form.
			conn.Close()
			conn, err = openCreditCardPage(ctx, br, tconn, urlToOpen)
			if err != nil {
				return errors.Wrap(err, "failed to open website")
			}
			defer conn.Close()
		}

		// Trigger the autofill on the credit card form page.
		nameTextBox := nodewith.Role(role.TextField).Name("Name on card")
		autofillPopup := nodewith.Role(role.ListBoxOption).ClassName("PopupCellView").First()
		if err := uiauto.Combine("clicking the Name on card field and choosing the suggested credit card",
			ui.DoDefaultUntil(nameTextBox, ui.Exists(autofillPopup)),
			ui.DoDefaultUntil(autofillPopup, ui.Exists(nodewith.Role(role.InlineTextBox).Name(creditCardFields[0].fieldValue))),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to trigger and use credit card autofill")
		}

		// Run JavaScript checks to confirm that all the fields are set correctly.
		for _, creditCardField := range creditCardFields {
			var valueFromHTML string
			if err := conn.Eval(ctx, "document.getElementById('"+creditCardField.htmlFieldID+"').value", &valueFromHTML); err != nil {
				return errors.Wrapf(err, "failed to get htmlFieldID=%s:", creditCardField.htmlFieldID)
			}
			if valueFromHTML != creditCardField.fieldValue {
				return errors.Errorf("credit card field was not set properly; got %q, want %q", valueFromHTML, creditCardField.fieldValue)

			}
		}
	}
	foundAnnotation := false
	if foundAnnotation, err = annotations.StopLoggingCheckLogs(ctx, cr, br, AutofillCreditCardAnnotationHash); err != nil {
		return errors.Wrap(err, "failed to save credit card")
	}

	if foundAnnotation != param.ShouldFindAnnotation {
		return errors.Errorf("unexpected autofill annotation result: got %t expected %t", foundAnnotation, param.ShouldFindAnnotation)
	}
	return nil

}

func openCreditCardPage(ctx context.Context, br *browser.Browser, tconn *chrome.TestConn, urlToOpen string) (*chrome.Conn, error) {
	ui := uiauto.New(tconn)
	conn, err := br.NewConn(ctx, urlToOpen)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open credit card page")
	}

	// Ensure the page is open.
	if err := ui.WaitUntilExists(nodewith.Name("OK").Role(role.Button).ClassName("test-target-button"))(ctx); err != nil {
		return nil, errors.Wrap(err, "expected to find the OK button on the credit card page")
	}

	return conn, nil
}
