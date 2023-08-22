// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"regexp"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

// shareNetworkTestScenario specifies the test scenario,
// which performs the following steps, in order:
//
//  1. Login as the specified user.
//  2. Join the networks (in order).
//  3. Perform the specified verification steps.
type shareNetworkTestScenario struct {
	loginAs        shareNetworkTestUser
	networksToJoin []*shareNetworkTestNetwork
	verifications  []func(*rpc.Client) action.Action
}

type shareNetworkTestUser int

const (
	deviceOwner shareNetworkTestUser = iota
	normalUser
	guest
)

type shareNetworkTestNetwork struct {
	*wificell.APIface
	configs     *shareNetworkTestNetworkConfigs
	joinRequest *wifi.JoinWifiRequest
}

type shareNetworkTestNetworkConfigs struct {
	ssidPrefix          string
	options             []hostapd.Option
	securityConfig      security.ConfigFactory
	sharedWithOtherUser bool
}

const shareNetworkTestNetworkPsk = "chromeos"

var (
	openNetwork = &shareNetworkTestNetwork{
		configs: &shareNetworkTestNetworkConfigs{
			ssidPrefix:          "Test_open_network_",
			options:             wificell.DefaultOpenNetworkAPOptions(),
			sharedWithOtherUser: true,
		},
		joinRequest: &wifi.JoinWifiRequest{
			Security:            &wifi.JoinWifiRequest_None{},
			ShareWithOtherUsers: wifi.JoinWifiRequest_Default,
		},
	}

	secureNetwork = &shareNetworkTestNetwork{
		configs: &shareNetworkTestNetworkConfigs{
			ssidPrefix:          "Test_WPA3_network_1_",
			securityConfig:      wpa.NewConfigFactory(shareNetworkTestNetworkPsk, wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
			options:             wificell.DefaultWPA3NetworkAPOptions(),
			sharedWithOtherUser: false,
		},
		joinRequest: &wifi.JoinWifiRequest{
			Security:            &wifi.JoinWifiRequest_Psk{Psk: shareNetworkTestNetworkPsk},
			ShareWithOtherUsers: wifi.JoinWifiRequest_TurnOff,
		},
	}

	secureNetwork2 = &shareNetworkTestNetwork{
		configs: &shareNetworkTestNetworkConfigs{
			ssidPrefix:          "Test_WPA3_network_2_",
			securityConfig:      wpa.NewConfigFactory(shareNetworkTestNetworkPsk, wpa.Mode(wpa.ModePureWPA3), wpa.Ciphers2(wpa.CipherCCMP)),
			options:             wificell.DefaultWPA3NetworkAPOptions(),
			sharedWithOtherUser: false,
		},
		joinRequest: &wifi.JoinWifiRequest{
			Security:            &wifi.JoinWifiRequest_Psk{Psk: shareNetworkTestNetworkPsk},
			ShareWithOtherUsers: wifi.JoinWifiRequest_TurnOff,
		},
	}
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShareNetwork,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify the share property of a network across different users",
		Contacts: []string{
			"cros-connectivity@google.com",
			"chromeos-connectivity-engprod@google.com",
			"shijinabraham@google.com",
			"chadduffin@chromium.org",
			"cienet-development@googlegroups.com",
			"chromeos-connectivity-cienet-external@google.com",
			"alfredyu@cienet.com",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.wifi.WifiService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.ui.AutomationService",
			wifiutil.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Val: []*shareNetworkTestScenario{
					// Login as device owner, join 2 different networks (one shared and one non-shared) and then ensure the shared network is connected.
					{
						loginAs: deviceOwner,
						networksToJoin: []*shareNetworkTestNetwork{
							secureNetwork,
							openNetwork,
						},
						verifications: []func(*rpc.Client) action.Action{
							openNetwork.checkConnected,
							openNetwork.forgetButtonAvailable, // Any user should be able to forget a shared network.
						},
					},
					// Login as another user and verify the connected shared network from last user session is still connected and shared with this user.
					{
						loginAs: normalUser,
						verifications: []func(*rpc.Client) action.Action{
							openNetwork.checkConnected,
							openNetwork.checkKnownNetwork,
							openNetwork.forgetButtonAvailable, // Any user should be able to forget a shared network.
							secureNetwork.isNotKnownNetwork,
						},
					},
					// Further add a non-shared network for later use.
					{
						loginAs: normalUser,
						networksToJoin: []*shareNetworkTestNetwork{
							secureNetwork2,
						},
					},
					// Verify that the shared network is shared with guest user and non-shared networks are not shared with guest user.
					{
						loginAs: guest,
						verifications: []func(*rpc.Client) action.Action{
							openNetwork.checkKnownNetwork,
							openNetwork.forgetButtonAvailable, // Any user should be able to forget a shared network.
							secureNetwork.isNotKnownNetwork,
							secureNetwork2.isNotKnownNetwork,
						},
					},
					// Verify the non-shared added by other user is indeed not shared with device owner.
					{
						loginAs: deviceOwner,
						verifications: []func(*rpc.Client) action.Action{
							secureNetwork2.isNotKnownNetwork,
						},
					},
				},
			},
		},
	})
}

// ShareNetwork verifies the share property of a network across different users.
func ShareNetwork(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)
	loginReqs := map[shareNetworkTestUser]*ui.NewRequest{
		deviceOwner: {
			LoginMode:   ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			Credentials: &ui.NewRequest_Credentials{Username: "deviceowner@gmail.com", Password: "testpass"},
		},
		normalUser: {
			LoginMode:   ui.LoginMode_LOGIN_MODE_FAKE_LOGIN,
			Credentials: &ui.NewRequest_Credentials{Username: "normaluser@gmail.com", Password: "testpass"},
			KeepState:   true,
		},
		guest: {
			LoginMode: ui.LoginMode_LOGIN_MODE_GUEST_LOGIN,
			KeepState: true,
		},
	}

	rpcClient := tf.DUTRPC(wificell.DefaultDUT)
	// Creating user pods, in a fixed order and skipping the guest user since
	// a user is device owner iff it's added first and there's no need to create guest user pod.
	for _, user := range []shareNetworkTestUser{deviceOwner, normalUser} {
		if err := loginAndPerformActions(ctx, rpcClient, loginReqs[user]); err != nil {
			s.Fatal("Failed to confirm network is available for owner users: ", err)
		}
		// Reuse user data once the user pod has created.
		loginReqs[user].KeepState = true
	}

	testScenario := s.Param().([]*shareNetworkTestScenario)
	// Configuring networks.
	for _, test := range testScenario {
		if len(test.networksToJoin) > 0 {
			user := test.loginAs
			if err := loginAndPerformActions(ctx, rpcClient, loginReqs[user], configureNetworks(tf, test.networksToJoin)); err != nil {
				s.Fatalf("Failed to configure networks under user %d: %v", user, err)
			}
		}
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer tf.DeconfigAllAPs(cleanupCtx)

	for _, test := range testScenario {
		user := test.loginAs
		var actions []action.Action

		// Verify that shill profile is loaded correctly for each user session.
		actions = append(actions, correctShillProfileIsLoaded(rpcClient, s.DUT()))

		if len(test.networksToJoin) > 0 {
			actions = append(actions, joinNetworks(rpcClient, test.networksToJoin))
		}
		for _, verification := range test.verifications {
			actions = append(actions, verification(rpcClient))
		}

		if err := loginAndPerformActions(ctx, rpcClient, loginReqs[user], actions...); err != nil {
			s.Fatal("Failed to perform test scenario: ", err)
		}
	}
}

// loginAndPerformActions logs in and perform test steps or verifications.
func loginAndPerformActions(ctx context.Context, rpcClient *rpc.Client, startCrRequest *ui.NewRequest, actions ...action.Action) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	if _, err := crSvc.New(ctx, startCrRequest); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer crSvc.Close(cleanupCtx, &emptypb.Empty{})
	defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, func() bool { return retErr != nil }, "ui_tree")

	for _, action := range actions {
		if err := action(ctx); err != nil {
			return err
		}
	}
	return nil
}

func configureNetworks(tf *wificell.TestFixture, networks []*shareNetworkTestNetwork) action.Action {
	return func(ctx context.Context) error {
		for idx, network := range networks {
			if network.APIface != nil {
				// Avoiding configure duplicate networks.
				continue
			}

			opts := append(network.configs.options, hostapd.SSID(hostapd.RandomSSID(network.configs.ssidPrefix)))
			ap, err := tf.ConfigureAP(ctx, opts, network.configs.securityConfig)
			if err != nil {
				return errors.Wrap(err, "failed to configure the AP")
			}

			networks[idx].APIface = ap
			networks[idx].joinRequest.Ssid = ap.Config().SSID
		}
		return nil
	}
}

func joinNetworks(rpcClient *rpc.Client, networks []*shareNetworkTestNetwork) action.Action {
	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
	return func(ctx context.Context) error {
		for _, network := range networks {
			if _, err := wifiSvc.JoinWifiFromQuickSettings(ctx, network.joinRequest); err != nil {
				return errors.Wrap(err, "failed to join WiFi from Quick Settings")
			}
		}
		return nil
	}
}

// checkConnected verifies a network is connected by check the network status label in the QuickSettings.
func (network *shareNetworkTestNetwork) checkConnected(rpcClient *rpc.Client) action.Action {
	wifiClient := &wificell.WifiClient{
		ShillServiceClient: wifi.NewShillServiceClient(rpcClient.Conn),
	}
	return func(ctx context.Context) error {
		return wifiClient.WaitForConnected(ctx, network.Config().SSID, true /* expect connected */)
	}
}

// checkKnownNetwork verifies a network is known network by check if the network is listed in the known network list.
func (network *shareNetworkTestNetwork) checkKnownNetwork(rpcClient *rpc.Client) action.Action {
	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
	return func(ctx context.Context) error {
		if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
			Ssids:   []string{network.Config().SSID},
			Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
		}); err != nil {
			return errors.Wrap(err, "failed to verify the network is listed in known networks")
		}
		return nil
	}
}

// isNotKnownNetwork returns an action that verifies a network is not a known network by check if the network is not listed in the known network list.
// The returned action will return an error if the network is known, and otherwise will return |nil|.
func (network *shareNetworkTestNetwork) isNotKnownNetwork(rpcClient *rpc.Client) action.Action {
	wifiSvc := wifi.NewWifiServiceClient(rpcClient.Conn)
	return func(ctx context.Context) error {
		if _, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
			Ssids:   []string{network.Config().SSID},
			Control: wifi.KnownNetworksControlsRequest_WaitUntilGone,
		}); err != nil {
			return errors.Wrap(err, "failed to verify the network is listed in known networks")
		}
		return nil
	}
}

// forgetButtonAvailable checks that the "Forget" button can be found on the
// details page of |network.Config().SSID| instead of just checking that the
// network is known since we want to check that a user is actually able to
// forget known networks, and not just that users can remember networks.
func (network *shareNetworkTestNetwork) forgetButtonAvailable(rpcClient *rpc.Client) action.Action {
	osSettingsSvc := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
	req := &ossettings.EvalJSWithShadowPiercerRequest{
		Expression: `var buttons = shadowPiercingQueryAll("cr-button");
		var forgetButton = undefined;
		buttons.forEach(button => {
			if (button.innerText.includes("Forget")) {
				forgetButton = button;
			}
		})
		forgetButton != undefined;`,
	}

	return func(ctx context.Context) error {
		if _, err := osSettingsSvc.OpenNetworkDetailPage(ctx, &ossettings.OpenNetworkDetailPageRequest{
			NetworkName: network.Config().SSID,
			NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
		}); err != nil {
			return errors.Wrap(err, "failed to open network detail page")
		}

		resp, err := osSettingsSvc.EvalJSWithShadowPiercer(ctx, req)
		if err != nil {
			return errors.Wrap(err, "failed to find the forget button")
		}

		if !resp.GetBoolValue() {
			return errors.New("forget button not found")
		}
		return nil
	}
}

// correctShillProfileIsLoaded verifies that the loaded shill profile is aims to the active user.
func correctShillProfileIsLoaded(rpcClient *rpc.Client, dut *dut.DUT) action.Action {
	crSvc := ui.NewChromeServiceClient(rpcClient.Conn)
	return func(ctx context.Context) error {
		resp, err := crSvc.UserHash(ctx, &emptypb.Empty{})
		if err != nil {
			return errors.Wrap(err, "failed to verify shill profile is loaded correctly")
		}

		// Validate that the loaded profile aims to current user.
		data, err := linuxssh.ReadFile(ctx, dut.Conn(), "/var/run/shill/loaded_profile_list")
		if err != nil {
			return errors.Wrap(err, "failed to read the loaded shill profile")
		}
		if !regexp.MustCompile(resp.GetUserHash()).MatchString(string(data)) {
			return errors.Errorf("the loaded shill profile is incorrect, got: %q; want: %q", string(data), resp.GetUserHash())
		}
		return nil
	}
}
