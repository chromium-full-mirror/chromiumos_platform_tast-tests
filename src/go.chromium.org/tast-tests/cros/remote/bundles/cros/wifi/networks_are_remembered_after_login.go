// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const networksAreRememberedAfterLoginTestPass = "TAST_TEST_CrOS_1!!"

type remainsRememberedTestParams struct {
	isHidden        bool
	apConfig        security.ConfigFactory
	joinWiFiRequest *wifi.JoinWifiRequest
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           NetworksAreRememberedAfterLogin,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify the networks added at OOBE and sign-in screen are remembered after logged in",
		Contacts: []string{
			// "cros-connectivity@google.com",
			// "chromeos-connectivity-engprod@google.com",
			"alfredyu@cienet.com",
			"cj.tsai@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		// ChromeOS > Software > System Services > Connectivity > WiFi
		BugComponent: "b:1131912",
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
			"tast.cros.ui.ChromeUIService",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name: "open_network",
				Val: &remainsRememberedTestParams{
					joinWiFiRequest: &wifi.JoinWifiRequest{
						Security: &wifi.JoinWifiRequest_None{},
					},
				},
			}, {
				Name: "encrypted_network",
				Val: &remainsRememberedTestParams{
					apConfig: wpa.NewConfigFactory(
						networksAreRememberedAfterLoginTestPass,
						wpa.Mode(wpa.ModePureWPA2),
						wpa.Ciphers2(wpa.CipherCCMP),
					),
					joinWiFiRequest: &wifi.JoinWifiRequest{
						Security: &wifi.JoinWifiRequest_Psk{Psk: networksAreRememberedAfterLoginTestPass},
					},
				},
			}, {
				Name: "hidden_network",
				Val: &remainsRememberedTestParams{
					isHidden: true,
					apConfig: wpa.NewConfigFactory(
						networksAreRememberedAfterLoginTestPass,
						wpa.Mode(wpa.ModePureWPA2),
						wpa.Ciphers2(wpa.CipherCCMP),
					),
					joinWiFiRequest: &wifi.JoinWifiRequest{
						Security: &wifi.JoinWifiRequest_Psk{Psk: networksAreRememberedAfterLoginTestPass},
					},
				},
			},
		},
	})
}

type screenIdentifier string

const (
	oobe         screenIdentifier = "OOBE"
	signInScreen screenIdentifier = "SignInScreen"
)

// NetworksAreRememberedAfterLogin verifies that the networks added at OOBE/sign-in screen are remembered after logged in.
func NetworksAreRememberedAfterLogin(ctx context.Context, s *testing.State) {
	apOptions := []hostapd.Option{
		hostapd.Channel(48),
		hostapd.Mode(hostapd.Mode80211nPure),
		hostapd.HTCaps(hostapd.HTCapHT40),
		hostapd.SpectrumManagement(),
	}

	params := s.Param().(*remainsRememberedTestParams)
	if params.isHidden {
		apOptions = append(apOptions, hostapd.Hidden())
	}

	tf := s.FixtValue().(*wificell.TestFixture)

	accessPoints := map[screenIdentifier]*wificell.APIface{oobe: nil, signInScreen: nil}
	for identifier := range accessPoints {
		opts := append(apOptions, hostapd.SSID(hostapd.RandomSSID(string(identifier))))

		ap, err := tf.ConfigureAP(ctx, opts, params.apConfig)
		if err != nil {
			s.Fatal("Failed to configure the AP: ", err)
		}
		defer tf.DeconfigAP(ctx, ap)

		var cancel context.CancelFunc
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, ap)
		defer cancel()

		accessPoints[identifier] = ap
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	startChromeRequest := func(screen screenIdentifier) *ui.NewRequest {
		req := &ui.NewRequest{
			LoginMode:                    ui.LoginMode_LOGIN_MODE_NO_LOGIN,
			SigninProfileTestExtensionId: s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		}
		if screen == signInScreen {
			req.KeepState = true
		}
		return req
	}

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)

	// Add network at OOBE.
	// Isolate the step to leverage `defer` pattern.
	func() {
		crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
		if _, err := crSvc.New(ctx, startChromeRequest(oobe)); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

		// Waiting for OOBE is ready for testing.
		chromeUISvc := ui.NewChromeUIServiceClient(rpcClient.Conn)
		if _, err := chromeUISvc.WaitForWelcomeScreen(ctx, &emptypb.Empty{}); err != nil {
			s.Fatal("Failed to wait for OOBE is ready for testing: ", err)
		}

		params.joinWiFiRequest.Ssid = accessPoints[oobe].Config().SSID
		if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, params.joinWiFiRequest); err != nil {
			s.Fatal("Failed to join WiFi from quick settings: ", err)
		}
	}()

	// Verify the network added at OOBE remains remembered after logged in.
	if err := signInAndVerify(ctx, rpcClient, accessPoints[oobe]); err != nil {
		s.Fatal("Failed to signIn and verify the network added at OOBE remains remembered: ", err)
	}

	// Add network at sign-in screen.
	// Isolate the step to leverage `defer` pattern.
	func() {
		crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
		if _, err := crSvc.New(ctx, startChromeRequest(signInScreen)); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

		params.joinWiFiRequest.Ssid = accessPoints[signInScreen].Config().SSID
		if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, params.joinWiFiRequest); err != nil {
			s.Fatal("Failed to join WiFi from quick settings: ", err)
		}
	}()

	// Verify that networks added at OOBE and sign-in screen both remain remembered after logged in.
	if err := signInAndVerify(ctx, rpcClient, accessPoints[signInScreen], accessPoints[oobe]); err != nil {
		s.Fatal("Failed to signIn and verify all networks remain remembered: ", err)
	}
}

// signInAndVerify logs in and verifies the specified networks are satisfy the following criteria:
// 1. Shown in wireless scan list.
// 2. Shown in remembered network list.
// 3. Shown in remembered network list and marked as shared network.
func signInAndVerify(ctx context.Context, rpcClient *rpc.Client, accessPoints ...*wificell.APIface) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, &ui.NewRequest{KeepState: true}); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})

	for _, ap := range accessPoints {
		wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)

		// If the AP is hidden and we have not explicitly marked the network as hidden the network will not be shown in the WiFi list.
		if !ap.Config().Hidden {
			if _, err := wifiSvc.WifiPageControl(ctx, &wifi.WifiPageControlRequest{
				Ssid:    ap.Config().SSID,
				Control: wifi.WifiPageControlRequest_WaitUntilExist,
			}); err != nil {
				return errors.Wrap(err, "failed to verify that added networks are shown in WiFi scan list after logged in")
			}
		}

		if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
			Ssids:   []string{ap.Config().SSID},
			Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
		}); err != nil {
			return errors.Wrap(err, "failed to verify that added networks are in remembered list after logged in")
		}

		if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
			Ssids:   []string{ap.Config().SSID},
			Control: wifi.KnownNetworksControlsRequest_ShownAsShared,
		}); err != nil {
			return errors.Wrap(err, "failed to verify that added networks are shared after logged in")
		}
	}
	return nil
}
