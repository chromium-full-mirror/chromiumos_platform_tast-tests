// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"chromiumos/tast/common/wifi/security"
	"chromiumos/tast/common/wifi/security/wpa"
	"chromiumos/tast/remote/wificell"
	"chromiumos/tast/remote/wificell/hostapd"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RememberedNetworksPersist,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify remembered networks persist across suspend/resume",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
			"toby.leung@cienet.com",
			"cienet-development@googlegroups.com",
		},
		BugComponent: "b:1131912",
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixt",
	})
}

// rememberedNetworksTestNetworks stores the network config resources.
type rememberedNetworksTestNetworks struct {
	// ap holds the AP interface of WiFi AP.
	ap *wificell.APIface
	// apConfig holds the configurations of WiFi AP.
	apConfig security.ConfigFactory
	// isHidden indicates the network is visible.
	isHidden bool
	// ssidPrefix is the prefix of the SSID.
	ssidPrefix string
}

// RememberedNetworksPersist verifies remembered networks persist across suspend/resume.
func RememberedNetworksPersist(ctx context.Context, s *testing.State) {
	var (
		tf       = s.FixtValue().(*wificell.TestFixture)
		networks = []*rememberedNetworksTestNetworks{
			{
				ssidPrefix: "open_network_",
			}, {
				apConfig:   wpa.NewConfigFactory("encrypted_network_password", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP, wpa.CipherCCMP)),
				ssidPrefix: "encrypted_network_ssid_",
			}, {
				apConfig:   wpa.NewConfigFactory("hidden_network_password", wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP, wpa.CipherCCMP)),
				ssidPrefix: "hidden_network_ssid_",
				isHidden:   true,
			},
		}
	)

	s.Log("Configuring APs")
	for _, network := range networks {
		apOpts := []hostapd.Option{
			hostapd.Channel(1),
			hostapd.Mode(hostapd.Mode80211g),
			hostapd.SSID(hostapd.RandomSSID(network.ssidPrefix)),
		}

		if network.isHidden {
			apOpts = append(apOpts, hostapd.Hidden())
		}

		cleanupAPCtx := ctx

		var err error
		if network.ap, err = tf.ConfigureAP(ctx, apOpts, network.apConfig); err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, network.ap); err != nil {
				s.Error("Failed to deconfig the AP: ", err)
			}
		}(cleanupAPCtx)

		var cancel context.CancelFunc
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, network.ap)
		defer cancel()
	}

	// cleanupCtx is the context with time reserved, used for cleaning up resources other than the AP.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	ssids, err := addRememberedNetworks(ctx, networks, tf, rpcClient)
	if err != nil {
		s.Fatal("Failed to add remembered networks: ", err)
	}

	s.Log("Suspending DUT")
	if err := tf.DUTWifiClient(wificell.DefaultDUT).Suspend(ctx, 10*time.Second); err != nil {
		s.Fatal("Failed to perform system suspend: ", err)
	}

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	// Creating a new session will clear the network setting, need to ensure reuse the existing session after resume.
	if _, err := cr.New(ctx, &ui.NewRequest{TryReuseSession: true, KeepState: true}); err != nil {
		s.Fatal("Failed to connect chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	wifiUIService := wifi.NewWifiServiceClient(rpcClient.Conn)
	if err := confirmNetworksRemembered(ctx, wifiUIService, ssids); err != nil {
		s.Fatal("Failed to verify the added networks are in remembered networks list: ", err)
	}

	if err := confirmNetworksConnectable(ctx, wifiUIService, ssids); err != nil {
		s.Fatal("Failed to verify the remembered networks can be connected: ", err)
	}
}

// addRememberedNetworks remembers the networks by adding it and ensure the networks are remembered.
func addRememberedNetworks(ctx context.Context, networks []*rememberedNetworksTestNetworks, tf *wificell.TestFixture, rpcClient *rpc.Client) (ssids []string, err error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
		return ssids, errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	// Remember a network by connecting to it.
	for _, network := range networks {
		if _, err = tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, network.ap); err != nil {
			return ssids, errors.Wrap(err, "failed to connect network")
		}
		ssids = append(ssids, network.ap.Config().SSID)
	}

	wifiUIService := wifi.NewWifiServiceClient(rpcClient.Conn)
	if err := confirmNetworksRemembered(ctx, wifiUIService, ssids); err != nil {
		return ssids, errors.Wrap(err, "failed to verify the added networks are in remembered networks list")
	}

	return ssids, nil
}

// confirmNetworksRemembered verifies the networks existed in 'Known Networks' list by checking the node can be found.
func confirmNetworksRemembered(ctx context.Context, wifiUI wifi.WifiServiceClient, ssids []string) error {
	if _, err := wifiUI.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   ssids,
		Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
	}); err != nil {
		return errors.Wrap(err, "failed to verify is not in 'Known Networks'")
	}

	return nil
}

// confirmNetworksConnectable verifies all networks are remembered and can be connected without fill in any authentications.
func confirmNetworksConnectable(ctx context.Context, wifiUI wifi.WifiServiceClient, ssids []string) error {
	if _, err := wifiUI.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   ssids,
		Control: wifi.KnownNetworksControlsRequest_Connect,
	}); err != nil {
		return errors.Wrap(err, "failed to connect to WiFi from 'Known Networks'")
	}

	return nil
}
