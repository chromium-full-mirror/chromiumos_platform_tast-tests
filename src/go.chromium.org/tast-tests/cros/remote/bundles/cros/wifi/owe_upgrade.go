// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/owe"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	ap "go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type apConf struct {
	apOpts    []ap.Option
	secConfig security.ConfigFactory
}

type apDef struct {
	name             string
	configs          []apConf
	expectedSecurity string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: OweUpgrade,
		Desc: "Verifies connectivity to network that has security upgraded from None to TransOwe and then to Owe",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesCapture),
		HardwareDeps:    hwdep.D(hwdep.WifiNotMarvell()),
		Requirements:    []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiGenSupportPMF, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

// OweUpgrade checks connectivity to a public network that is:
// - initially configured to be an open network,
// - then has configuration changed to be transitional mode OWE,
// - and then switches to the pure OWE network.
// For each configuration it is expected that we will be able to connect w/o any
// problems so the steps are:
//  1. Configure an open AP.
//  2. Connect, test connection, disconnect and deconfigure the AP.
//  3. Configure transitional mode OWE AP with public BSS having the same SSID
//     as in point 1 and the hidden one using different name.
//  4. Connect, test connection, disconnect and deconfigure both BSSes.
//  5. Configure a pure OWE AP using the same SSID as in point 1.
//  6. Connect, test connection, disconnect and deconfigure the AP.
//  7. Configure an open AP again (simulating a downgrade).
//  8. Connect, test connection, disconnect and deconfigure the AP.
func OweUpgrade(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	servicePath := ""

	// Get the name of the DUT WiFi interface (for BSS flushing/discovery).
	clientIface, err := tf.ClientInterface(ctx)
	if err != nil {
		s.Fatal("Failed to get DUT interface: ", err)
	}

	// Additional scans are not a problem during security upgrades but for the downgrade
	// (owe->public) there is possible following race scenario:
	// - we deconfigure OWE AP,
	// - flush the BSSes,
	// - the background scan results come with BSSID of just deconfigured AP
	// Then the security of the service will be still be matching the higher security (OWE)
	// while the actual connection will be made to the public endpoint (since those late results
	// are not really available).  To avoid that problem let's turn off autonomous scanning for
	// the test and rely on explicit requests.
	ctx, restoreBgAndFg, err := tf.DUTWifiClient(wificell.DefaultDUT).TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	connectAP := func(ctx context.Context, apDef *apDef) (retErr error) {
		var ap *wificell.APIface
		var cleanupCtx context.Context
		var cancel context.CancelFunc

		for _, cfg := range apDef.configs {
			ap, err = tf.ConfigureAP(ctx, cfg.apOpts, cfg.secConfig)
			if err != nil {
				return errors.Wrap(err, "failed to configure the AP")
			}
			s.Log("Configured BSS with BSSID: ", ap.Config().BSSID)
			cleanupCtx = ctx
			ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
			defer cancel()

			defer func(ctx context.Context) {
				if err := tf.DeconfigAP(ctx, ap); err != nil {
					retErr = errors.Wrap(err, "failed to deconfig the AP")
				}
			}(cleanupCtx)
		}
		// Discover the lastly configured BSS to make sure everything is up and running.
		bssid := ap.Config().BSSID
		ssid := ap.Config().SSID
		if err := tf.DUTWifiClient(wificell.DefaultDUT).DiscoverBSSID(ctx, bssid, clientIface, []byte(ssid)); err != nil {
			return errors.Wrap(err, "failed to discover AP")
		}
		connResp, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap)
		if err != nil {
			return errors.Wrap(err, "failed to connect to WiFi")
		}
		cleanupCtx = ctx
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()
		defer func(ctx context.Context) {
			if err := tf.DisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
				retErr = errors.Wrap(err, "failed to disconnect WiFi")
			}
		}(cleanupCtx)

		if servicePath == "" {
			servicePath = connResp.ServicePath
		} else if servicePath != connResp.ServicePath {
			return errors.Errorf("Service path has changed: got %s, want %s", connResp.ServicePath, servicePath)
		}

		srvcResp, err := tf.DUTWifiClient(wificell.DefaultDUT).QueryService(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get service properties")
		}
		s.Logf("Connected to %s with Security: %s", connResp.ServicePath, srvcResp.Wifi.Security)
		if srvcResp.Wifi.Security != apDef.expectedSecurity {
			return errors.Errorf("Wrong service security: got %s, want %s",
				srvcResp.Wifi.Security, apDef.expectedSecurity)
		}
		return nil
	}

	ssid := ap.RandomSSID("TAST_TEST_")
	commonApOpts := []ap.Option{ap.SSID(ssid), ap.Mode(ap.Mode80211g), ap.Channel(1)}
	bssOpen, err := ap.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSSID: ", err)
	}
	bssOwe, err := ap.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSSID: ", err)
	}
	oweSsid := ap.RandomSSID("TAST_TEST_OWE_")

	apDefs := []apDef{
		{
			name: "public",
			configs: []apConf{{
				apOpts: commonApOpts,
			}},
			expectedSecurity: shillconst.SecurityNone,
		},
		{
			name: "transitional OWE",
			configs: []apConf{{
				apOpts:    append(commonApOpts[1:], ap.SSID(oweSsid), ap.BSSID(bssOwe.String())),
				secConfig: owe.NewConfigFactory(owe.ModeTransOWE, owe.SSID(ssid), owe.BSSID(bssOpen)),
			}, { // Open BSS should be last (so that hidden is ready when open announces it).
				apOpts:    append(commonApOpts, ap.BSSID(bssOpen.String())),
				secConfig: owe.NewConfigFactory(owe.ModeTransOpen, owe.SSID(oweSsid), owe.BSSID(bssOwe)),
			}},
			expectedSecurity: shillconst.SecurityTransOWE,
		}, {
			name: "pure OWE",
			configs: []apConf{{
				apOpts:    commonApOpts,
				secConfig: owe.NewConfigFactory(owe.ModePureOWE),
			}},
			expectedSecurity: shillconst.SecurityOWE,
		},
	}

	apNum := len(apDefs)
	// This +1 is intentional - it is supposed to test a downgrade back to
	// public (for example too many user complains after an upgrade to owe
	// and this causes the admin to revert the upgrade and go back to an
	// open network).
	for i := 0; i < apNum+1; i++ {
		idx := i % apNum
		s.Log("Testing connection to: ", apDefs[idx].name)
		if err = connectAP(ctx, &apDefs[idx]); err != nil {
			s.Fatal("Failure during AP connection: ", err)
		}
		// The connectAP function above disconnects and deconfigures the
		// AP, but on disconnect shill triggers the scan automatically,
		// so it might catch existing BSSes right before they get
		// removed.  If the scan takes long time these results might
		// come after we have flushed BSSes below and after configuring
		// the new APs (in the next connectAP invocation) giving
		// incorrect view of the RF environment.  To avoid that let's
		// wait until the scan finishes before continuing.
		if _, err := tf.DUTWifiClient(wificell.DefaultDUT).WaitScanIdle(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to wait for current scan to be done: ", err)
		}
		s.Log("Flushing BSS cache")
		if err := tf.DUTWifiClient(wificell.DefaultDUT).FlushBSS(ctx, clientIface, 0); err != nil {
			s.Fatal("Failed to flush BSS list: ", err)
		}
	}
}
