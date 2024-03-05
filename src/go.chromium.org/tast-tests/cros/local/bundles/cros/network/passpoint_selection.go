// Copyright 2022 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/hostapd"
	"go.chromium.org/tast-tests/cros/local/hwsec"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/wpasupplicant"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// selectionTest describes the parameters of a single test case.
type selectionTest struct {
	// credentials is the list of Passpoint credentials used for the test.
	credentials []*passpoint.Credentials
	// aps is the list of test access points used to test the network selection.
	aps []passpoint.AccessPoint
	// expectedSSID is the name of the network that should be connected at the end of the test.
	expectedSSID string
}

// Quoting IEEE802.11-2020 §9.4.1.31:
// "The Organization Identifier field contains a public unique identifier
// assigned by the IEEE Registration Authority as a 24-bit OUI, a 24-bit CID,
// or a 36-bit OUI-36.
// The length of the Organization Identifier field is the minimum number of
// octets required to contain the entire IEEE-assigned identifier. Thus, the
// Organization Identifier field is 3 octets in length if the IEEE-assigned
// identifier is an OUI or CID, or 5 octets in length if the IEEE-assigned
// identifier is an OUI-36."
// Both shill and Android internally stores OIs as integers. This means that
// it is possible for the representation to lose the leading 00s information
// from the OIs. To fix the issue, shill has an internal logic to assume the
// correct 00s padding for OIs (b/277299855).
const (
	ouiOI                   = "871d2e"
	ouiOIWithLeadingZeros   = "00002e"
	oui36OI                 = "0a1bc500b5"
	oui36OIWithLeadingZeros = "000bc500b5"
)

func init() {
	// Selection tests are designed to verify a device connects to the best
	// available Passpoint network. Shill is provisioned with one (or more) set
	// of Passpoint credentials, and the device is exposed to one (or more)
	// Passpoint compatible access point. The test then verifies the device
	// select the best possible Passpoint network.
	testing.AddTest(&testing.Test{
		Func:     PasspointSelection,
		Desc:     "Wi-Fi Passpoint network selection tests",
		Contacts: []string{"cros-networking@google.com", "damiendejean@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Fixture:      "shillSimulatedWiFi",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      5 * time.Minute,
		Requirements: []string{tdreq.WiFiGenSupportPasspoint},
		Params: []testing.Param{
			{
				Name: "home_match_with_domain",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-blue",
				},
			}, {
				Name: "home_match_with_oi",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							HomeOIs: []string{passpoint.HomeOI},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-green",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-green",
				},
			}, {
				Name: "home_match_with_required_oi",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains:         []string{passpoint.BlueDomain},
							RequiredHomeOIs: []string{passpoint.HomeOI},
							Auth:            passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:   "passpoint-another-blue",
							Domain: passpoint.BlueDomain,
							Realms: []string{passpoint.BlueDomain},
							Auth:   passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-blue",
				},
			}, {
				Name: "home_match_only_required_ois",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains:         []string{passpoint.BlueDomain},
							RequiredHomeOIs: []string{passpoint.HomeOI, passpoint.RoamingOI1},
							Auth:            passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue-home",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI, passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-blue-oi1",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-blue-oi2",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI2},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-blue-home",
				},
			}, {
				Name: "roaming_match_with_domain",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-green",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain, passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-green",
				},
			}, {
				Name: "roaming_match_with_oi",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains:    []string{passpoint.BlueDomain},
							HomeOIs:    []string{passpoint.HomeOI},
							RoamingOIs: []string{passpoint.RoamingOI1},
							Auth:       passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-green",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-green",
				},
			}, {
				Name: "home_over_roaming_ap",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains:    []string{passpoint.BlueDomain},
							HomeOIs:    []string{passpoint.HomeOI},
							RoamingOIs: []string{passpoint.RoamingOI1},
							Auth:       passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-green",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-blue",
				},
			}, {
				Name: "roaming_match_with_security",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-ttls",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-tls",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI2},
							Auth:               passpoint.AuthTLS,
						},
					},
					expectedSSID: "passpoint-ttls",
				},
			}, {
				Name: "two_home_credentials",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							HomeOIs: []string{passpoint.HomeOI},
							Auth:    passpoint.AuthTTLS,
						}, {
							Domains: []string{passpoint.RedDomain},
							HomeOIs: []string{passpoint.HomeOI},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-blue",
				},
			}, {
				Name: "two_roaming_credentials",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains:    []string{passpoint.GreenDomain},
							HomeOIs:    []string{passpoint.RoamingOI1},
							RoamingOIs: []string{passpoint.HomeOI},
							Auth:       passpoint.AuthTTLS,
						}, {
							Domains:    []string{passpoint.RedDomain},
							HomeOIs:    []string{passpoint.RoamingOI2},
							RoamingOIs: []string{passpoint.HomeOI},
							Auth:       passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-blue",
				},
			}, {
				Name: "home_over_roaming_credentials",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							HomeOIs: []string{passpoint.HomeOI},
							Auth:    passpoint.AuthTTLS,
						}, {
							Domains:    []string{passpoint.RedDomain},
							HomeOIs:    []string{passpoint.RoamingOI2},
							RoamingOIs: []string{passpoint.RoamingOI1},
							Auth:       passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-green",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-blue",
				},
			}, {
				Name: "home_with_two_domains",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain, passpoint.RedDomain},
							HomeOIs: []string{passpoint.HomeOI},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-red",
							Domain:             passpoint.RedDomain,
							Realms:             []string{passpoint.RedDomain, passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-red",
				},
			}, {
				Name: "home_match_with_tls_realm",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							Auth:    passpoint.AuthTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-tls-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTLS,
						}, {
							SSID:               "passpoint-ttls-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-tls-blue",
				},
			}, {
				Name: "match_with_oui",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							HomeOIs: []string{ouiOI},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-oui-oi",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{ouiOI},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-oui-oi",
				},
			}, {
				Name: "match_with_oui_with_leading_zeros",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							HomeOIs: []string{ouiOIWithLeadingZeros},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-oui-oi",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{ouiOIWithLeadingZeros},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-oui-oi",
				},
			}, {
				Name: "match_with_oui36",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							HomeOIs: []string{oui36OI},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-oui36-oi",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{oui36OI},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-oui36-oi",
				},
			}, {
				Name: "match_with_oui36_with_leading_zeros",
				Val: selectionTest{
					credentials: []*passpoint.Credentials{
						{
							Domains: []string{passpoint.BlueDomain},
							HomeOIs: []string{oui36OIWithLeadingZeros},
							Auth:    passpoint.AuthTTLS,
						},
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-oui36-oi",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{oui36OIWithLeadingZeros},
							Auth:               passpoint.AuthTTLS,
						},
					},
					expectedSSID: "passpoint-oui36-oi",
				},
			},
		},
	})
}

// selectionTestContext contains the required environment to run the current test case.
type selectionTestContext struct {
	// manager is the proxy to Shill Manager API.
	manager *shill.Manager
	// aps is the list of access points/server created for the test.
	aps []*hostapd.Server
	// credentials is the set of Passpoint credentials under test.
	credentials []*passpoint.Credentials
	// hasTLS is true when the test will use TLS authentication.
	hasTLS bool
	// clientIface is the simulated interface used by Shill as a client interface.
	clientIface string
	// expectedAP is the access point instance where to expect the device connection.
	expectedAP *hostapd.Server
	// expectedSSID is the name of the network where to expect the device connection.
	expectedSSID string
}

func PasspointSelection(ctx context.Context, s *testing.State) {
	// Reserve a little time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	tc, err := prepareSelectionTest(ctx, s)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}

	// Prepare the certificate storage.
	var certOrKeyID string
	if tc.hasTLS {
		store, err := netcertstore.CreateStore(ctx, hwsec.NewCmdRunner())
		if err != nil {
			s.Fatal("Failed to create cert store: ", err)
		}
		defer store.Cleanup(cleanupCtx)

		id, err := store.InstallCertKeyPair(ctx, passpoint.TestCerts.ClientCred.PrivateKey, passpoint.TestCerts.ClientCred.Cert)
		if err != nil {
			s.Fatal("Failed to install test certificate/private key: ", err)
		}
		certOrKeyID = fmt.Sprintf("%d:%s", store.UserToken.Slot, id)
	}

	// Create a profile dedicated to the test
	profileName := passpoint.RandomProfileName()
	profile, err := tc.manager.CreateFakeUserProfile(ctx, profileName)
	if err != nil {
		s.Fatal("Failed to create test profile: ", err)
	}
	defer func(ctx context.Context) {
		// Remove the test profile
		if err := tc.manager.RemoveFakeUserProfile(ctx, profileName); err != nil {
			s.Fatal("Failed to clean test profile: ", err)
		}
	}(cleanupCtx)

	// Ensure the cache of scanned network will be flushed right after the
	// removal of the access points.
	// As the test run is quicker than real life network changes, the cache might
	// keep a list of available networks between two tests and lead to spurious
	// connection attempts from Shill (b/282090326) on networks that does no
	// exist anymore.
	wpas, err := wpasupplicant.NewSupplicant(ctx)
	if err != nil {
		s.Fatal("Failed to create wpa_supplicant proxy")
	}
	iface, err := wpas.GetInterface(ctx, tc.clientIface)
	if err != nil {
		s.Fatalf("Failed to obtain %s interface from wpa_supplicant", tc.clientIface)
	}
	// Set the expiration time of the cache entries to 0 (i.e. immediate flush).
	defer iface.FlushBSS(cleanupCtx, 0)

	// Start all the access points.
	for _, ap := range tc.aps {
		testing.ContextLogf(ctx, "Creating %s", ap)
		if err := ap.Start(ctx); err != nil {
			s.Fatal("Failed to start access point: ", err)
		}
		defer ap.Stop()
	}

	// Add the sets of credentials to Shill.
	for _, c := range tc.credentials {
		if c.Auth == passpoint.AuthTLS {
			c.CertID = certOrKeyID
			c.KeyID = certOrKeyID
		}
		prop, err := c.ToShillProperties()
		if err != nil {
			s.Fatal("Failed to get credentials' shill properties: ", err)
		}
		if err := tc.manager.AddPasspointCredentials(ctx, profile, prop); err != nil {
			s.Fatal("Failed to set Passpoint credentials: ", err)
		}
	}

	// Create a monitor to collect access point events.
	testing.ContextLogf(ctx, "Starting monitor for %s", tc.expectedAP)
	m := hostapd.NewMonitor()
	if err := m.Start(ctx, tc.expectedAP); err != nil {
		s.Fatal("Failed to start hostapd monitor: ", err)
	}
	defer func(ctx context.Context) {
		if err := m.Stop(ctx); err != nil {
			s.Fatal("Failed to stop hostapd monitor: ", err)
		}
	}(cleanupCtx)

	wifi, err := shill.NewWifiManager(ctx, tc.manager)
	if err != nil {
		s.Fatal("Failed to obtain Wi-Fi manager: ", err)
	}

	// Delay to wait for a network to be discovered.
	const scanAndWaitTimeout = time.Minute
	// Trigger a scan
	if _, err := wifi.ScanAndWaitForService(ctx, tc.expectedSSID, scanAndWaitTimeout); err != nil {
		s.Fatal("Failed to request an active scan: ", err)
	}

	// Check the device connects to the access point.
	if err := hostapd.WaitForSTAAssociated(ctx, m, tc.clientIface, hostapd.STAAssociationTimeout); err != nil {
		s.Fatal("Passpoint client not connected to access point: ", err)
	}
}

// prepareSelectionTest creates a test profile, reads the test parameters and delivers a test context.
func prepareSelectionTest(ctx context.Context, s *testing.State) (tc *selectionTestContext, err error) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to shill Manager")
	}

	// Obtain the simulated interfaces from the fixture environment.
	ifaces := s.FixtValue().(*hwsim.ShillSimulatedWiFi)

	// Obtain the parameters for the current test case and ensure it does not
	// require more interfaces than the fixture provided.
	params := s.Param().(selectionTest)
	if len(params.aps) > len(ifaces.AP) {
		return nil, errors.Errorf("test case requires %d AP interfaces, fixture setup only %d",
			len(params.aps), len(ifaces.AP))
	}
	if len(ifaces.Client) < 1 {
		return nil, errors.Errorf("test case requires at least one client interface, fixture setup only %d",
			len(ifaces.Client))
	}

	// Check if one of the credentials uses TLS.
	hasTLS := false
	for _, c := range params.credentials {
		if c.Auth == passpoint.AuthTLS {
			hasTLS = true
			break
		}
	}

	// Create one access point per test network
	var servers []*hostapd.Server
	var expectedServer *hostapd.Server
	for i, ap := range params.aps {
		server := ap.ToServer(ifaces.AP[i], s.OutDir())
		servers = append(servers, server)
		if ap.SSID == params.expectedSSID {
			expectedServer = server
		}
	}

	if expectedServer == nil {
		return nil, errors.Errorf("no match between expected SSID %q and created servers", params.expectedSSID)
	}

	return &selectionTestContext{
		manager:      m,
		aps:          servers,
		credentials:  params.credentials,
		hasTLS:       hasTLS,
		clientIface:  ifaces.Client[0],
		expectedAP:   expectedServer,
		expectedSSID: params.expectedSSID,
	}, nil
}
