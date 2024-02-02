// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/hostapd"
	"go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type removalTest struct {
	// credentials is the set of Passpoint credentials used for the given test
	// case.
	credentials *passpoint.Credentials
	// ap is the access point configuration used in the removal test.
	ap *passpoint.AccessPoint
}

func init() {
	// Credentials removal test is designed to verifies that the different
	// credentials removal operations are working as expected. In particular:
	// - a profile pop (logout) only unloads credentials from Shill,
	// - service removal through unloads credentials from Shill and
	//   from the storage.
	testing.AddTest(&testing.Test{
		Func:     PasspointCredentialsRemoval,
		Desc:     "Wi-Fi Passpoint credentials removal test",
		Contacts: []string{"cros-networking@google.com", "damiendejean@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Fixture:      "shillSimulatedWiFi",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi", "chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      3*chrome.LoginTimeout + 3*time.Minute,
		Requirements: []string{tdreq.WiFiGenSupportPasspoint},
		Params: []testing.Param{
			{
				Name: "tls",
				Val: removalTest{
					credentials: &passpoint.Credentials{
						Domains: []string{passpoint.BlueDomain},
						Auth:    passpoint.AuthTLS,
					},
					ap: &passpoint.AccessPoint{
						SSID:               "passpoint-tls-blue",
						Domain:             passpoint.BlueDomain,
						Realms:             []string{passpoint.BlueDomain},
						RoamingConsortiums: []string{passpoint.HomeOI},
						Auth:               passpoint.AuthTLS,
					},
				},
			}, {
				Name: "ttls",
				Val: removalTest{
					credentials: &passpoint.Credentials{
						Domains: []string{passpoint.RedDomain},
						Auth:    passpoint.AuthTTLS,
					},
					ap: &passpoint.AccessPoint{
						SSID:               "passpoint-ttls",
						Domain:             passpoint.RedDomain,
						Realms:             []string{passpoint.RedDomain},
						RoamingConsortiums: []string{passpoint.HomeOI},
						Auth:               passpoint.AuthTTLS,
					},
				},
			},
		},
	})
}

func PasspointCredentialsRemoval(ctx context.Context, s *testing.State) {
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
	tc := s.Param().(removalTest)

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
	if err := chromeLogin(ctx); err != nil {
		s.Fatal("Failed to login: ", err)
	}

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}

	// Create a wifi manager to follow the state of the WiFi service on client side.
	wifi, err := shill.NewWifiManager(ctx, m)
	if err != nil {
		s.Fatal("Failed to obtain Wi-Fi manager: ", err)
	}

	// Allow Shill to perform interworking select and match networks.
	if err := m.SetInterworkingSelectEnabled(ctx, ifaces.Client[0], true); err != nil {
		s.Fatal("Failed to enable interworking selection: ", err)
	}
	defer m.SetInterworkingSelectEnabled(cleanupCtx, ifaces.Client[0], false)

	// Create the test access point.
	ap := tc.ap.ToServer(ifaces.AP[0], s.OutDir())
	if err := ap.Start(ctx); err != nil {
		s.Fatal("Failed to start test access point: ", err)
	}
	defer ap.Stop()

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

	// Create a hostapd monitor to obtain association events from the access point.
	hmon := hostapd.NewMonitor()
	if err := hmon.Start(ctx, ap); err != nil {
		s.Fatal("Failed to start hostapd monitor: ", err)
	}
	defer func(ctx context.Context) {
		if err := hmon.Stop(ctx); err != nil {
			s.Fatal("Failed to stop hostapd monitor: ", err)
		}
	}(cleanupCtx)

	s.Log("Checking initial association with Passpoint network")

	// Check the credentials allow association with the access point.
	if err := checkSTAAssociation(ctx, wifi, hmon, ifaces.Client[0], tc.ap.SSID, true); err != nil {
		s.Fatal("Failed to associate with the access point: ", err)
	}

	s.Log("Checking association with Passpoint network after logout")

	// Logout
	if err := chromeLogout(ctx); err != nil {
		s.Fatal("Failed to logout: ", err)
	}

	// Check the DUT can't connect after logout.
	if err := checkSTAAssociation(ctx, wifi, hmon, ifaces.Client[0], tc.ap.SSID, false); err != nil {
		s.Fatal("Failed to check STA do not associate the access point: ", err)
	}

	s.Log("Checking association with Passpoint network after login")

	// Login
	if err = chromeLogin(ctx); err != nil {
		s.Fatal("Failed to login again: ", err)
	}

	// Check the credentials allow association with the access point.
	if err := checkSTAAssociation(ctx, wifi, hmon, ifaces.Client[0], tc.ap.SSID, true); err != nil {
		s.Fatal("Failed to associate with the access point: ", err)
	}

	// Remove the service created to connect to the Passpoint AP.
	service, err := wifi.ScanAndWaitForService(ctx, tc.ap.SSID, 30*time.Second)
	if err != nil {
		s.Fatalf("Failed to obtain WiFi service for %q: %v", tc.ap.SSID, err)
	}
	if err := service.Remove(ctx); err != nil {
		s.Fatal("Failed to remove WiFi service: ", err)
	}

	s.Log("Checking association after service removal")

	// Check we're not able to connect anymore.
	if err := checkSTAAssociation(ctx, wifi, hmon, ifaces.Client[0], tc.ap.SSID, false); err != nil {
		s.Fatal("Failed to associate with the access point: ", err)
	}

	// Logout and login again to check
	if err := chromeLogout(ctx); err != nil {
		s.Fatal("Failed to logout: ", err)
	}
	if err = chromeLogin(ctx); err != nil {
		s.Fatal("Failed to login again: ", err)
	}

	// Check we're not able to connect anymore.
	if err := checkSTAAssociation(ctx, wifi, hmon, ifaces.Client[0], tc.ap.SSID, false); err != nil {
		s.Fatal("Failed to associate with the access point: ", err)
	}
}

func chromeLogin(ctx context.Context) error {
	cred := chrome.Creds{User: netcertstore.TestUsername, Pass: netcertstore.TestPassword}
	cr, err := chrome.New(
		ctx,
		chrome.KeepState(),     // to avoid resetting TPM
		chrome.FakeLogin(cred), // to use the same user as certs are installed for
	)
	if err != nil {
		return err
	}
	if err := cr.Close(ctx); err != nil {
		return errors.Wrap(err, "failed to close Chrome connection")
	}
	return nil
}

func chromeLogout(ctx context.Context) error {
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return errors.Wrap(err, "failed to restart ui")
	}
	return nil
}

func checkSTAAssociation(ctx context.Context, wm *shill.WifiManager, mon *hostapd.Monitor, iface, ssid string, associated bool) error {
	// Delay to wait for a network to be discovered or associated.
	const timeout = 30 * time.Second

	// Trigger a scan
	if _, err := wm.ScanAndWaitForService(ctx, ssid, timeout); err != nil {
		return errors.Wrapf(err, "failed to wait for service %q: ", ssid)
	}

	// Check the device connects to the access point.
	if err := hostapd.WaitForSTAAssociated(ctx, mon, iface, timeout); err != nil {
		if errors.Is(err, hostapd.ErrAssociationTimeout) && !associated {
			// We don't expect an association, so a timeout means success.
			return nil
		}
		return err
	}

	return nil
}
