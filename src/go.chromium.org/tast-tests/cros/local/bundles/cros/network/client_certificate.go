// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net/http/httptest"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/certpageutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/managedclientcert"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// ClientCertificate verifies, end-to-end, the managed client certificate that
// Chrome provisions for a Google Admin Console (GAC) managed user.
//
// What the test does:
// It logs in as the provided GAC-managed user, then confirms the certificate
// through four independent surfaces:
//  1. chrome://connectors-internals: a managed client certificate identity
//     exists with the expected key trust level (HW on devices with a TPM, OS
//     otherwise; selected by the test parameter).
//  2. Certificate Manager (Platform Client Certs): the certificate is listed.
//  3. A local HTTPS server that requires a TLS client certificate (see
//     managedclientcert.StartClientAuthServer): Chrome auto-selects the managed
//     certificate via the AutoSelectCertificateForUrls policy (no picker is
//     shown) and the page reports that the certificate authenticated
//     ("Authentication OK").
//  4. Chaps / PKCS#11: the certificate is present in the user's token.
//
// Google Admin Console setup (on the OU that contains the provided account):
//   - Enable the managed client certificate provisioning policy (Browser or
//     Profile level). Provisioning must use the Google-hosted CA so the issued
//     certificate's issuer common name is "Chrome Enterprise CA" (see
//     managedclientcert.ExpectedIssuer); a different CA changes the issuer and
//     fails the test.
//   - Set the AutoSelectCertificateForUrls policy so Chrome presents the managed
//     certificate to the local test server automatically instead of showing a
//     "Select a certificate" picker. The server listens on 127.0.0.1 on a random
//     port, so the pattern MUST use a port wildcard:
//       {"pattern":"https://127.0.0.1:*","filter":{"ISSUER":{"CN":"Chrome Enterprise CA"}}}
//
// DUT requirements:
//   - Outbound internet access (GAIA login).
//   - A TPM is optional: the key is hardware-backed (HW) with a TPM and
//     software-backed (OS) without one. The matching variant (.hw / .os) is
//     selected automatically by the variant's TPM hardware dependency.
//
// How to run:
// This test uses an account from the TAPE pool gcac_cert_provisioning (via
// managedclientcert.LoggedInFixture) and runs with:
//
//	tast run <dut> network.ClientCertificate.*
//
// (An account can also be provided manually via -var tape.provided_account='user@domain:password').

func init() {
	testing.AddTest(&testing.Test{
		Func: ClientCertificate,
		Desc: "Verify the managed client certificate is provisioned and usable",
		Contacts: []string{
			"cbe-cep-eng@google.com",         // Team
			"vishwa.kalubowila@codimite.com", // Test author
			"seblalancette@chromium.org",     // Test owner
		},
		BugComponent: "b:1000044",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Fixture:      managedclientcert.LoggedInFixture,
		Timeout:      5 * time.Minute,
		// The managed client certificate key is hardware-backed on devices with a
		// TPM and falls back to software otherwise. Each variant runs only on the
		// matching hardware and asserts its expected Key Trust Level.
		Params: []testing.Param{
			{
				Name:              "hw",
				ExtraHardwareDeps: hwdep.D(hwdep.HasTpm()),
				Val:               "HW",
			},
			{
				Name:              "os",
				ExtraHardwareDeps: hwdep.D(hwdep.HasNoTpm()),
				Val:               "OS",
			},
		},
	})
}

func ClientCertificate(ctx context.Context, s *testing.State) {
	v := s.FixtValue().(managedclientcert.FixtValue)
	cr := v.Chrome()
	tconn := v.TestAPIConn()

	// wantTrust is the expected Key Trust Level for this variant: "HW" on devices
	// with a TPM, "OS" otherwise.
	wantTrust := s.Param().(string)

	info := verifyConnectorsInternals(ctx, s, cr, wantTrust)

	verifyListedInCertManager(ctx, s, cr, tconn, info)

	website := managedclientcert.StartClientAuthServer()
	defer website.Close()
	verifyTLSClientAuth(ctx, s, cr, website)

	verifyChapsUserToken(ctx, s, v.Username())
}

// verifyConnectorsInternals opens chrome://connectors-internals, waits for the
// managed client certificate to be provisioned, and verifies its key trust level
// matches wantTrust ("HW" on TPM devices, "OS" otherwise).
func verifyConnectorsInternals(ctx context.Context, s *testing.State, cr *chrome.Chrome, wantTrust string) managedclientcert.Info {
	conn, err := cr.NewConn(ctx, managedclientcert.MCCURL)
	if err != nil {
		s.Fatalf("Failed to open %s: %v", managedclientcert.MCCURL, err)
	}
	defer conn.Close()
	s.Logf("Opened %s", managedclientcert.MCCURL)

	// The page snapshots the cert state at load time, so ReadInfo reloads it every
	// attempt to force a fresh read.
	var info managedclientcert.Info
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		info, err = managedclientcert.ReadInfo(ctx, conn)
		if err != nil {
			return err
		}
		// "None" means the policy is disabled at every level
		if info.PolicyLevels == "None" {
			return testing.PollBreak(errors.New(
				"managed client certificate policy is not enabled (Enabled Policy Levels: None); check the account's policy on GAC"))
		}
		if !info.HasIdentity {
			return errors.New("managed client certificate identity not provisioned yet")
		}
		if info.TrustLevel == "" {
			return errors.New("key trust level not populated yet")
		}
		return nil
	}, &testing.PollOptions{Timeout: 1 * time.Minute, Interval: 3 * time.Second}); err != nil {
		s.Fatal("Failed waiting for the managed client certificate: ", err)
	}

	s.Logf("Managed client certificate: %+v", info)

	if info.TrustLevel != wantTrust {
		s.Errorf("Key Trust Level = %q, want %q", info.TrustLevel, wantTrust)
	}
	if info.Issuer != managedclientcert.ExpectedIssuer {
		s.Errorf("Issuer = %q, want %q", info.Issuer, managedclientcert.ExpectedIssuer)
	}
	if info.PublicKeyHash == "" {
		s.Error("Public Key Hash is empty, want a non-empty value")
	}

	return info
}

// verifyListedInCertManager opens the Certificate Manager Platform Client Certs
// subpage and verifies that a client certificate is listed.
func verifyListedInCertManager(ctx context.Context, s *testing.State, cr *chrome.Chrome, tconn *chrome.TestConn, info managedclientcert.Info) {
	ui := uiauto.New(tconn)

	conn, err := cr.NewConn(ctx, certpageutils.CertificatesPageURL)
	if err != nil {
		s.Fatalf("Failed to open %s: %v", certpageutils.CertificatesPageURL, err)
	}
	defer conn.Close()

	if err := certpageutils.OpenUserInstalledClientCertsNewUI(ctx, ui); err != nil {
		s.Fatal("Failed to open the Platform Client Certs subpage: ", err)
	}

	// Match the provisioned identity's Subject common name from verifyConnectorsInternals.
	certEntry := nodewith.NameContaining(info.Subject).First()
	if err := ui.WithTimeout(30 * time.Second).WaitUntilExists(certEntry)(ctx); err != nil {
		s.Fatal("No client certificate listed under Platform Client Certs: ", err)
	}
	s.Log("Client certificate is listed under Platform Client Certs")
}

// verifyTLSClientAuth navigates to the local server that requires TLS
// client-certificate authentication (see managedclientcert.StartClientAuthServer)
// and verifies the page reports success. Chrome selects the managed certificate
// automatically via the AutoSelectCertificateForUrls policy, so no
// "Select a certificate" picker is expected.
func verifyTLSClientAuth(ctx context.Context, s *testing.State, cr *chrome.Chrome, website *httptest.Server) {
	conn, err := managedclientcert.OpenClientAuth(ctx, cr, website)
	if err != nil {
		s.Fatal("Failed to open the client-auth page: ", err)
	}
	defer conn.Close()

	if err := webutil.WaitForQuiescence(ctx, conn, 30*time.Second); err != nil {
		s.Fatalf("Failed to wait for %s to load: %v", website.URL, err)
	}

	var html string
	if err := conn.Eval(ctx, "document.documentElement.outerHTML", &html); err != nil {
		s.Fatal("Failed to read the client-auth page HTML: ", err)
	}
	if !strings.Contains(html, managedclientcert.ClientAuthSuccessText) {
		s.Errorf("Client-auth page did not report success: %q not found in HTML output; got: %s", managedclientcert.ClientAuthSuccessText, html)
	} else {
		s.Log("TLS client authentication succeeded")
	}
}

// verifyChapsUserToken verifies that the managed client certificate is present
// in the Chaps / PKCS#11 user token belonging to username.
func verifyChapsUserToken(ctx context.Context, s *testing.State, username string) {
	cert, err := managedclientcert.FindUserTokenCert(ctx, username, managedclientcert.ExpectedIssuer)
	if err != nil {
		s.Fatal("Failed to inspect the Chaps user token: ", err)
	}
	if cert == nil {
		s.Errorf("No certificate issued by %q found in the user token", managedclientcert.ExpectedIssuer)
		return
	}
	s.Logf("Found managed client certificate in the user token (subject %q)", cert.Subject.CommonName)
}
