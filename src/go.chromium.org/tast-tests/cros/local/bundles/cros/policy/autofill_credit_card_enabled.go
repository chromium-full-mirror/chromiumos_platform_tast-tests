// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"crypto/tls"
	"fmt"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/autofillpayments"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/netexport"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	autofillCreditCardCertFile       = "certificate.pem"
	autofillCreditCardKeyFile        = "key.pem"
	autofillCreditCardCaCertFile     = "ca-cert.pem"
	autofillCreditCardHTMLFile       = "autofill_credit_card_enabled.html"
	autofillCreditCardAnnotationHash = "88863520"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AutofillCreditCardEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Behavior of AutofillCreditCardEnabled policy, checking the correspoding toggle button states (restriction and checked) after setting the policy",
		Contacts: []string{
			"chrome-payments-team@google.com", // Feature owner
			"dp-chromeos-eng@google.com",
		},
		BugComponent: "b:1457039",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
		Data: []string{autofillCreditCardHTMLFile, autofillCreditCardKeyFile, autofillCreditCardCertFile, autofillCreditCardCaCertFile},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.AutofillCreditCardEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

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

func AutofillCreditCardEnabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve 10 seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	server, err := newLocalHTTPSTestServer(s.DataPath(autofillCreditCardHTMLFile), s.DataPath(autofillCreditCardCertFile), s.DataPath(autofillCreditCardKeyFile))
	if err != nil {
		s.Fatal("Failed to start the server: ", err)
	}
	defer server.Close()

	for key, param := range autofillpayments.GetTestCases() {

		s.Run(ctx, param.Name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.Policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to open the browser: ", err)
			}
			defer closeBrowser(cleanupCtx)
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.Name)

			// Open the net-export page and start logging.
			netExport, err := netexport.Start(ctx, cr, br, s.Param().(browser.Type))
			if err != nil {
				s.Fatal("Failed to start net export: ", err)
			}
			defer netExport.Cleanup(cleanupCtx)

			// TODO(crbug.com/1298550): Don't rely on all files being in same directory.
			baseDirectory := filepath.Dir(s.DataPath(autofillCreditCardCertFile))
			if err := autofillpayments.TriggerAutofillCreditCardEnabled(ctx,
				networkrequestmonitor.OptionalServiceParams{
					Chrome:        cr,
					Browser:       br,
					Server:        server,
					BaseDirectory: baseDirectory,
					PolicySetting: key}); err != nil {
				s.Fatal("Failed to trigger autofill for payments: ", err)
			}

			foundAnnotation, err := netExport.Find(autofillpayments.AutofillCreditCardAnnotationHash)
			if err != nil {
				s.Fatal("Failed to search net log file for annotation: ", err)
			}
			if foundAnnotation != param.ShouldFindAnnotation {
				s.Fatalf("Annotation mismatch = got %t, want %t", foundAnnotation, param.ShouldFindAnnotation)
			}
		})
	}
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
