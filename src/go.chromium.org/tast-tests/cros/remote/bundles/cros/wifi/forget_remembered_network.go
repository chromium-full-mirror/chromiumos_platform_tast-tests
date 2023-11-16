// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ForgetRememberedNetwork,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify the behavior when a user forgets a remembered network which is currently connected",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      7 * time.Minute,
	})
}

// ForgetRememberedNetwork verifies the behavior when a user forgets a remembered network which is currently connected.
func ForgetRememberedNetwork(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	apOpts := []hostapd.Option{
		hostapd.Channel(1),
		hostapd.Mode(hostapd.Mode80211g),
	}

	const password = "chromeos_test_pass"
	securityConfig := wpa.NewConfigFactory(
		password,
		wpa.Mode(wpa.ModePureWPA),
		wpa.Ciphers(wpa.CipherTKIP, wpa.CipherCCMP),
	)

	ap, err := tf.ConfigureAP(ctx, apOpts, securityConfig)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.DeconfigAP(ctx, ap); err != nil {
			s.Error("Failed to deconfig the AP: ", err)
		}
	}(ctx)
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	if err := connectAndForgetNetwork(ctx, rpcClient, ap.Config().SSID, password); err != nil {
		s.Fatal("Failed to connect to and forget the network: ", err)
	}
	if _, err := crSvc.Close(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to close Chrome: ", err)
	}

	for _, test := range []struct {
		name    string
		execute action.Action
	}{
		{"suspend", suspend(tf.DUTWifiClient(wificell.DefaultDUT))},
		{"reboot", reboot(s.DUT())},
	} {
		s.Run(ctx, test.name, func(ctx context.Context, s *testing.State) {
			// cleanupCtx is the context with time reserved, used for cleaning up resources other than the AP.
			cleanupCtx := ctx
			ctx, cancel = ctxutil.Shorten(ctx, 15*time.Second)
			defer cancel()

			s.Logf("Executing %q", test.name)
			if err := test.execute(ctx); err != nil {
				s.Fatal("Failed to execute the test: ", err)
			}

			// The Chrome session will be destroyed after reboot, initiate a new RPC client for the coming tests.
			rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
			if err != nil {
				s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
			}
			defer rpcClient.Close(cleanupCtx)

			crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
			// TryReuseSession can ensure reuse existing session after suspend, but also will start a new Chrome session after reboot.
			if _, err := crSvc.New(ctx, &ui.NewRequest{TryReuseSession: true, KeepState: true}); err != nil {
				s.Fatal("Failed to reuse Chrome session: ", err)
			}
			defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

			if err := checkForgottenAndConnectable(ctx, rpcClient, ap.Config().SSID); err != nil {
				s.Fatalf("Failed to verify the result after %q: %v", test.name, err)
			}
		})
	}
}

// connectAndForgetNetwork connects, forgets the network and verifies the connections are being set correctly.
func connectAndForgetNetwork(ctx context.Context, rpcClient *rpc.Client, ssid, password string) (retErr error) {
	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)

	if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, &wifi.JoinWifiRequest{
		Ssid:     ssid,
		Security: &wifi.JoinWifiRequest_Psk{Psk: password},
	}); err != nil {
		return errors.Wrapf(err, "failed to connect to WiFi %q", ssid)
	}

	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ssid},
		Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
	}); err != nil {
		return errors.Wrapf(err, "failed to verify %q is in 'Known Networks'", ssid)
	}

	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ssid},
		Control: wifi.KnownNetworksControlsRequest_Forget,
	}); err != nil {
		return errors.Wrapf(err, "failed to forget WiFi %q", ssid)
	}

	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ssid},
		Control: wifi.KnownNetworksControlsRequest_WaitUntilGone,
	}); err != nil {
		return errors.Wrapf(err, "failed to verify %q is gone from 'Known Networks'", ssid)
	}

	return nil
}

// checkForgottenAndConnectable verifies the specific network is forgotten and not in 'Known Networks' page after suspending and rebooting DUT.
// It also ensure the specified network is connectable.
func checkForgottenAndConnectable(ctx context.Context, rpcClient *rpc.Client, ssid string) (retErr error) {
	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)

	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ssid},
		Control: wifi.KnownNetworksControlsRequest_WaitUntilGone,
	}); err != nil {
		return errors.Wrapf(err, "failed to verify %q is not in 'Known Networks'", ssid)
	}

	wifiClient := &wificell.WifiClient{ShillServiceClient: wifi.NewShillServiceClient(rpcClient.Conn)}

	expectProps := map[string]interface{}{
		shillconst.ServicePropertyName:        ssid,
		shillconst.ServicePropertyConnectable: false, // The given network is encrypted. Reconnecting to it would require the password. Therefore, it should not be connectable.
	}

	path, err := wifiClient.GetServicePath(ctx, expectProps)
	if err != nil {
		return errors.Wrap(err, "failed to get the expected service path")
	}
	if path == "" {
		return errors.New("got empty service path")
	}

	return nil
}

func suspend(wifiClient *wificell.WifiClient) action.Action {
	return func(ctx context.Context) error {
		return wifiClient.Suspend(ctx, 10*time.Second)
	}
}

func reboot(dut *dut.DUT) action.Action {
	return dut.Reboot
}
