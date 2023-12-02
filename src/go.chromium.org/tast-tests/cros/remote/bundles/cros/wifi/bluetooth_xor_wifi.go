// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/services/cros/bluetooth"

	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BluetoothXorWifi,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that Bluetooth and Wifi can function when the other phy is disabled",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func", "wificell_reboot"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.WifiStateNormal, tbdep.PeripheralWifiStateWorking},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			wificell.BluetoothServiceName,
		},
		Vars: []string{"router"},
		// List of requirements this test satisfies.
		Requirements:    []string{tdreq.WiFiCoexSupportBT, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func BluetoothXorWifi(ctx context.Context, s *testing.State) {
	// Clean up on exit.
	defer func(ctx context.Context) {
		d := s.DUT()
		r, err := rpc.Dial(ctx, d, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect rpc: ", err)
		}
		defer r.Close(ctx)
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()
		// Enable Bluetooth device.
		bluetoothService, err := wifiutil.NewBluetoothServiceClient(ctx, r.Conn)
		if err != nil {
			s.Error("Failed to create new bluetooth service client: ", err)
		} else {
			if _, err := bluetoothService.Enable(ctx, &empty.Empty{}); err != nil {
				s.Error("Failed to re-enable bluetooth: ", err)
			}
		}
		wifiClient := wifi.NewShillServiceClient(r.Conn)
		// Enable WiFi.
		if _, err := wifiClient.SetWifiEnabled(ctx, &wifi.SetWifiEnabledRequest{Enabled: true}); err != nil {
			s.Error("Could not enable WiFi: ", err)
		}
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Initialize TestFixture Options.
	tfOps := wificell.NewTFOptionsBuilder()
	tfOps.DutTarget(s.DUT(), s.RPCHint())
	if router, ok := s.Var("router"); ok && router != "" {
		tfOps.PrimaryRouterTargets(router)
	}
	tfOps.EnableDutUI(true)

	// Assert WiFi is up.
	// TODO(b/279663413): Tests should not manually initialize the wifi test fixture class.
	tf, err := wificell.NewTestFixture(ctx, ctx, tfOps.Build())
	if err != nil {
		s.Fatal("Failed to set up test fixture: ", err)
	}
	defer func(ctx context.Context) {
		if err := tf.Close(ctx); err != nil {
			s.Error("Failed to properly take down test fixture: ", err)
		}
	}(ctx)
	ctx, cancel = tf.ReserveForClose(ctx)
	defer cancel()

	if err := wifiutil.AssertWifiEnabled(ctx, tf); err != nil {
		s.Fatal("Wifi not functioning: ", err)
	}

	// Initialize gRPC connection with DUT.
	d := s.DUT()
	r, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect rpc: ", err)
	}
	defer r.Close(ctx)

	// Validate phys can function without the other on multiple channels
	channels := [4]int{36, 149, 1, 11}
	wifiClient := wifi.NewShillServiceClient(r.Conn)
	bluetoothService, err := wifiutil.NewBluetoothServiceClient(ctx, r.Conn)
	if err != nil {
		s.Fatal("Failed to create new bluetooth service client: ", err)
	}
	for _, ch := range channels {
		if err := togglePhys(ctx, ch, bluetoothService, tf, wifiClient, true); err != nil {
			s.Fatalf("Failed to run WiFi without Bluetooth path on channel %d: %v", ch, err)
		}
		if err := togglePhys(ctx, ch, bluetoothService, tf, wifiClient, false); err != nil {
			s.Fatalf("Failed to run Bluetooth without WiFi path on channel %d: %v", ch, err)
		}
	}
}

func togglePhys(ctx context.Context, channel int, bluetoothService bluetooth.BluetoothServiceClient, tf *wificell.TestFixture, wifiClient wifi.ShillServiceClient, enableWifiFirst bool) error {
	// Disable and Assert Wifi is down
	if err := setAssertWifi(ctx, tf, wifiClient, []int{}, false); err != nil {
		return err
	}
	// Disable Bluetooth and assert Bluetooth is down.
	if err := setAssertBluetooth(ctx, bluetoothService, false); err != nil {
		return err
	}

	if enableWifiFirst {
		// Enable and Assert WiFi is up
		if err := setAssertWifi(ctx, tf, wifiClient, []int{channel}, true); err != nil {
			return err
		}
		// Enable and Assert Bluetooth is up.
		if err := setAssertBluetooth(ctx, bluetoothService, true); err != nil {
			return err
		}
	} else {
		// Enable and Assert Bluetooth is up.
		if err := setAssertBluetooth(ctx, bluetoothService, true); err != nil {
			return err
		}
		// Enable and Assert WiFi is up
		if err := setAssertWifi(ctx, tf, wifiClient, []int{channel}, true); err != nil {
			return err
		}
	}
	return nil
}

func setAssertBluetooth(ctx context.Context, bluetoothService bluetooth.BluetoothServiceClient, enabled bool) error {
	if enabled {
		// Enable Bluetooth and assert Bluetooth is up.
		testing.ContextLog(ctx, "Enabling bluetooth")
		if _, err := bluetoothService.Enable(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to enable bluetooth")
		}
		testing.ContextLog(ctx, "Verifying that bluetooth has been enabled")
		if err := wifiutil.PollBluetoothPoweredStatus(ctx, bluetoothService, true); err != nil {
			return errors.Wrap(err, "failed to verify bluetooth adapter was powered on")
		}
		if err := wifiutil.ValidateBluetoothFunctional(ctx, bluetoothService); err != nil {
			return errors.Wrap(err, "failed to validate Bluetooth status")
		}
	} else {
		// Disable Bluetooth and assert Bluetooth is down.
		testing.ContextLog(ctx, "Disabling bluetooth")
		if _, err := bluetoothService.Disable(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to disable bluetooth")
		}
		testing.ContextLog(ctx, "Verifying that bluetooth has been disabled")
		if err := wifiutil.PollBluetoothPoweredStatus(ctx, bluetoothService, false); err != nil {
			return errors.Wrap(err, "failed to verify bluetooth adapter was powered off")
		}
	}
	return nil
}

func setAssertWifi(ctx context.Context, tf *wificell.TestFixture, wifiClient wifi.ShillServiceClient, channels []int, enabled bool) error {
	if enabled {
		// Enable WiFi.
		if _, err := wifiClient.SetWifiEnabled(ctx, &wifi.SetWifiEnabledRequest{Enabled: true}); err != nil {
			return errors.Wrap(err, "could not enable WiFi")
		}
		// Assert WiFi is functional on channel.
		if len(channels) != 1 {
			return errors.Errorf("unexpected number of channels provided, got %d", len(channels))
		}
		if err := wifiutil.AssertWifiEnabledOnChannel(ctx, tf, channels[0]); err != nil {
			return errors.Wrap(err, "Wifi not functioning")
		}
	} else {
		// Disable WiFi.
		if _, err := wifiClient.SetWifiEnabled(ctx, &wifi.SetWifiEnabledRequest{Enabled: false}); err != nil {
			return errors.Wrap(err, "could not disable WiFi")
		}
		// Assert WiFi is down.
		if response, err := wifiClient.GetWifiEnabled(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "could not get WiFi status")
		} else if response.Enabled {
			return errors.Wrap(err, "Wifi is on, expected to be off")
		}
	}
	return nil
}
