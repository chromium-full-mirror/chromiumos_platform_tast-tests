// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// shareNetworkTestScenario specifies the test scenario,
// which performs the following steps, in order:
//
//  1. Login as the specified user.
//  2. Join the networks.
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
			"tast.cros.ui.AutomationService",
			wifiutil.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Val: []*shareNetworkTestScenario{
					// Login as device owner, join an open network and then ensure the network is connected.
					{
						loginAs: deviceOwner,
						networksToJoin: []*shareNetworkTestNetwork{
							openNetwork,
						},
						verifications: []func(*rpc.Client) action.Action{
							openNetwork.checkConnected,
						},
					},
					// Verify that the network is still connected and also shared with secondary user.
					{
						loginAs: normalUser,
						verifications: []func(*rpc.Client) action.Action{
							openNetwork.checkConnected,
							openNetwork.checkKnownNetwork,
						},
					},
					// Verify that the network is still connected and also shared with guest user.
					{
						loginAs: guest,
						verifications: []func(*rpc.Client) action.Action{
							openNetwork.checkConnected,
							openNetwork.checkKnownNetwork,
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
		var actions []action.Action
		if len(test.networksToJoin) > 0 {
			actions = append(actions, joinNetworks(rpcClient, test.networksToJoin))
		}
		for _, verification := range test.verifications {
			actions = append(actions, verification(rpcClient))
		}

		user := test.loginAs
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
