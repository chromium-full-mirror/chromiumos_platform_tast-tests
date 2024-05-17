// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisconnectFromNetwork,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the user has a way of disconnecting from the current network on a Chromebook",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent:   "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Attr:           []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:    []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wificell.BrowserChromeServiceName,
			wificell.WifiUIServiceName,
			wificell.OsSettingsServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      4 * time.Minute,
	})
}

// DisconnectFromNetwork verifies that the user has a way of disconnecting from
// the current network on a Chromebook.
func DisconnectFromNetwork(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	var apForDisconnectBehavior, apForAutoConnectBehavior *wificell.APIface
	var networkSSIDs []string
	// To test the auto-connect behavior, the network to be auto-connected to should
	// be connected first, and then attempt to connect to another network, making the
	// second network become the currently connected network.
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)
	for _, networkAP := range []struct {
		ap          **wificell.APIface
		apOptions   []hostapd.Option
		autoConnect bool
	}{
		{
			ap: &apForAutoConnectBehavior,
			apOptions: []hostapd.Option{
				hostapd.Mode(hostapd.Mode80211nPure),
				hostapd.Channel(1),
				hostapd.HTCaps(hostapd.HTCapHT20),
				hostapd.SSID((hostapd.RandomSSID("auto_connect_network"))),
			},
			autoConnect: true,
		}, {
			ap: &apForDisconnectBehavior,
			apOptions: []hostapd.Option{
				hostapd.Mode(hostapd.Mode80211nPure),
				hostapd.Channel(48),
				hostapd.HTCaps(hostapd.HTCapHT20),
				hostapd.SSID((hostapd.RandomSSID("disconnect_network"))),
			},
			autoConnect: false,
		},
	} {
		ap, err := tf.ConfigureAP(ctx, networkAP.apOptions, nil)
		if err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
		defer tf.DeconfigAP(ctx, ap)
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()

		var configProps = map[string]interface{}{shillconst.ServicePropertyAutoConnect: networkAP.autoConnect}
		// Retry connecting to the network because connecting to a network might result in a failure
		// when another auto-connect network is already connected before. (see b/320811867)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			expectProps := map[string]interface{}{
				shillconst.ServicePropertyName:        ap.Config().SSID,
				shillconst.ServicePropertyIsConnected: true,
			}
			if _, err := wifiClient.GetServicePath(ctx, expectProps); err != nil {
				// Connect to the network if it is not already connected.
				_, err = tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap, dutcfg.ConnProperties(configProps))
				return err
			}
			return nil
		}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 5 * time.Second}); err != nil {
			s.Fatal("Failed to connect to network: ", err)
		}
		*networkAP.ap = ap
		networkSSIDs = append(networkSSIDs, ap.Config().SSID)
	}

	settingsSvc := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	resp, err := settingsSvc.KnownWifiNetworks(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to retrieve known networks: ", err)
	}

	// Validating the connected network and the network to be auto-connected
	// to are the only known network.
	if len(networkSSIDs) != len(resp.GetSsids()) {
		s.Fatalf(`Failed to verify the test networks are the only known networks; got: %v, want: %v`, resp.GetSsids(), networkSSIDs)
	}
	for _, ssid := range networkSSIDs {
		if found := slices.Contains(resp.GetSsids(), ssid); !found {
			s.Fatalf(`Failed to verify the networks are the only known networks: %q is not known network`, ssid)
		}
	}

	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
	// Verify that the network can be disconnected by clicking the "Disconnect"
	// button and the status will be "Not Connected".
	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{apForDisconnectBehavior.Config().SSID},
		Control: wifi.KnownNetworksControlsRequest_Disconnect,
	}); err != nil {
		s.Fatal("Failed to disconnect network: ", err)
	}

	// Verify the network will be auto-connected once the current connected
	// network is disconnected.
	if err := wifiClient.WaitForConnected(ctx, apForAutoConnectBehavior.Config().SSID, true /* expectedValue */); err != nil {
		s.Fatal("Failed to verify network is auto connected: ", err)
	}

	// Verify that the auto-connected network can be disconnected by clicking the
	// "Disconnect" button and the status will be "Not Connected".
	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{apForAutoConnectBehavior.Config().SSID},
		Control: wifi.KnownNetworksControlsRequest_Disconnect,
	}); err != nil {
		s.Fatal("Failed to disconnect network: ", err)
	}

	// Verify that the auto-connected network is disconnected after clicking
	// the "Disconnect" button.
	if err := wifiClient.WaitForConnected(ctx, apForAutoConnectBehavior.Config().SSID, false /* expectedValue */); err != nil {
		s.Fatal("Failed to verify the auto-connected network can be disconnected: ", err)
	}

	// Verify that the network stays disconnected for a while.
	// Applying a reversed check here to ensure the network does not get connected
	// during the whole check, expecting an error indicates that the test success
	// since the network should not be connected, should fail the test otherwise.
	if err := wifiClient.WaitForConnected(ctx, apForAutoConnectBehavior.Config().SSID, true /* expectedValue */); err == nil { // if NO error
		s.Fatal("Failed to verify the network does not get automatically reconnected and stays disconnected: ", err)
	}
}
