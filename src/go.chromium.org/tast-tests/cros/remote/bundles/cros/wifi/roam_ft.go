// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/tbdep"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpaeap"
	"go.chromium.org/tast-tests/cros/remote/wifi/iw"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type roamFTparam struct {
	apOpts     []hostapd.Option
	secConfFac security.ConfigFactory
	mixed      bool
}

var (
	roamFTCert1 = certificate.TestCert1()
	roamFTCert2 = certificate.TestCert2()
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RoamFT,
		Desc: "Verifies that DUT can roam with FT auth suites",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent:    "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:            []string{"group:wificell", "wificell_func"},
		TestBedDeps:     []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:     []string{wificell.ShillServiceName},
		Fixture:         wificell.FixtureID(wificell.TFFeaturesBridgeAndVeth),
		Requirements:    []string{tdreq.WiFiGenSupportMBO, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
		Params: []testing.Param{{
			Name: "psk",
			Val: roamFTparam{
				secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP), wpa.FTMode(wpa.FTModePure)),
			},
		}, {
			Name: "mixed_psk",
			Val: roamFTparam{
				secConfFac: wpa.NewConfigFactory("chromeos", wpa.Mode(wpa.ModePureWPA2), wpa.Ciphers2(wpa.CipherCCMP), wpa.FTMode(wpa.FTModeMixed)),
				mixed:      true,
			},
		}, {
			Name: "eap",
			Val: roamFTparam{
				secConfFac: wpaeap.NewConfigFactory(
					roamFTCert1.CACred.Cert, roamFTCert1.ServerCred,
					wpaeap.ClientCACert(roamFTCert1.CACred.Cert), wpaeap.ClientCred(roamFTCert1.ClientCred),
					wpaeap.Mode(wpa.ModePureWPA2), wpaeap.FTMode(wpa.FTModePure),
				),
			},
		}, {
			Name: "mixed_eap",
			Val: roamFTparam{
				secConfFac: wpaeap.NewConfigFactory(
					roamFTCert1.CACred.Cert, roamFTCert1.ServerCred,
					wpaeap.ClientCACert(roamFTCert1.CACred.Cert), wpaeap.ClientCred(roamFTCert1.ClientCred),
					wpaeap.Mode(wpa.ModePureWPA2), wpaeap.FTMode(wpa.FTModeMixed),
				),
				mixed: true,
			},
		}, {
			Name:              "sae",
			ExtraSoftwareDeps: []string{"wpa3_sae"},
			// TODO: b/323077686 - Stabilize wifi.RoamFT.sae and wpa3_eap tests
			ExtraAttr: []string{"wificell_unstable"},
			Val: roamFTparam{
				apOpts: []hostapd.Option{
					hostapd.PMF(hostapd.PMFRequired),
				},
				secConfFac: wpa.NewConfigFactory(
					"chromeos", wpa.Mode(wpa.ModePureWPA3),
					wpa.Ciphers2(wpa.CipherCCMP), wpa.FTMode(wpa.FTModePure),
				),
			},
		}, {
			Name:              "mixed_sae",
			ExtraSoftwareDeps: []string{"wpa3_sae"},
			// TODO: b/323077686 - Stabilize wifi.RoamFT.sae and wpa3_eap tests
			ExtraAttr: []string{"wificell_unstable"},
			Val: roamFTparam{
				apOpts: []hostapd.Option{
					hostapd.PMF(hostapd.PMFRequired),
				},
				secConfFac: wpa.NewConfigFactory(
					"chromeos", wpa.Mode(wpa.ModePureWPA3),
					wpa.Ciphers2(wpa.CipherCCMP), wpa.FTMode(wpa.FTModeMixed),
				),
				mixed: true,
			},
		},
		},
	})
}

func RoamFT(ctx context.Context, s *testing.State) {
	/*
	   RoamFT tests the ability to perform fast roaming, where the DUT
	   doesn’t go through the whole regular association process during
	   roaming. The ordering of RoamFT is as follows:
	   1. Set up the bridge for the communication between ap0 and ap1.
	   2. Start ap0 and the DHCP server.
	   3. Connect the DUT to ap0 and verify the connection.
	   4. Start a property watcher on the DUT.
	   5. Start ap1.
	   6. ap0 sends a BSS transition management (TM) request to the DUT.
	   7. Upon the reception of the BSS TM request, the DUT scans and
	      transitions to ap1.
	   8. Check the property watcher and see if the DUT stays connected
	      during the roaming.
	   9. Verify the connection to ap1.
	*/
	tf := s.FixtValue().(*wificell.TestFixture)

	router, err := tf.StandardRouter()
	if err != nil {
		s.Fatal("Failed to get router: ", err)
	}

	br, err := tf.GetBridgesOnRouterID(wificell.DefaultRouter)
	if err != nil {
		s.Fatal("Failed to get bridge names on router: ", err)
	}

	apID := 0
	uniqueAPName := func() string {
		id := strconv.Itoa(apID)
		apID++
		return id
	}

	clientMAC, err := tf.ClientHardwareAddr(ctx)
	if err != nil {
		s.Fatal("Unable to get DUT MAC address: ", err)
	}

	// runOnce sets up the network environment as mentioned above, and verifies the DUT is able to roam between the APs iff expectedFailure is not set.
	runOnce := func(ctx context.Context, apOpts []hostapd.Option, secConfFac security.ConfigFactory, expectedFailure bool) {
		var cancel context.CancelFunc
		var err error

		mac0, err := hostapd.RandomMAC()
		if err != nil {
			s.Fatal("Failed to get a random mac address: ", err)
		}
		mac1, err := hostapd.RandomMAC()
		if err != nil {
			s.Fatal("Failed to get a random mac address: ", err)
		}
		var (
			id0 = hex.EncodeToString(mac0)
			id1 = hex.EncodeToString(mac1)
		)
		const (
			key0 = "1f1e1d1c1b1a191817161514131211100f0e0d0c0b0a09080706050403020100"
			key1 = "000102030405060708090a0b0c0d0e0f101112131415161718191a1b1c1d1e1f"
			mdID = "a1b2"
		)

		ap0SecConf, err := secConfFac.Gen()
		if err != nil {
			s.Fatal("Failed to generate security config: ", err)
		}
		ap0Ops := []hostapd.Option{
			hostapd.Channel(1), hostapd.Mode(hostapd.Mode80211g), hostapd.BSSID(mac0.String()),
			hostapd.MobilityDomain(mdID), hostapd.NASIdentifier(id0), hostapd.R1KeyHolder(id0),
			hostapd.R0KHs(fmt.Sprintf("%s %s %s", mac1, id1, key0)),
			hostapd.R1KHs(fmt.Sprintf("%s %s %s", mac1, mac1, key1)),
			hostapd.Bridge(br[0]), hostapd.SecurityConfig(ap0SecConf),
		}
		ap0Ops = append(ap0Ops, apOpts...)
		ap0Conf, err := hostapd.NewConfig(ap0Ops...)
		if err != nil {
			s.Fatal("Failed to generate the hostapd config for AP0: ", err)
		}
		ap1SecConf, err := secConfFac.Gen()
		if err != nil {
			s.Fatal("Failed to generate security config: ", err)
		}
		ap1Ops := []hostapd.Option{
			hostapd.SSID(ap0Conf.SSID), hostapd.Channel(157), hostapd.Mode(hostapd.Mode80211acPure), hostapd.BSSID(mac1.String()),
			hostapd.HTCaps(hostapd.HTCapHT40Plus), hostapd.VHTCaps(hostapd.VHTCapSGI80), hostapd.VHTChWidth(hostapd.VHTChWidth80), hostapd.VHTCenterChannel(155),
			hostapd.MobilityDomain(mdID), hostapd.NASIdentifier(id1), hostapd.R1KeyHolder(id1),
			hostapd.R0KHs(fmt.Sprintf("%s %s %s", mac0, id0, key1)),
			hostapd.R1KHs(fmt.Sprintf("%s %s %s", mac0, mac0, key0)),
			hostapd.Bridge(br[1]), hostapd.SecurityConfig(ap1SecConf),
		}
		ap1Ops = append(ap1Ops, apOpts...)
		ap1Conf, err := hostapd.NewConfig(ap1Ops...)
		if err != nil {
			s.Fatal("Failed to generate the hostapd config for AP1: ", err)
		}

		s.Log("Starting the first AP on ", br[0])
		ap0Name := uniqueAPName()
		ap0, err := router.StartHostapd(ctx, ap0Name, ap0Conf)
		if err != nil {
			s.Fatal("Failed to start the hostapd server on the first AP: ", err)
		}
		defer func(ctx context.Context) {
			if err := router.StopHostapd(ctx, ap0); err != nil {
				s.Error("Failed to stop the hostapd server on the first AP: ", err)
			}
		}(ctx)
		ctx, cancel = ap0.ReserveForClose(ctx)
		defer cancel()

		var (
			serverIP    = net.IPv4(192, 168, 0, 254)
			startIP     = net.IPv4(192, 168, 0, 1)
			endIP       = net.IPv4(192, 168, 0, 128)
			broadcastIP = net.IPv4(192, 168, 0, 255)
			mask        = net.IPv4Mask(255, 255, 255, 0)
		)
		s.Logf("Starting the DHCP server on %s, serverIP=%s", br[0], serverIP)
		ds, err := router.StartDHCP(ctx, ap0Name, br[0], startIP, endIP, serverIP, broadcastIP, mask, nil)
		if err != nil {
			s.Fatal("Failed to start the DHCP server: ", err)
		}
		defer func(ctx context.Context) {
			if err := router.StopDHCP(ctx, ds); err != nil {
				s.Error("Failed to stop the DHCP server: ", err)
			}
		}(ctx)
		ctx, cancel = ds.ReserveForClose(ctx)
		defer cancel()

		connResp, err := tf.ConnectWifi(ctx, ap0.Config().SSID, dutcfg.ConnSecurity(ap0SecConf))
		if err != nil {
			if expectedFailure {
				s.Log("Failed to connect to the AP as expected; Tearing down")
				return
			}
			s.Fatal("Failed to connect to the AP: ", err)
		}
		if expectedFailure {
			s.Fatal("Expected failure but succeeded")
		}
		roamSucceeded := false
		defer func(ctx context.Context) {
			if roamSucceeded {
				return
			}
			if err := tf.CleanDisconnectWifi(ctx); err != nil {
				s.Error("Failed to disconnect from the AP: ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()

		if err := tf.PingFromDUT(ctx, serverIP.String()); err != nil {
			s.Fatal("Failed to ping from the DUT: ", err)
		}

		s.Log("Connected to the first AP; Start roaming")
		props := []*wificell.ShillProperty{{
			Property:       shillconst.ServicePropertyWiFiRoamState,
			ExpectedValues: []interface{}{shillconst.RoamStateConfiguration},
			Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
		}, {
			Property:       shillconst.ServicePropertyWiFiRoamState,
			ExpectedValues: []interface{}{shillconst.RoamStateReady},
			Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
		}, {
			Property:       shillconst.ServicePropertyWiFiRoamState,
			ExpectedValues: []interface{}{shillconst.RoamStateIdle},
			Method:         wifi.ExpectShillPropertyRequest_ON_CHANGE,
		}, {
			Property:       shillconst.ServicePropertyWiFiBSSID,
			ExpectedValues: []interface{}{mac1.String()},
			Method:         wifi.ExpectShillPropertyRequest_CHECK_ONLY,
		}}
		waitCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		waitForProps, err := tf.WifiClient().ExpectShillProperty(waitCtx, connResp.ServicePath, props, []string{shillconst.ServicePropertyIsConnected})
		if err != nil {
			s.Fatal("Failed to create a property watcher: ", err)
		}

		s.Log("Starting the second AP on ", br[1])
		ap1Name := uniqueAPName()
		ap1, err := router.StartHostapd(ctx, ap1Name, ap1Conf)
		if err != nil {
			s.Fatal("Failed to start the hostapd server on the second AP: ", err)
		}
		defer func(ctx context.Context) {
			if err := router.StopHostapd(ctx, ap1); err != nil {
				s.Error("Failed to stop the hostapd server on the second AP: ", err)
			}
		}(ctx)
		ctx, cancel = ap1.ReserveForClose(ctx)
		defer cancel()

		s.Logf("Sending BSS TM Request from AP %s to DUT %s", mac0, clientMAC)
		req := hostapd.BSSTMReqParams{Neighbors: []string{mac1.String()}}
		if err := ap0.SendBSSTMRequest(ctx, clientMAC, req); err != nil {
			s.Fatal("Failed to send BSS TM Request: ", err)
		}

		monitorResult, err := waitForProps()
		if err != nil {
			s.Fatal("Failed to wait for the properties: ", err)
		}
		roamSucceeded = true
		defer func(ctx context.Context) {
			if err := tf.CleanDisconnectWifi(ctx); err != nil {
				s.Error("Failed to disconnect from the AP: ", err)
			}
		}(ctx)
		// Check that we don't disconnect along the way here, in case we're ping-ponging around APs --
		// and after the first (failed) roam, the second re-connection will not be testing FT at all.
		for _, ph := range monitorResult {
			if ph.Name == shillconst.ServicePropertyIsConnected {
				if !ph.Value.(bool) {
					s.Error("Failed to stay connected during the roaming process")
				}
			}
		}

		// Verify the L3 connectivity and make sure that the DUT stays connected to the second AP.
		if err := tf.PingFromDUT(ctx, serverIP.String()); err != nil {
			s.Fatal("Failed to verify connection: ", err)
		}
		dutState, err := tf.WifiClient().QueryService(ctx)
		if err != nil {
			s.Fatal("Failed to query service: ", err)
		}
		if dutState.Wifi.Bssid != mac1.String() {
			s.Fatalf("Unexpected BSSID: got %s, want %s", dutState.Wifi.Bssid, mac1)
		}
	}
	hasFTSupport := func(ctx context.Context) bool {
		phys, _, err := iw.NewRemoteRunner(s.DUT().Conn()).ListPhys(ctx)
		if err != nil {
			s.Fatal("Failed to check SME capability: ", err)
		}
		for _, p := range phys {
			for _, c := range p.Commands {
				// A DUT which has SME capability should support FT.
				if c == "authenticate" {
					return true
				}
				// A full-mac driver that supports update_ft_ies functions also supports FT.
				if c == "update_ft_ies" {
					return true
				}
			}
		}
		return false
	}

	ctx, restoreBgAndFg, err := tf.WifiClient().TurnOffBgAndFgscan(ctx)
	if err != nil {
		s.Fatal("Failed to turn off the background and/or foreground scan: ", err)
	}
	defer func() {
		if err := restoreBgAndFg(); err != nil {
			s.Error("Failed to restore the background and/or foreground scan config: ", err)
		}
	}()

	allowRoamResp, err := tf.WifiClient().GetScanAllowRoamProperty(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get the ScanAllowRoam property: ", err)
	}
	if allowRoamResp.Allow {
		if _, err := tf.WifiClient().SetScanAllowRoamProperty(ctx, &wifi.SetScanAllowRoamPropertyRequest{Allow: false}); err != nil {
			s.Error("Failed to set ScanAllowRoam property to false: ", err)
		}
		defer func(ctx context.Context) {
			if _, err := tf.WifiClient().SetScanAllowRoamProperty(ctx, &wifi.SetScanAllowRoamPropertyRequest{Allow: allowRoamResp.Allow}); err != nil {
				s.Errorf("Failed to set ScanAllowRoam property back to %v: %v", allowRoamResp.Allow, err)
			}
		}(ctx)
	}

	ftResp, err := tf.WifiClient().GetGlobalFTProperty(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get the global FT property: ", err)
	}
	defer func(ctx context.Context) {
		if _, err := tf.WifiClient().SetGlobalFTProperty(ctx, &wifi.SetGlobalFTPropertyRequest{Enabled: ftResp.Enabled}); err != nil {
			s.Errorf("Failed to set global FT property back to %v: %v", ftResp.Enabled, err)
		}
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, time.Second)
	defer cancel()

	param := s.Param().(roamFTparam)
	// Turn on the global FT and test once.
	if _, err := tf.WifiClient().SetGlobalFTProperty(ctx, &wifi.SetGlobalFTPropertyRequest{Enabled: true}); err != nil {
		s.Fatal("Failed to turn on the global FT property: ", err)
	}
	// Expect failure if we are running pure FT test and the DUT is not supporting SME.
	runOnce(ctx, param.apOpts, param.secConfFac, !param.mixed && !hasFTSupport(ctx))
	// Run the test without global FT. It should pass iff we configured the AP in mixed mode.
	if _, err := tf.WifiClient().SetGlobalFTProperty(ctx, &wifi.SetGlobalFTPropertyRequest{Enabled: false}); err != nil {
		s.Fatal("Failed to turn off the global FT property: ", err)
	}
	runOnce(ctx, param.apOpts, param.secConfFac, !param.mixed)
}
