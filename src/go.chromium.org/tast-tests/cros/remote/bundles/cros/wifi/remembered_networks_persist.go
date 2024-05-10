// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/dutcfg"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	rememberedNetworksPersistDefaultTimeout = 3 * time.Minute
	suspendResumeTimeout                    = time.Minute
)

type rememberedNetworksPersistParam struct {
	testScenario    func(context.Context, *wificell.TestFixture) error
	tryReuseSession bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           RememberedNetworksPersist,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Verify remembered networks persist across suspend/resume, reboot and logout/login",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		Attr:         []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesWithUI),
		Params: []testing.Param{
			{
				Name: "suspend",
				Val: rememberedNetworksPersistParam{
					testScenario: func(ctx context.Context, tf *wificell.TestFixture) error {
						return tf.DUTWifiClient(wificell.DefaultDUT).Suspend(ctx, 10*time.Second)
					},
					tryReuseSession: true,
				},
				Timeout: rememberedNetworksPersistDefaultTimeout + suspendResumeTimeout,
			}, {
				Name:              "reboot",
				ExtraSoftwareDeps: []string{"reboot"},
				Val: rememberedNetworksPersistParam{
					testScenario: func(ctx context.Context, tf *wificell.TestFixture) error {
						return tf.RebootDUT(ctx, wificell.DefaultDUT)
					},
				},
				Timeout: rememberedNetworksPersistDefaultTimeout + wificell.DUTRebootTimeout,
			}, {
				Name: "re_login",
				Val: rememberedNetworksPersistParam{
					// The re-login action can be done by initiating a new Chrome session which is
					// already done automatically for each test case.
					testScenario: func(ctx context.Context, tf *wificell.TestFixture) error { return nil },
				},
				Timeout: rememberedNetworksPersistDefaultTimeout,
			},
		},
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

// RememberedNetworksPersist verifies remembered networks persist across suspend/resume, reboot and logout/login.
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

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	var ssids []string

	s.Log("Configuring APs")
	for _, networkConfig := range networks {
		apOpts := []hostapd.Option{
			hostapd.Channel(1),
			hostapd.Mode(hostapd.Mode80211g),
			hostapd.SSID(hostapd.RandomSSID(networkConfig.ssidPrefix)),
		}

		if networkConfig.isHidden {
			apOpts = append(apOpts, hostapd.Hidden())
		}

		cleanupAPCtx := ctx

		var err error
		if networkConfig.ap, err = tf.ConfigureAP(ctx, apOpts, networkConfig.apConfig); err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.DeconfigAP(ctx, networkConfig.ap); err != nil {
				s.Error("Failed to deconfig the AP: ", err)
			}
		}(cleanupAPCtx)

		// Disable the auto-connect property to avoid the network being connected before the connect attempt since the DUT could be auto-connected to any known network with this property is enabled.
		// See more at b/306296619.
		disableAutoConnect := map[string]interface{}{
			shillconst.ServicePropertyAutoConnect: false,
		}
		if _, err := tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, networkConfig.ap, dutcfg.ConnProperties(disableAutoConnect)); err != nil {
			s.Fatal("Failed to connect network: ", err)
		}
		ssids = append(ssids, networkConfig.ap.Config().SSID)

		var cancel context.CancelFunc
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, networkConfig.ap)
		defer cancel()
	}

	wifiUIService := wifi.NewWifiServiceClient(rpcClient.Conn)
	if err := confirmNetworksRemembered(ctx, wifiUIService, ssids); err != nil {
		s.Fatal("Failed to verify the added networks are in remembered networks list: ", err)
	}

	test := s.Param().(rememberedNetworksPersistParam)

	if err := test.testScenario(ctx, tf); err != nil {
		s.Fatal("Failed to perform test scenario: ", err)
	}

	// Retrieve resource again due to rebooting could cause RPC client closing.
	rpcClient = tf.DUTRPC(wificell.DefaultDUT)

	if err := loginAndVerifyRememberedNetworksPersist(ctx, rpcClient.Conn, test.tryReuseSession, ssids); err != nil {
		s.Fatal("Failed to verify remembered networks persist after login: ", err)
	}
}

func loginAndVerifyRememberedNetworksPersist(ctx context.Context, conn *grpc.ClientConn, tryReuseSession bool, ssids []string) error {
	cr := ui.NewChromeServiceClient(conn)
	wifiUIService := wifi.NewWifiServiceClient(conn)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	loginRequest := &ui.NewRequest{KeepState: true, TryReuseSession: tryReuseSession}

	if _, err := cr.New(ctx, loginRequest); err != nil {
		return errors.Wrap(err, "failed to connect chrome")
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	if err := confirmNetworksRemembered(ctx, wifiUIService, ssids); err != nil {
		return errors.Wrap(err, "failed to verify the added networks are in remembered networks list")
	}

	if err := confirmNetworksConnectable(ctx, wifiUIService, ssids); err != nil {
		return errors.Wrap(err, "failed to verify the remembered networks can be connected")
	}

	return nil
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
