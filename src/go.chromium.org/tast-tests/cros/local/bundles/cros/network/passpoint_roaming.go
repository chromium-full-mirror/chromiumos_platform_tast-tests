// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/passpoint"
	"go.chromium.org/tast-tests/cros/local/hostapd"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/wpasupplicant"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// roamingTest describes the parameters of a single test case.
type roamingTest struct {
	// credentials is the of credentials under test.
	credentials *passpoint.Credentials
	// aps is the list of access points to setup for the test.
	aps []passpoint.AccessPoint
}

func init() {
	// Set of tests designed to reproduce Passpoint "roaming" scenario where a
	// device is expected to be able to match successively with different
	// networks using the same set of credentials. It differs from Wi-Fi roaming
	// in that it is not a move from one access point to another depending on
	// link quality (RSSI, SNR, etc) but a move from one network to another
	// using the same set of credentials.  It aims to reproduce the situation
	// where a device with a set of credentials moves from one place to another
	// and need to connect to the Passpoint networks around.
	testing.AddTest(&testing.Test{
		Func:     PasspointRoaming,
		Desc:     "Passpoint network roaming tests",
		Contacts: []string{"cros-networking@google.com", "damiendejean@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFi",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      3 * time.Minute,
		Requirements: []string{tdreq.WiFiGenSupportPasspoint},
		Params: []testing.Param{
			{
				Name: "home_to_home",
				Val: roamingTest{
					credentials: &passpoint.Credentials{
						Domains: []string{passpoint.BlueDomain},
						HomeOIs: []string{passpoint.HomeOI},
						Auth:    passpoint.AuthTTLS,
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-green",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						},
					},
				},
			}, {
				Name: "roaming_to_home",
				Val: roamingTest{
					credentials: &passpoint.Credentials{
						Domains:    []string{passpoint.BlueDomain},
						HomeOIs:    []string{passpoint.HomeOI},
						RoamingOIs: []string{passpoint.RoamingOI1, passpoint.RoamingOI2},
						Auth:       passpoint.AuthTTLS,
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-green",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						},
					},
				},
			}, {
				Name: "home_to_roaming",
				Val: roamingTest{
					credentials: &passpoint.Credentials{
						Domains:    []string{passpoint.BlueDomain},
						HomeOIs:    []string{passpoint.HomeOI},
						RoamingOIs: []string{passpoint.RoamingOI1, passpoint.RoamingOI2},
						Auth:       passpoint.AuthTTLS,
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-red",
							Domain:             passpoint.RedDomain,
							Realms:             []string{passpoint.RedDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI2},
							Auth:               passpoint.AuthTTLS,
						},
					},
				},
			}, {
				Name: "roaming_to_roaming",
				Val: roamingTest{
					credentials: &passpoint.Credentials{
						Domains:    []string{passpoint.BlueDomain},
						HomeOIs:    []string{passpoint.HomeOI},
						RoamingOIs: []string{passpoint.RoamingOI1, passpoint.RoamingOI2},
						Auth:       passpoint.AuthTTLS,
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-green",
							Domain:             passpoint.GreenDomain,
							Realms:             []string{passpoint.GreenDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-red",
							Domain:             passpoint.RedDomain,
							Realms:             []string{passpoint.RedDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI2},
							Auth:               passpoint.AuthTTLS,
						},
					},
				},
			}, {
				Name: "oi_to_oi",
				Val: roamingTest{
					credentials: &passpoint.Credentials{
						Domains: []string{passpoint.BlueDomain},
						HomeOIs: []string{passpoint.HomeOI, passpoint.RoamingOI1, passpoint.RoamingOI2},
						Auth:    passpoint.AuthTTLS,
					},
					aps: []passpoint.AccessPoint{
						{
							SSID:               "passpoint-blue-1",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.HomeOI},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-blue-2",
							Domain:             passpoint.BlueDomain,
							Realms:             []string{passpoint.BlueDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI1, passpoint.RoamingOI2},
							Auth:               passpoint.AuthTTLS,
						}, {
							SSID:               "passpoint-red",
							Domain:             passpoint.RedDomain,
							Realms:             []string{passpoint.RedDomain},
							RoamingConsortiums: []string{passpoint.RoamingOI2},
							Auth:               passpoint.AuthTTLS,
						},
					},
				},
			},
		},
	})
}

// testAP contains information related to a test access point.
type testAP struct {
	// ssid is the name of the network provided by the access point.
	ssid string
	// server is the instance of hostapd that will provide the network.
	server *hostapd.Server
}

// roamingTestContext contains the environment required to run a test case.
type roamingTestContext struct {
	// manager is the proxy to Shill Manager API.
	manager *shill.Manager
	// clientIface is the simulated interface used by Shill.
	clientIface string
	// aps is the list of access point created for the test.
	aps []testAP
	// credentials is the set of Passpoint credentials under test.
	credentials *passpoint.Credentials
}

func PasspointRoaming(ctx context.Context, s *testing.State) {
	// Reserve a little time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	tc, err := prepareRoamingTest(ctx, s)
	if err != nil {
		s.Fatal("Failed to initialize test: ", err)
	}

	// Create a profile dedicated to the test.
	profileName := passpoint.RandomProfileName()
	profile, err := tc.manager.CreateFakeUserProfile(ctx, profileName)
	if err != nil {
		s.Fatal("Failed to create test profile: ", err)
	}
	defer func(ctx context.Context) {
		// Remove the test profile.
		err := tc.manager.RemoveFakeUserProfile(ctx, profileName)
		if err != nil {
			s.Fatal("Failed to clean test profile: ", err)
		}
	}(cleanupCtx)

	// Allow Shill to perform interworking select and match networks.
	if err := tc.manager.SetInterworkingSelectEnabled(ctx, tc.clientIface, true); err != nil {
		s.Fatal("Failed to enable interworking selection: ", err)
	}
	defer tc.manager.SetInterworkingSelectEnabled(ctx, tc.clientIface, false)

	// Obtain a proxy to wpa_supplicant to flush BSS cache during the test.
	wpas, err := wpasupplicant.NewSupplicant(ctx)
	if err != nil {
		s.Fatal("Failed to create wpa_supplicant proxy")
	}
	iface, err := wpas.GetInterface(ctx, tc.clientIface)
	if err != nil {
		s.Fatalf("Failed to obtain %s interface from wpa_supplicant", tc.clientIface)
	}

	// Add the set of credentials to Shill.
	prop, err := tc.credentials.ToShillProperties()
	if err != nil {
		s.Fatal("Failed to get credentials' shill properties: ", err)
	}
	if err := tc.manager.AddPasspointCredentials(ctx, profile, prop); err != nil {
		s.Fatal("Failed to set Passpoint credentials: ", err)
	}

	for _, ap := range tc.aps {
		if err := runApTestCase(ctx, s, tc, ap); err != nil {
			s.Fatalf("Failed to connect to access point %s: %v", ap.ssid, err)
		}
		// Flush the cache of previously scanned networks.
		// As the test does quick AP changes, the previous access point stays in
		// wpa_supplicant cache for a while which causes tests failures (see
		// b/245919543). As the APs are not changing that quickly in real life,
		// so we can safely clean the cache without breaking the test validity.
		iface.FlushBSS(ctx, 0)
	}
}

func runApTestCase(ctx context.Context, s *testing.State, tc *roamingTestContext, ap testAP) (retErr error) {
	// Create the test access point.
	testing.ContextLogf(ctx, "Creating %s", ap.server)
	if err := ap.server.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start access point")
	}
	defer func() {
		if err := ap.server.Stop(); retErr == nil && err != nil {
			retErr = errors.Wrap(err, "failed to stop access point")
		}
	}()

	// Create a monitor to collect access point events.
	testing.ContextLogf(ctx, "Starting monitor for %s", ap.server)
	m := hostapd.NewMonitor()
	if err := m.Start(ctx, ap.server); err != nil {
		return errors.Wrap(err, "failed to start hostapd monitor")
	}
	defer func(ctx context.Context) {
		if err := m.Stop(ctx); retErr == nil && err != nil {
			retErr = errors.Wrap(err, "failed to stop hostapd monitor")
		}
	}(ctx)

	// Reserve time for the monitor cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	wifi, err := shill.NewWifiManager(ctx, tc.manager)
	if err != nil {
		return errors.Wrap(err, "failed to obtain Wi-Fi manager")
	}

	// Delay to wait for a network to be discovered.
	const scanAndWaitTimeout = time.Minute
	// Trigger a scan and wait for the network to be discovered by Shill.
	// b/245919543: a scan request might be refused because a scan is
	// automatically started after the previous AP disappeared. If the new
	// AP appears during the ongoing scan but late, it might not be discovered
	// and the test will fail.
	if _, err := wifi.ScanAndWaitForService(ctx, ap.ssid, scanAndWaitTimeout); err != nil {
		return errors.Wrap(err, "failed to request an active scan")
	}

	// Wait for the station to associate with the access point.
	if err := hostapd.WaitForSTAAssociated(ctx, m, tc.clientIface, hostapd.STAAssociationTimeout); err != nil {
		return errors.Wrap(err, "failed to check station association")
	}

	return nil
}

// prepareRoamingTest creates a test profile, reads the test parameters and delivers a test context.
func prepareRoamingTest(ctx context.Context, s *testing.State) (*roamingTestContext, error) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to shill Manager")
	}

	// Obtain the test case.
	params := s.Param().(roamingTest)

	// Obtain the simulated interfaces from the fixture environment.
	ifaces := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	if len(ifaces.AP) < len(params.aps) {
		return nil, errors.Wrapf(err, "roaming test require at least %d simulated interface", len(params.aps))
	}
	if len(ifaces.Client) < 1 {
		return nil, errors.Wrap(err, "roaming test require at least one simulated client interface")
	}

	// Create one access point per test network.
	var aps []testAP
	for i, ap := range params.aps {
		server := ap.ToServer(ifaces.AP[i], s.OutDir())
		aps = append(aps, testAP{
			ssid:   ap.SSID,
			server: server,
		})
	}

	return &roamingTestContext{
		manager:     m,
		clientIface: ifaces.Client[0],
		credentials: params.credentials,
		aps:         aps,
	}, nil
}
