// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/pkcs11/netcertstore"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/passpoint"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/shill"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type knownNetworksPageUITest struct {
	// credentials is the set of Passpoint credentials used for the given test case.
	credentials *passpoint.Credentials
}

func init() {
	// Passpoint known network UI test is to verify that the installed passpoint
	// subscription showed properly in Known Networks Page in Settings.
	testing.AddTest(&testing.Test{
		Func:     PasspointKnownNetworkUI,
		Desc:     "Wi-Fi Passpoint known network UI test",
		Contacts: []string{"cros-networking@google.com", "damiendejean@google.com", "jiajunz@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFi",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi", "chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      chrome.LoginTimeout + 3*time.Minute,
		Requirements: []string{tdreq.WiFiGenSupportPasspoint},
		Params: []testing.Param{
			{
				Name: "tls",
				Val: knownNetworksPageUITest{
					credentials: &passpoint.Credentials{
						Domains: []string{passpoint.BlueDomain},
						Auth:    passpoint.AuthTLS,
					},
				},
			},
			{
				Name: "ttls",
				Val: knownNetworksPageUITest{
					credentials: &passpoint.Credentials{
						Domains: []string{passpoint.RedDomain},
						Auth:    passpoint.AuthTTLS,
					},
				},
			},
		},
	})
}

func PasspointKnownNetworkUI(ctx context.Context, s *testing.State) {
	// Reserve a little time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	// Get the interfaces created by hwsim.
	ifaces := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	if len(ifaces.Client) < 1 || len(ifaces.AP) < 1 {
		s.Fatal("Failed to obtain at least one client and AP interface from hwsim")
	}

	// Get the test case parameters.
	tc := s.Param().(knownNetworksPageUITest)

	// Create a certificate storage if needed.
	if tc.credentials.Auth == passpoint.AuthTLS {
		store, err := netcertstore.CreateStore(ctx, hwsec.NewCmdRunner())
		if err != nil {
			s.Fatal("Failed to create cert store: ", err)
		}
		defer store.Cleanup(cleanupCtx)

		id, err := store.InstallCertKeyPair(ctx, passpoint.TestCerts.ClientCred.PrivateKey, passpoint.TestCerts.ClientCred.Cert)
		if err != nil {
			s.Fatal("Failed to install test certificate/private key: ", err)
		}
		certOrKeyID := fmt.Sprintf("%d:%s", store.UserToken.Slot, id)
		tc.credentials.CertID = certOrKeyID
		tc.credentials.KeyID = certOrKeyID
	}

	// Start a user session.
	cred := chrome.Creds{User: netcertstore.TestUsername, Pass: netcertstore.TestPassword}
	cr, err := chrome.New(
		ctx,
		chrome.KeepState(),     // to avoid resetting TPM
		chrome.FakeLogin(cred), // to use the same user as certs are installed for
		chrome.EnableFeatures("PasspointSettings"),
		chrome.DisableFeatures("LocalPasswordForConsumers"), // b/328576285
	)
	if err != nil {
		s.Fatal("Failed to login: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}

	// Add the credentials set to the system.
	profile, err := m.ActiveProfile(ctx)
	if err != nil {
		s.Fatal("Failed to get active profile: ", err)
	}
	props, err := tc.credentials.ToShillProperties()
	if err != nil {
		s.Fatal("Failed to create Shill properties from Passpoint credentials: ", err)
	}
	if err := m.AddPasspointCredentials(ctx, profile.ObjectPath(), props); err != nil {
		s.Fatal("Failed to add Passpoint credentials to Shill: ", err)
	}

	ui := uiauto.New(tconn)
	const pageShortURL = "knownNetworks"
	condition := ui.Exists(nodewith.NameContaining("Known Networks").Role(role.Heading).Ancestor(ossettings.WindowFinder))

	_, err = ossettings.LaunchAtPageURL(ctx, tconn, cr, pageShortURL, condition)
	if err != nil {
		s.Fatal("Failed to launch at Known Network page: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	if err := ui.WaitUntilExists(ossettings.PasspointSubscriptionTitle)(ctx); err != nil {
		s.Fatal("Failed to show passpoint subscription section in known networks page: ", err)
	}

	if err := uiauto.Combine("Navigate to subscription detailed page",
		ui.LeftClick(nodewith.Name(tc.credentials.Domains[0]).Role(role.Link)),
		ui.WaitUntilExists(nodewith.Name(tc.credentials.Domains[0]).Role(role.Heading)),
	)(ctx); err != nil {
		s.Fatal("Failed to click on the passpoint subscription: ", err)
	}
}
