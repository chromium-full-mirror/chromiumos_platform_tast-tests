// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ControlAutoconnectWithUI,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify user should be able to specify whether or not a particular network can auto-connect after turning WiFi off/on, rebooting and waking up from sleep",
		Contacts: []string{
			"cros-connectivity@google.com",
			"cros-conn-test-team@google.com",
			"alfredyu@cienet.com",
			"cienet-development@googlegroups.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1131912", // ChromeOS > Software > System Services > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_e2e_unstable"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			"tast.cros.chrome.uiauto.ossettings.OsSettingsService",
			"tast.cros.wifi.WifiService",
			wifiutil.FaillogServiceName,
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      wificell.FixtureID(wificell.TFFeaturesNone),
		Params: []testing.Param{
			{
				Name: "cycle_wifi",
				Val:  cycleWifi,
			}, {
				Name: "suspend_and_wake",
				Val:  suspendAndWake,
			}, {
				Name:    "reboot_dut",
				Val:     rebootDUT,
				Timeout: 5*time.Minute + wificell.DUTRebootTimeout,
			},
		},
	})
}

type apIdentifier int

const (
	primaryAP apIdentifier = iota
	anotherAP
	notUsedAP1 // For simulate multiple Wifi networks available within the range.
	notUsedAP2 // For simulate multiple Wifi networks available within the range.
)

// ControlAutoconnectWithUI verifies user should be able to specify whether or not a particular
// network can auto-connect after turning WiFi off/on, rebooting and waking up from sleep.
func ControlAutoconnectWithUI(ctx context.Context, s *testing.State) {
	tf := s.FixtValue().(*wificell.TestFixture)

	ap := map[apIdentifier]*apUtil{}
	for _, id := range []apIdentifier{primaryAP, anotherAP, notUsedAP1, notUsedAP2} {
		accessPoint, err := tf.ConfigureAP(ctx, wificell.DefaultOpenNetworkAPOptions(), nil)
		if err != nil {
			s.Fatal("Failed to setup an AP: ", err)
		}

		var cancel context.CancelFunc
		cleanupCtx := ctx
		ctx, cancel = tf.ReserveForDeconfigAP(ctx, accessPoint)
		defer cancel()
		defer tf.DeconfigAP(cleanupCtx, accessPoint)

		ap[id] = &apUtil{APIface: accessPoint, tf: tf}
	}
	cleanupCtx := ctx
	ctx, cancel := tf.ReserveForDisconnect(ctx)
	defer cancel()
	defer tf.CleanDisconnectDUTFromWifi(cleanupCtx, wificell.DefaultDUT)

	// Isolate the step to leverage `defer` pattern.
	func(ctx context.Context) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		cr := ui.NewChromeServiceClient(tf.DUTRPC(wificell.DefaultDUT).Conn)
		if _, err := cr.New(ctx, &ui.NewRequest{}); err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}
		defer cr.Close(cleanupCtx, &emptypb.Empty{})

		// Checking if the known network list is empty as the precondition of this test.
		settingsSvc := ossettings.NewOsSettingsServiceClient(tf.DUTRPC(wificell.DefaultDUT).Conn)
		resp, err := settingsSvc.KnownWifiNetworks(ctx, &emptypb.Empty{})
		if err != nil {
			s.Fatal("Failed to acquire the known network list: ", err)
		} else if len(resp.GetSsids()) != 0 {
			s.Fatal("Failed to verify DUT is ready for test: known network list is not empty")
		}

		// Preparing the primary known network for upcoming tests.
		if err := ap[primaryAP].connectAndWaitForItToBeKnownNetwork(ctx); err != nil {
			s.Fatalf("Failed to connect to AP %q: %v", ap[primaryAP].Config().SSID, err)
		}
	}(ctx)

	for _, test := range []struct {
		description string
		setup       action.Action
		verify      action.Action
	}{
		{
			description: "the network should automatically connect since auto-connect is enabled by default",
			setup:       func(ctx context.Context) error { return nil },
			verify:      ap[primaryAP].ensureConnectedState(true),
		}, {
			description: "the network should not automatically connect when auto-connect is disabled",
			setup:       ap[primaryAP].setAutoConnect(false),
			verify:      ap[primaryAP].ensureConnectedState(false),
		}, {
			description: "a network with auto-connect enabled should be automatically connected to even when we are connected to a network with auto-connect disabled",
			setup: action.Combine("setup another known network and the auto-connect property of each known network",
				ap[anotherAP].connectAndWaitForItToBeKnownNetwork,
				ap[anotherAP].setAutoConnect(false),
				ap[primaryAP].setAutoConnect(true),
			),
			verify: ap[primaryAP].ensureConnectedState(true),
		}, {
			description: "networks with auto-connect disabled should not be automatically connected to",
			setup: action.Combine("setup the auto-connect property of each known network",
				ap[anotherAP].setAutoConnect(false),
				ap[primaryAP].setAutoConnect(false),
			),
			verify: action.Combine("verify connected status of each known network",
				ap[primaryAP].ensureConnectedState(false),
				ap[anotherAP].ensureConnectedState(false),
			),
		},
	} {
		s.Run(ctx, test.description, func(ctx context.Context, s *testing.State) {
			if err := startChromeAndPerformAction(ctx, tf, test.setup); err != nil {
				s.Fatal("Failed to setup for test: ", err)
			}

			action := s.Param().(func(context.Context, *wificell.TestFixture) error)
			if err := action(ctx, tf); err != nil {
				s.Fatal("Failed to perform the action that triggers WiFi to be restarted: ", err)
			}

			if err := startChromeAndPerformAction(ctx, tf, test.verify); err != nil {
				s.Fatal("Failed to verify the test: ", err)
			}
		})
	}
}

// startChromeAndPerformAction will either re-use the existing Chrome session if one exists or
// start a new Chrome session. Afterwards, this function will execute |action|.
func startChromeAndPerformAction(ctx context.Context, tf *wificell.TestFixture, action action.Action) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := ui.NewChromeServiceClient(tf.DUTRPC(wificell.DefaultDUT).Conn)
	if _, err := cr.New(ctx, &ui.NewRequest{KeepState: true, TryReuseSession: true}); err != nil {
		return errors.Wrap(err, "failed to start Chrome")
	}
	defer cr.Close(cleanupCtx, &emptypb.Empty{})

	return action(ctx)
}

type apUtil struct {
	*wificell.APIface
	tf *wificell.TestFixture
}

func (ap *apUtil) connectAndWaitForItToBeKnownNetwork(ctx context.Context) error {
	if _, err := ap.tf.ConnectWifiAPFromDUT(ctx, wificell.DefaultDUT, ap.APIface); err != nil {
		return errors.Wrapf(err, "failed to connect to AP %q", ap.Config().SSID)
	}
	if err := ap.ensureConnectedState(true)(ctx); err != nil {
		return errors.Wrapf(err, "failed to wait for the AP %q is connected", ap.Config().SSID)
	}
	return nil
}

func (ap *apUtil) ensureConnectedState(state bool) action.Action {
	return func(ctx context.Context) error {
		if state {
			if err := ap.tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, ap.Config().SSID, true /* expectedValue */); err != nil {
				return err
			}

			wifiSvc := wifi.NewWifiServiceClient(ap.tf.DUTRPC(wificell.DefaultDUT).Conn)
			_, err := wifiSvc.KnownNetworksControls(ctx, &wifi.KnownNetworksControlsRequest{
				Ssids:   []string{ap.Config().SSID},
				Control: wifi.KnownNetworksControlsRequest_WaitUntilExist,
			})
			return err
		}

		// There could be a short latency before auto-connect to be effective,
		// the network is likely to be not connected at first when the WiFi is just came back, then automatically connect back shortly after.
		// Hence, "AP not connected" is verified by "wait for it to be connected and expecting an error".
		if err := ap.tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(ctx, ap.Config().SSID, true /* expectedValue */); err != nil {
			instantCheckCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
			defer cancel()
			// Call the same RPC again to check it's not connected as an error can indicates other failures.
			return ap.tf.DUTWifiClient(wificell.DefaultDUT).WaitForConnected(instantCheckCtx, ap.Config().SSID, false /* expectedValue */)
		}
		return errors.Errorf("the AP %q is connected", ap.Config().SSID)
	}
}

func (ap *apUtil) setAutoConnect(enable bool) action.Action {
	return func(ctx context.Context) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()

		req := &ossettings.OpenNetworkDetailPageRequest{
			NetworkName: ap.Config().SSID,
			NetworkType: ossettings.OpenNetworkDetailPageRequest_WIFI,
		}

		rpcClient := ap.tf.DUTRPC(wificell.DefaultDUT)
		osSettings := ossettings.NewOsSettingsServiceClient(rpcClient.Conn)
		if _, err := osSettings.OpenNetworkDetailPage(ctx, req); err != nil {
			return errors.Wrap(err, "failed to open network page")
		}
		defer osSettings.Close(cleanupCtx, &emptypb.Empty{})
		defer wifiutil.DumpUITreeWithScreenshotToFile(cleanupCtx, rpcClient.Conn, func() bool { return retErr != nil }, "set_auto_connect")

		option := &ossettings.SetToggleOptionRequest{
			ToggleOptionName: "Automatically connect to this network",
			Enabled:          enable,
		}
		if _, err := osSettings.SetToggleOption(ctx, option); err != nil {
			return errors.Wrapf(err, "failed to set toggle option to %v", enable)
		}
		return nil
	}
}

// cycleWifi refreshes the Wifi feature and then verifies the Wifi network status.
// The Wifi feature will be refreshed by disabling Wifi feature and then enabling Wifi feature.
func cycleWifi(ctx context.Context, tf *wificell.TestFixture) error {
	wifiClient := tf.DUTWifiClient(wificell.DefaultDUT)
	if err := wifiClient.SetWifiEnabled(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable Wifi feature")
	}
	if err := wifiClient.SetWifiEnabled(ctx, true); err != nil {
		return errors.Wrap(err, "failed to enable Wifi feature")
	}
	return nil
}

func suspendAndWake(ctx context.Context, tf *wificell.TestFixture) error {
	return tf.DUTWifiClient(wificell.DefaultDUT).Suspend(ctx, 10*time.Second)
}

func rebootDUT(ctx context.Context, tf *wificell.TestFixture) error {
	return tf.RebootDUT(ctx, wificell.DefaultDUT)
}
