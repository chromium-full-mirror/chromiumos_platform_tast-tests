// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/wifi/passpoint"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/hostapd"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// passpointARCDialogTestParams is the parameter of PasspointARCDialog test.
type passpointARCDialogTestParams struct {
	allow  bool
	legacy bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:     PasspointARCDialog,
		Desc:     "Checks if Passpoint dialog works on ARC",
		Contacts: []string{"cros-networking@google.com", "jasongustaman@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Fixture:      "shillSimulatedWiFiWithArcBootedWithPasspoint",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi", "shill-wifi", "chrome", "arc"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "allow",
			Val: passpointARCDialogTestParams{
				allow: true,
			},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name: "allow_legacy_sdk",
			Val: passpointARCDialogTestParams{
				allow:  true,
				legacy: true,
			},
		}, {
			Name: "dont_allow",
			Val: passpointARCDialogTestParams{
				allow: false,
			},
			ExtraSoftwareDeps: []string{"android_vm"},
		}, {
			Name: "dont_allow_legacy_sdk",
			Val: passpointARCDialogTestParams{
				allow:  false,
				legacy: true,
			},
		}},
		Timeout:      7 * time.Minute,
		Requirements: []string{tdreq.WiFiGenSupportPasspoint},
	})
}

// PasspointARCDialog tests the dialog created on app's Passpoint request:
// 1. Create Passpoint credentials.
// 2. Add the credentials through either the suggestion or legacy API.
// 3. Wait for the dialog to show up.
// 4. Allow / reject the request.
// 5. Wait for connection to ensure that the credentials are added.
func PasspointARCDialog(ctx context.Context, s *testing.State) {
	const (
		apkP = "ArcPasspointTest_P.apk"
		apkR = "ArcPasspointTest_R.apk"
		pkg  = "org.chromium.arc.testapp.passpoint"
		cls  = "org.chromium.arc.testapp.passpoint.MainActivity"

		legacyButtonID     = "org.chromium.arc.testapp.passpoint:id/button_legacy"
		suggestionButtonID = "org.chromium.arc.testapp.passpoint:id/button_suggestion"

		allowButton     = "Allow"
		dontAllowButton = "Don't allow"

		// Fully qualified domain name used to connect to the AP.
		// This value must match the domain of the certificate used by the AP, go.chromium.org/tast-tests/cros/common/crypto/certificate TestCert1().
		// This is because ARC fills its EAP domain suffix match with Passpoint credentials' FQDN.
		fqdn = "chromelab-wifi-testbed-server.mtv.google.com"

		fileName = "test.wificonfig"
	)

	params := s.Param().(passpointARCDialogTestParams)
	apk := apkR
	buttonID := suggestionButtonID
	if params.legacy {
		apk = apkP
		buttonID = legacyButtonID
	}

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

	// ARC-related setup.
	a := s.FixtValue().(*hwsim.ShillSimulatedWiFi).ARC
	d := s.FixtValue().(*hwsim.ShillSimulatedWiFi).UIDevice

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
	defer m.SetInterworkingSelectEnabled(ctx, ifaces.Client[0], false)
	// Start AP.
	ap := passpoint.AccessPoint{
		SSID:               "passpoint-ttls-home-oi",
		Domain:             fqdn,
		Realms:             []string{fqdn},
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

	// Provision Passpoint credentials from ARC.
	creds := passpoint.Credentials{
		Domains: []string{fqdn},
		HomeOIs: []string{passpoint.HomeOI},
		Auth:    passpoint.AuthTTLS,
	}
	config, err := creds.ToAndroidConfig(ctx)
	if err != nil {
		s.Fatal("Failed to create Android Passpoint config: ", err)
	}

	// Create Passpoint config file in "Downloads".
	ownerID, err := cryptohome.UserHash(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user hash: ", err)
	}
	filePath := filepath.Join("/home/user", ownerID, "Downloads", fileName)
	if err := ioutil.WriteFile(filePath, []byte(config), 0644); err != nil {
		s.Fatal("Failed to write Passpoint config: ", err)
	}
	defer os.Remove(filePath)

	// Start ARC Passpoint app.
	s.Log("Installing Passpoint app")
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}
	defer func() {
		testing.ContextLog(cleanupCtx, "Uninstalling Passpoint app")
		if err := a.Uninstall(cleanupCtx, pkg); err != nil {
			s.Fatal("Failed to uninstall Passpoint app: ", err)
		}
	}()
	s.Log("Starting app")
	activity, err := arc.NewActivity(a, pkg, cls)
	if err != nil {
		s.Fatal("Failed creating app activity: ", err)
	}
	defer activity.Close(ctx)
	if err := activity.Start(ctx, tconn); err != nil {
		s.Fatal("Failed to start test activity: ", err)
	}
	defer activity.Stop(cleanupCtx, tconn)

	// Add Passpoint credentials through the app.
	s.Log("Adding Passpoint credentials")
	if err := d.Object(ui.ID(buttonID)).WaitForExists(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to start app: ", err)
	}
	if err := d.Object(ui.ID(buttonID)).Click(ctx); err != nil {
		s.Error("Failed click add Passpoint button: ", err)
	}
	defer func() {
		a.Command(cleanupCtx, "cmd", "wifi", "remove-passpoint-config", creds.FQDN()).Run(testexec.DumpLogOnError)
	}()
	// Open the Passpoint credentials.
	downloads := nodewith.Name("Downloads").First()
	file := nodewith.Name(fileName).First()
	if err := uiauto.Combine("open Passpoint credentials file",
		ac.WaitUntilExists(downloads),
		ac.DoDefaultUntil(downloads, ac.Exists(file)),
		ac.RetryUntil(ac.DoubleClick(file), ac.Gone(file)),
	)(ctx); err != nil {
		s.Fatal("Failed to open Passpoint credentials file: ", err)
	}

	// Allow or reject the app from provisioning the Passpoint credentials.
	// TODO(b/271777307): Use node's ID instead of name.
	var button *nodewith.Finder
	if params.allow {
		button = nodewith.Name(allowButton).First()
	} else {
		button = nodewith.Name(dontAllowButton).First()
	}
	if err := uiauto.Combine("accept or reject the app's request",
		ac.WaitUntilExists(button),
		ac.DoDefaultUntil(button, ac.Gone(button)),
	)(ctx); err != nil {
		s.Fatal("Failed to accept or reject app's request: ", err)
	}

	// Check for connection.
	wifi, err := shill.NewWifiManager(ctx, m)
	if err != nil {
		s.Fatal("Failed to obtain Wi-Fi manager: ", err)
	}
	// Delay to wait for a network to be discovered.
	// Trigger a scan.
	if _, err := wifi.ScanAndWaitForService(ctx, ap.SSID, time.Minute); err != nil {
		s.Fatal("Failed to request an active scan: ", err)
	}
	// Wait for the station to associate with the access point.
	if err := hostapd.WaitForSTAAssociated(ctx, h, ifaces.Client[0], hostapd.STAAssociationTimeout); err != nil && params.allow {
		s.Fatal("Failed to wait for STA association: ", err)
	} else if err == nil && !params.allow {
		s.Fatal("Unwanted STA association to the AP")
	}
}
