// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"
	"google.golang.org/protobuf/types/known/wrapperspb"

	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/networkui"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

// pskForTesting stores the pre-shared key for test on a network.
type pskForTesting struct {
	// value is the pre-shared key of the WiFi network.
	value string
	// isCorrect indicates the pre-shared key is correct for the network.
	isCorrect bool
}

// testNetworkInfo stores the information of a network for testing.
type testNetworkInfo struct {
	// psk is the pre-shared key information of the network.
	psk *pskForTesting
	// ssidPrefix is the prefix of the SSID.
	ssidPrefix string
	// ap holds the AP interface of WiFi AP.
	ap *wificell.APIface
	// channel is the configured channel of the AP.
	channel int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           CredentialsMasking,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify the behaviors of connecting to networks that store valid and invalid password on the Sign in screen",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.BluetoothStateNormal, tbdep.PeripheralWifiStateWorking},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: append(
			wifiutil.JoinWifiServiceNames,
			wifiutil.FaillogServiceName,
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.ui.AutomationService",
			"tast.cros.wifi.WifiService",
			"tast.cros.networkui.CrosNetworkConfigService",
		),
		SoftwareDeps: []string{"chrome"},
		Fixture:      "wificellFixt",
		Timeout:      3 * time.Minute,
	})
}

// CredentialsMasking verifies the behaviors of connecting to networks that store correct and wrong password on the Sign in screen.
func CredentialsMasking(ctx context.Context, s *testing.State) {
	const correctPsk = "correct_password"

	tf := s.FixtValue().(*wificell.TestFixture)

	testNetworks := []*testNetworkInfo{{
		psk:        &pskForTesting{value: correctPsk, isCorrect: true},
		ssidPrefix: "Network_For_Test_Correct_Psk_",
		channel:    1, // Must use different band (b/313541152#comment16).
	}, {
		psk:        &pskForTesting{value: "wrong_password", isCorrect: false},
		ssidPrefix: "Network_For_Test_Wrong_Psk_",
		channel:    48, // Must use different band (b/313541152#comment16).
	}}

	securityConfig := wpa.NewConfigFactory(correctPsk, wpa.Mode(wpa.ModePureWPA), wpa.Ciphers(wpa.CipherTKIP, wpa.CipherCCMP))
	for _, testNetwork := range testNetworks {
		opts := []hostapd.Option{
			hostapd.Mode(hostapd.Mode80211nPure),
			hostapd.HTCaps(hostapd.HTCapHT20),
			hostapd.SSID(hostapd.RandomSSID(testNetwork.ssidPrefix)),
			hostapd.Channel(testNetwork.channel),
		}

		var (
			err    error
			cancel context.CancelFunc
		)
		if testNetwork.ap, err = tf.ConfigureAP(ctx, opts, securityConfig); err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
		cleanupCtx := ctx
		defer tf.DeconfigAP(cleanupCtx, testNetwork.ap)
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, testNetwork.ap)
		defer cancel()
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	wifiSvc := tf.DUTWifiClient(wificell.DefaultDUT)
	// Ensure the WiFi is enabled to perform the test.
	if err := wifiSvc.SetWifiEnabled(ctx, true); err != nil {
		s.Fatal("Failed to enable WiFi feature: ", err)
	}

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	cr := ui.NewChromeServiceClient(rpcClient.Conn)

	// Boot the DUT to sign in screen by add a user and then restart Chrome.
	if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
		s.Fatal("Failed to create user: ", err)
	}
	if _, err := cr.Close(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to close Chrome: ", err)
	}
	if _, err := cr.New(ctx, &ui.NewRequest{
		LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
		SigninProfileTestExtensionId: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		KeepState:                    true,
	}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	wifiSvcClient := wifi.NewWifiServiceClient(rpcClient.Conn)
	netconfigSvc := networkui.NewCrosNetworkConfigServiceClient(rpcClient.Conn)
	// Setup all networks before starting test.
	for _, testNetwork := range testNetworks {
		// Setup known network by joining the WiFi network with the correct credential.
		if _, err := wifiSvcClient.JoinWifiFromQuickSettings(ctx, &wifi.JoinWifiRequest{
			Ssid:     testNetwork.ap.Config().SSID,
			Security: &wifi.JoinWifiRequest_Psk{Psk: correctPsk},
		}); err != nil {
			s.Fatal("Failed to join WiFi from Quick Settings: ", err)
		}
		if err := tf.DisconnectDUTFromWifi(ctx, wificell.DefaultDUT); err != nil {
			s.Fatal("Failed to disconnect network: ", err)
		}

		// Updating the credential and disable auto-connect to meet the test needs.
		if _, err := netconfigSvc.ConfigureNetwork(ctx, &networkui.ConfigureNetworkRequest{
			ConfigProperties: &networkui.NetworkTypeConfigProperties{
				ConfigProperties: &networkui.NetworkTypeConfigProperties_WifiConfigProperties{
					WifiConfigProperties: &networkui.WiFiConfigProperties{
						Ssid:     testNetwork.ap.Config().SSID,
						Security: &networkui.WiFiConfigProperties_Psk{Psk: testNetwork.psk.value},
					},
				},
				AutoConnect: &wrapperspb.BoolValue{Value: false},
			},
		}); err != nil {
			s.Fatal("Failed to config network with auto-connect disabled: ", err)
		}
	}
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

	uiauto := ui.NewAutomationServiceClient(rpcClient.Conn)
	quickSettingsSvc := quicksettings.NewQuickSettingsServiceClient(rpcClient.Conn)
	keyboardSvc := inputs.NewKeyboardServiceClient(rpcClient.Conn)
	for _, testNetwork := range testNetworks {
		if _, err := quickSettingsSvc.SelectNetwork(ctx, &quicksettings.SelectNetworkRequest{
			Ssid: testNetwork.ap.Config().SSID,
		}); err != nil {
			s.Fatal("Failed to select WiFi from Quick Settings: ", err)
		}

		if testNetwork.psk.isCorrect {
			// Verify the network can automatically connect when a valid password is stored.
			if err := tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, testNetwork.ap.Config().SSID, true); err != nil {
				s.Fatal("Failed to wait for WiFi connection: ", err)
			}
		} else {
			const (
				defaultTimeout  = 15 * time.Second
				extendedTimeout = 60 * time.Second
			)

			start := time.Now()

			// Expecting the "Join Wi-Fi network" dialog pops up when attempt to connect to a known network that has stored an incorrect password.
			if _, err := uiauto.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
				Finder:  wifiutil.JoinWiFiNetworkDialogFinder,
				Timeout: durationpb.New(extendedTimeout),
			}); err != nil {
				s.Fatal("Failed to check if the Network Settings dialog shown: ", err)
			}
			// Ensure the "Join Wi-Fi network" dialog will be closed after test.
			defer keyboardSvc.Accel(ctx, &inputs.AccelRequest{Key: "Esc"})
			defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, s.HasError, s.TestName())

			// "Join Wi-Fi network" dialog might take a long time to pop up, raising error for record.
			elapsed := time.Since(start)
			if elapsed > defaultTimeout {
				s.Fatalf("Failed to check if the Network Settings dialog shown: the dialog doesn't pop up in time, time elapsed: %s", elapsed)
			}

			// Clicking the eye icon and check that previously used wrong network is not displayed in the next step.
			eyeIconFinder := ui.Node().HasClass("icon-visibility").Finder()
			if _, err := uiauto.LeftClick(ctx, &ui.LeftClickRequest{Finder: eyeIconFinder}); err != nil {
				s.Fatal("Failed to click the eye icon: ", err)
			}

			passwordFinder := ui.Node().Name(testNetwork.psk.value).Finder()
			// The password should not be shown in clear text when the the eye icon is clicked.
			// Check that clear text password is not shown, as the key box should be empty after clicking the eye button as expected.
			if _, err := uiauto.EnsureGone(ctx, &ui.EnsureGoneRequest{
				Finder:  passwordFinder,
				Timeout: durationpb.New(5 * time.Second),
			}); err != nil {
				s.Fatal("Failed to check if the password is not in clear text: ", err)
			}
		}
	}
}
