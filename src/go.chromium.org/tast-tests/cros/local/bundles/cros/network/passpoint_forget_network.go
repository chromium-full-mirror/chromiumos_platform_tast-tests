// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/netconfigtypes"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/passpoint"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/hostapd"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     PasspointForgetNetwork,
		Desc:     "Checks if Passpoint credentials are removed after removing a Passpoint network",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFiWithChromeLoggedInWithPasspoint",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi", "chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      7 * time.Minute,
		Requirements: []string{tdreq.WiFiGenSupportPasspoint},
	})
}

// PasspointForgetNetwork tests that Passpoint credentials are forgotten after
// its network is forgotten:
// 1. Add Passpoint credentials to shill
// 3. Wait for connection to the AP.
// 4. Forget the connected network.
// 5. Ensure that there is no connection to the AP anymore.
func PasspointForgetNetwork(ctx context.Context, s *testing.State) {
	// Reserve a little time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
	defer cancel()

	// Chrome-related setup.
	cr := s.FixtValue().(*hwsim.ShillSimulatedWiFi).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}
	ac := uiauto.New(tconn)

	// Shill-related setup.
	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}
	ifaces := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	if len(ifaces.AP) < 1 {
		s.Fatal("Test requires at least one simulated interface")
	}
	if len(ifaces.Client) < 1 {
		s.Fatal("Test requires at least one simulated client interface")
	}
	// Allow Shill to perform interworking select and match networks.
	if err := m.SetInterworkingSelectEnabled(ctx, ifaces.Client[0], true); err != nil {
		s.Fatal("Failed to enable interworking selection: ", err)
	}
	defer m.SetInterworkingSelectEnabled(cleanupCtx, ifaces.Client[0], false)
	// Start AP.
	ap := passpoint.AccessPoint{
		SSID:               "TestSSID",
		Domain:             passpoint.BlueDomain,
		Realms:             []string{passpoint.BlueDomain},
		RoamingConsortiums: []string{passpoint.HomeOI},
		Auth:               passpoint.AuthTTLS,
	}
	server := ap.ToServer(ifaces.AP[0], s.OutDir())
	if err := server.Start(ctx); err != nil {
		s.Fatal("Failed to start access point: ", err)
	}
	defer server.Stop()
	// Create a monitor to collect access point events.
	h := hostapd.NewMonitor()
	if err := h.Start(ctx, server); err != nil {
		s.Fatal("Failed to start hostapd monitor: ", err)
	}
	defer func() {
		if err := h.Stop(cleanupCtx); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to stop hostapd monitor: ", err)
		}
	}()
	// Get shill profile for the active user.
	profile, err := m.WaitForUserProfile(ctx)
	if err != nil {
		s.Fatal("Failed to wait for shill user profile: ", err)
	}

	// Add the sets of credentials to Shill.
	creds := passpoint.Credentials{
		Domains: []string{passpoint.BlueDomain},
		Auth:    passpoint.AuthTTLS,
	}
	prop, err := creds.ToShillProperties()
	if err != nil {
		s.Fatal("Failed to get credentials' shill properties: ", err)
	}
	if err := m.AddPasspointCredentials(ctx, profile, prop); err != nil {
		s.Fatal("Failed to set Passpoint credentials: ", err)
	}

	// Check for connection.
	wm, err := shill.NewWifiManager(ctx, m)
	if err != nil {
		s.Fatal("Failed to obtain Wi-Fi manager: ", err)
	}
	// Delay to wait for a network to be discovered.
	// Trigger a scan.
	if _, err := wm.ScanAndWaitForService(ctx, ap.SSID, time.Minute); err != nil {
		s.Fatal("Failed to request an active scan: ", err)
	}
	// Wait for the station to associate with the access point.
	if err := hostapd.WaitForSTAAssociated(ctx, h, ifaces.Client[0], hostapd.STAAssociationTimeout); err != nil {
		s.Fatal("Failed to wait for STA association: ", err)
	}

	// Forget network.
	if _, err := ossettings.OpenNetworkDetailPage(ctx, tconn, cr, ap.SSID, netconfigtypes.WiFi); err != nil {
		s.Fatal("Failed to oen WiFi network page: ", err)
	}
	// Open the forget dialog and confirm subscription removal.
	forget := nodewith.Name("Forget").First()
	confirm := nodewith.Name("Confirm").First()
	if err := uiauto.Combine("forget current active WiFi network",
		ac.WaitUntilExists(forget),
		ac.RetryUntil(ac.LeftClick(forget), ac.Exists(confirm)),
		ac.RetryUntil(ac.LeftClick(confirm), ac.Gone(confirm)),
	)(ctx); err != nil {
		s.Fatal("Failed to forget current active WiFi network: ", err)
	}

	// Delay to wait for a network to be discovered.
	// Trigger a scan.
	if _, err := wm.ScanAndWaitForService(ctx, ap.SSID, time.Minute); err != nil {
		s.Fatal("Failed to request an active scan: ", err)
	}
	// Device should not reconnect to Passpoint networks.
	if err := hostapd.WaitForSTAAssociated(ctx, h, ifaces.Client[0], hostapd.STAAssociationTimeout); err == nil {
		s.Fatal("Unexpected STA association. Expected Passpoint credentials to be removed")
	}
}
