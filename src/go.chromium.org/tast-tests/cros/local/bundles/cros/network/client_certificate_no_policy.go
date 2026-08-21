// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net/http/httptest"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/managedclientcert"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ClientCertificateNoPolicy is the negative counterpart of ClientCertificate: it
// verifies that no managed client certificate exists for a user whose
// provisioning policy is disabled.
//
// What the test does:
// It logs in as the provided GAC-managed user and confirms none of the
// managed-certificate artifacts exist:
//  1. chrome://connectors-internals: Enabled Policy Levels is "None" and no
//     managed client certificate identity is present.
//  2. A local HTTPS server that requires a TLS client certificate (see
//     managedclientcert.StartClientAuthServer): no certificate picker appears
//     and TLS client authentication does NOT succeed, since there's no
//     certificate to present.
//  3. Chaps / PKCS#11: no certificate issued by the enterprise CA in the user's token.
//
// Unlike ClientCertificate, this does not check the Certificate Manager listing.
// With no identity on connectors-internals (1) and no certificate in the Chaps
// user token (3), the certificate provably does not exist, so a UI listing check
// would add no coverage and would only risk breaking on Certificate Manager UI
// changes.
//
// Google Admin Console setup (on the OU that contains the provided account):
//   - The managed client certificate provisioning policy must be DISABLED at
//     every level, so chrome://connectors-internals reports "None".
//
// DUT requirements:
//   - Outbound internet access (GAIA login).
//
// How to run:
// This test uses a default GAC-managed account (without the client certificate
// provisioning policy enabled) from the TAPE pool default_managed (via
// managedclientcert.NoPolicyLoggedInFixture) and runs with:
//
//	tast run <dut> network.ClientCertificateNoPolicy
//
// (An account can also be provided manually via -var tape.provided_account='user@domain:password').

func init() {
	testing.AddTest(&testing.Test{
		Func: ClientCertificateNoPolicy,
		Desc: "Verify no managed client certificate exists when the policy is disabled",
		Contacts: []string{
			"cbe-cep-eng@google.com",         // Team
			"vishwa.kalubowila@codimite.com", // Test author
			"seblalancette@chromium.org",     // Test owner
		},
		BugComponent: "b:1000044",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Fixture:      managedclientcert.NoPolicyLoggedInFixture,
		Timeout:      5 * time.Minute,
	})
}

func ClientCertificateNoPolicy(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(managedclientcert.FixtValue)
	cr := v.Chrome()
	tconn := v.TestAPIConn()

	verifyNoManagedClientCert(ctx, s, cr)

	website := managedclientcert.StartClientAuthServer()
	defer website.Close()
	verifyNoTLSClientAuth(ctx, s, cr, tconn, website)

	verifyNoChapsUserToken(ctx, s, v.Username())
}

// verifyNoManagedClientCert opens chrome://connectors-internals and verifies the
// managed client certificate policy is disabled and no identity is provisioned.
func verifyNoManagedClientCert(ctx context.Context, s *testing.State, cr *chrome.Chrome) {
	conn, err := cr.NewConn(ctx, managedclientcert.MCCURL)
	if err != nil {
		s.Fatalf("Failed to open %s: %v", managedclientcert.MCCURL, err)
	}
	defer conn.Close()

	// The page snapshots the cert state at load time, so ReadInfo reloads it every
	// attempt to force a fresh read.
	var info managedclientcert.Info
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		info, err = managedclientcert.ReadInfo(ctx, conn)
		if err != nil {
			return err
		}
		if info.PolicyLevels == "" {
			return errors.New("connectors-internals page not populated yet")
		}
		return nil
	}, &testing.PollOptions{Timeout: 45 * time.Second, Interval: 2 * time.Second}); err != nil {
		s.Fatal("Failed to read connectors-internals: ", err)
	}

	s.Logf("Managed client certificate state: %+v", info)

	if info.PolicyLevels != "None" {
		s.Errorf("Enabled Policy Levels = %q, want %q (the account should have the policy disabled)", info.PolicyLevels, "None")
	}
	if info.HasIdentity {
		s.Errorf("Unexpected managed client certificate identity present: name=%q issuer=%q trust=%q", info.IdentityName, info.Issuer, info.TrustLevel)
	}
}

// verifyNoTLSClientAuth navigates to the local server that requires TLS
// client-certificate authentication and verifies it does not report success.
func verifyNoTLSClientAuth(ctx context.Context, s *testing.State, cr *chrome.Chrome, tconn *chrome.TestConn, website *httptest.Server) {
	ui := uiauto.New(tconn)

	conn, err := managedclientcert.OpenClientAuth(ctx, cr, website)
	if err != nil {
		s.Fatal("Failed to open the client-auth page: ", err)
	}
	defer conn.Close()

	// A certificate picker should never appear here (no certificate should
	// exist), so only wait briefly for it instead of the full page timeout.
	certPopup := nodewith.Name("Select a certificate").First()
	if err := ui.WithTimeout(5 * time.Second).WaitUntilExists(certPopup)(ctx); err == nil {
		s.Fatal("Unexpected certificate picker appeared: a client certificate exists when none was expected")
	}

	// Without a client certificate the load may end on an error page, so don't
	// fail if quiescence times out; just inspect whatever rendered.
	if err := webutil.WaitForQuiescence(ctx, conn, 30*time.Second); err != nil {
		s.Log("Page did not reach quiescence (expected without a client cert): ", err)
	}

	var html string
	if err := conn.Eval(ctx, "document.documentElement.outerHTML", &html); err != nil {
		s.Fatal("Failed to read the client-auth page HTML: ", err)
	}
	if strings.Contains(html, managedclientcert.ClientAuthSuccessText) {
		s.Errorf("Client-auth page unexpectedly reported success: %q found in HTML output", managedclientcert.ClientAuthSuccessText)
	} else {
		s.Log("TLS client authentication did not succeed, as expected")
	}
}

// verifyNoChapsUserToken verifies that no certificate issued by the enterprise CA
// is present in the Chaps / PKCS#11 user token belonging to username.
func verifyNoChapsUserToken(ctx context.Context, s *testing.State, username string) {
	cert, err := managedclientcert.FindUserTokenCert(ctx, username, managedclientcert.ExpectedIssuer)
	if err != nil {
		s.Fatal("Failed to inspect the Chaps user token: ", err)
	}
	if cert != nil {
		s.Errorf("Unexpected certificate issued by %q found in the user token (subject %q)", managedclientcert.ExpectedIssuer, cert.Subject.CommonName)
		return
	}
	s.Log("No managed client certificate in the Chaps user token, as expected")
}
