// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/tunneled1x"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// credentials of the PEAP network.
const (
	identity = "testuser"
	password = "password"
)

type connectPEAPGuestWithoutCertTestParam struct {
	apOpts         []hostapd.Option
	securityConfig security.ConfigFactory
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           GuestConnectToWifiWithoutCert,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that guest user can connect to a 802.1x PEAP secure WiFi network without certificate",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: append(
			wifiutil.JoinWifiServiceNames,
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
		),
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "peap",
				Val: &connectPEAPGuestWithoutCertTestParam{
					apOpts: append(wificell.DefaultOpenNetworkAPOptions(), hostapd.SSID(hostapd.RandomSSID("GuestConnectToWifiWithoutCert_"))),
					securityConfig: tunneled1x.NewConfigFactory(
						certificate.TestCert1().CACred.Cert, certificate.TestCert1().ServerCred,
						certificate.TestCert1().CACred.Cert, identity, password,
						tunneled1x.OuterProtocol(tunneled1x.Layer1TypePEAP),
						tunneled1x.InnerProtocol(tunneled1x.Layer2TypeMSCHAPV2),
					),
				},
			},
		},
	})
}

// GuestConnectToWifiWithoutCert verifies that guest user can connect to a 802.1x PEAP secure WiFi network without certificate.
func GuestConnectToWifiWithoutCert(ctx context.Context, s *testing.State) {
	wifiCredential := &wifiutil.PEAP{
		Identity: identity,
		Password: password,
		// Don't validate the certificate as guest should be able to join to the PEAP WiFi without one.
		CaCert: wifiutil.CertificateComboBoxOptionDoNotCheck,
	}

	tf := s.FixtValue().(*wificell.TestFixture)
	param := s.Param().(*connectPEAPGuestWithoutCertTestParam)
	ap, err := tf.ConfigureAP(ctx, param.apOpts, param.securityConfig)
	if err != nil {
		s.Fatal("Failed to configure the AP: ", err)
	}
	cleanupAPCtx := ctx
	ctx, cancel := tf.ReserveForDeconfigAP(ctx, ap)
	defer cancel()
	defer tf.DeconfigAP(cleanupAPCtx, ap)

	cleanupCtx := ctx
	ctx, cancel = ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	cleanup, err := wifiutil.JoinWifiFromQuickSettings(ctx, rpcClient.Conn, ap.Config().SSID, wifiCredential)
	defer cleanup(cleanupCtx)
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, s.TestName())
	if err != nil {
		s.Fatal("Failed to join WiFi: ", err)
	}

	if err := tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, ap.Config().SSID, true); err != nil {
		s.Fatal("Failed to wait for connected: ", err)
	}

	// Disconnecting it for the upcoming test of reconnect attempt.
	if err := tf.DisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
		s.Fatal("Failed to disconnect network: ", err)
	}

	// Verify that guest user can have known network and it'll still listed as known network after disconnection.
	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ap.Config().SSID},
		Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
	}); err != nil {
		s.Fatal("Failed to verify the network is in 'Known Networks': ", err)
	}

	// Verify that guest user can save the credentials of a known network and reconnect to it directly, without entering credentials.
	if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
		Ssids:   []string{ap.Config().SSID},
		Control: wifi.KnownNetworksControlsRequest_Connect,
	}); err != nil {
		s.Fatal("Failed to reconnect to WiFi from 'Known Networks': ", err)
	}
	if err := tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, ap.Config().SSID, true); err != nil {
		s.Fatal("Failed to wait for connected: ", err)
	}
}
