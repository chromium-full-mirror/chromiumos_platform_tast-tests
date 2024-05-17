// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/testing"
)

// bssidProperty represents the network config properties that contain BSSIDs.
type bssidProperty int

const (
	allowlist bssidProperty = iota
	requested
)

type bssidPropertiesTestCase struct {
	apMatch          []bssidProperty
	apNotMatch       []bssidProperty
	configProps      map[string]interface{}
	expectConnection bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: BSSIDProperties,
		Desc: "Checks that BSSIDAllowlist and BSSIDRequested properties take effect separately and together",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps:  []string{wificell.ShillServiceName},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Requirements: []string{tdreq.WiFiGenSupportWiFi, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		Params: []testing.Param{{
			// The BSSID allowlist property matches the AP's BSSID.
			Name: "allowlist",
			Val: bssidPropertiesTestCase{
				apMatch:          []bssidProperty{allowlist},
				expectConnection: true,
			},
		}, {
			// The BSSID requested property matches the AP's BSSID.
			Name: "requested",
			Val: bssidPropertiesTestCase{
				apMatch:          []bssidProperty{requested},
				expectConnection: true,
			},
		}, {
			// The allowlist and requested properties both match the AP's BSSID.
			Name: "allowlist_requested",
			Val: bssidPropertiesTestCase{
				apMatch:          []bssidProperty{allowlist, requested},
				expectConnection: true,
			},
		}, {
			// The BSSID allowlist property does not match the AP's BSSID.
			Name: "allowlist_mismatch",
			Val: bssidPropertiesTestCase{
				apNotMatch:       []bssidProperty{allowlist},
				expectConnection: false,
			},
		}, {
			// The BSSID requested property does not match the AP's BSSID.
			Name: "requested_mismatch",
			Val: bssidPropertiesTestCase{
				apNotMatch:       []bssidProperty{requested},
				expectConnection: false,
			},
		}, {
			// The BSSID allowlist property matches the AP's BSSID, but the requested property does not.
			Name: "allowlist_not_requested",
			Val: bssidPropertiesTestCase{
				apMatch:          []bssidProperty{allowlist},
				apNotMatch:       []bssidProperty{requested},
				expectConnection: false,
			},
		}, {
			// The BSSID requested property matches the AP's BSSID, but the allowlist property does not.
			Name: "requested_not_allowlisted",
			Val: bssidPropertiesTestCase{
				apMatch:          []bssidProperty{requested},
				apNotMatch:       []bssidProperty{allowlist},
				expectConnection: false,
			},
		}, {
			// The BSSID allowlist property specifies that it should not associate with any AP.
			Name: "allowlist_none",
			Val: bssidPropertiesTestCase{
				configProps: map[string]interface{}{
					shillconst.ServicePropertyWiFiBSSIDAllowlist: []string{"00:00:00:00:00:00"},
				},
				expectConnection: false,
			},
		}},
	})
}

// BSSIDProperties tests two properties: BSSIDAllowlist and BSSIDRequested. The values of
// BSSIDAllowlist and/or BSSIDRequested, and the AP's own BSSID, will determine if the wifi network
// should connect to the AP or not.
//
// Concretely, this test:
//  1. Sets up an AP
//  2. Sets the BSSID properties on the wifi network and tries to connect to the AP
//  3. If the connection was expected to succeed, then tries to ping the AP to verify connectivity
//  4. Cleans up
func BSSIDProperties(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	hwAddr, err := hostapd.RandomMAC()
	if err != nil {
		s.Fatal("Failed to generate random BSSID: ", err)
	}
	apBSSID := hwAddr.String()

	// Configure an AP on the specific channel with given SSID.
	s.Log("Setting up the AP with BSSID ", apBSSID)
	options := []hostapd.Option{hostapd.Mode(hostapd.Mode80211nPure), hostapd.Channel(1), hostapd.HTCaps(hostapd.HTCapHT20), hostapd.BSSID(apBSSID)}
	ap, err := tf.ConfigureAP(ctx, options, nil)
	if err != nil {
		s.Fatal("Failed to set up AP: ", err)
	}
	defer func(ctx context.Context) {
		s.Log("Deconfiguring the AP")
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	configProps := map[string]interface{}{}

	// Keep configs that were passed in from the test params.
	configPropsParam := s.Param().(bssidPropertiesTestCase).configProps
	if configPropsParam != nil {
		s.Log("Starting with configParams from test params")
		configProps = configPropsParam
	}

	// Set BSSID of properties based on |apMatch| and |apNotMatch|.
	for _, prop := range s.Param().(bssidPropertiesTestCase).apMatch {
		if prop == allowlist {
			configProps[shillconst.ServicePropertyWiFiBSSIDAllowlist] = []string{apBSSID}
		}
		if prop == requested {
			configProps[shillconst.ServicePropertyWiFiBSSIDRequested] = apBSSID
		}
	}

	// Generate a MAC address that is not the same as |apBSSID|.
	otherHwAddr := hwAddr
	otherHwAddr[5]++
	otherBSSID := otherHwAddr.String()
	for _, prop := range s.Param().(bssidPropertiesTestCase).apNotMatch {
		if prop == allowlist {
			configProps[shillconst.ServicePropertyWiFiBSSIDAllowlist] = []string{otherBSSID}
		}
		if prop == requested {
			configProps[shillconst.ServicePropertyWiFiBSSIDRequested] = otherBSSID
		}
	}

	// Set the properties and connect.
	_, err = tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap, dutcfg.ConnProperties(configProps))

	if err == nil {
		// Connected, prepare cleanup.
		defer func(ctx context.Context) {
			if err := tf.DisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
				s.Error("Failed to disconnect WiFi: ", err)
			}
		}(ctx)
		ctx, cancel = tf.ReserveForDisconnect(ctx)
		defer cancel()

		if !s.Param().(bssidPropertiesTestCase).expectConnection {
			s.Errorf("Didn't expect to connect to WiFi SSID: %s: %v", ap.Config().SSID, err)
			return
		}
	} else {
		// Didn't connect.
		if s.Param().(bssidPropertiesTestCase).expectConnection {
			s.Errorf("Expected to connect to WiFi SSID: %s, but didn't", ap.Config().SSID)
		}
		return
	}

	// Verify connection.
	if _, err := tf.PingFromSpecificDUT(ctx, wificell.DefaultDUT, ap.ServerIP().String()); err != nil {
		s.Error("Failed to ping from the DUT: ", err)
	}
}
