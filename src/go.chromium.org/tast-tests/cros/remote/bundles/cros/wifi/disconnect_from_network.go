// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"golang.org/x/exp/slices"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           DisconnectFromNetwork,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that the user has a way of disconnecting from the current network on a Chromebook",
		Contacts: []string{
			// TODO(b/311444590): Enable the following contacts after the test is stabled.
			// "cros-connectivity@google.com",
			// "chromeos-connectivity-engprod@google.com",
			"cj.tsai@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
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

	var apForDisconnectBehavior, apForAutoConnectBehavior *wificell.APIface
	// To test the auto-connect behavior, the network to be auto-connected to should
	// be connected first, and then attempt to connect to another network, making the
	// second network become the currently connected network.
	networksForTests := []**wificell.APIface{&apForAutoConnectBehavior, &apForDisconnectBehavior}
	networkSSIDs := make([]string, 0, len(networksForTests))
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)
	for _, networkAP := range networksForTests {
		ap, err := tf.DefaultOpenNetworkAP(ctx)
		if err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
		defer tf.DeconfigAP(ctx, ap)
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()

		var configProps = map[string]interface{}{}
		// Disable auto-connect property if the network is not intended to be
		// connected automatically.
		if networkAP == &apForDisconnectBehavior {
			configProps[shillconst.ServicePropertyAutoConnect] = false
		}
		if _, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap, dutcfg.ConnProperties(configProps)); err != nil {
			s.Fatal("Failed to connect to network: ", err)
		}
		*networkAP = ap
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
	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
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
