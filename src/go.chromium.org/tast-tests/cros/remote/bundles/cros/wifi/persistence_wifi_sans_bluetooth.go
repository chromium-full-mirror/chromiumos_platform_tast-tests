// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wifi

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	tdreq "go.chromium.org/tast-tests/cros/common/testdevicerequirements"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wifi/wifiutil"
	"go.chromium.org/tast-tests/cros/remote/wificell"
	"go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PersistenceWifiSansBluetooth,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that WiFi remains operational when Bluetooth is disabled on reboot",
		Contacts: []string{
			"chromeos-wifi-champs@google.com", // WiFi oncall rotation; or http://b/new?component=893827
		},
		BugComponent: "b:893827", // ChromeOS > Platform > Connectivity > WiFi
		Attr:         []string{"group:wificell", "wificell_func"},
		// Jacuzzi devices are prone to becoming inaccessible over ethernet on reboot which impacts future tests in the test suite (b/178529170).
		// We will disable the persistence tests on jacuzzi devices as these tests perform a reboot (b:181057823).
		// We choose not to use hwdep.SkipOnPlatform as the filter relies on an identifier internal to mosys.
		// In the case for jacuzzi, the relevant platform identifier is actually kukui which is unintuitive (crbug.com/1124372).
		// As a result, we have defined a softwaredep, no_eth_loss_on_reboot, to service as a skiplist for this test.
		// TODO: remove this swdep when the jacuzzi issue is fixed (b:178449023)
		SoftwareDeps: []string{"chrome", "reboot", "no_eth_loss_on_reboot"},
		ServiceDeps: []string{
			wificell.ShillServiceName,
			"tast.cros.browser.ChromeService",
			wificell.BluetoothServiceName,
		},
		Vars:    []string{"router"},
		VarDeps: []string{"wifi.signinProfileTestExtensionManifestKey"},
		// As a workaround to b:239583375, we increase the test duration as this test reinitializes a new test fixture
		// which causes a second router reboot on openwrt routers. We have not noticed timeout cases on gales.
		Timeout: 10 * time.Minute,
		// List of requirements this test satisfies.
		Requirements:    []string{tdreq.WiFiCoexSupportBT, tdreq.WiFiProcPassFW, tdreq.WiFiProcPassAVL, tdreq.WiFiProcPassAVLBeforeUpdates, tdreq.WiFiProcPassMatfunc, tdreq.WiFiProcPassMatfuncBeforeUpdates},
		VariantCategory: `{"name": "WifiBtChipset_Soc_Kernel"}`,
	})
}

func PersistenceWifiSansBluetooth(ctx context.Context, s *testing.State) {
	// Clean up on exit.
	credKey := s.RequiredVar("wifi.signinProfileTestExtensionManifestKey")
	defer func(ctx context.Context) {
		d := s.DUT()
		r, err := rpc.Dial(ctx, d, s.RPCHint())
		if err != nil {
			s.Fatal("Failed to connect rpc: ", err)
		}
		defer r.Close(ctx)

		// Re-enable Bluetooth in cleanup.
		chromeService, err := wifiutil.NewChromeServiceClient(ctx, r.Conn, credKey)
		if err != nil {
			s.Fatal("Failed to create new chrome service client: ", err)
		}
		defer func(ctx context.Context) {
			_, err := chromeService.Close(ctx, &empty.Empty{})
			if err != nil {
				s.Error("Failed to close chrome service client: ", err)
			}
		}(ctx)
		bluetoothService, err := wifiutil.NewBluetoothServiceClient(ctx, r.Conn)
		if err != nil {
			s.Fatal("Failed to create new bluetooth service client: ", err)
		}
		s.Logf("Setting bluetooth enabled on boot setting to %t", true)
		if _, err := bluetoothService.SetEnabledOnBoot(ctx, &bluetooth.SetEnabledOnBootRequest{
			AdapterEnabledOnBoot: true,
		}); err != nil {
			s.Fatal("Failed to set bluetooth enabled on boot: ", err)
		}
		s.Log("Re-enabling bluetooth on DUT")
		if _, err := bluetoothService.Enable(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to re-enable bluetooth: ", err)
		}
	}(ctx)
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Initialize TestFixture Options.
	tfOpsBuilder := wificell.NewTFOptionsBuilder()
	tfOpsBuilder.DutTarget(s.DUT(), s.RPCHint())
	if router, ok := s.Var("router"); ok && router != "" {
		tfOpsBuilder.PrimaryRouterTargets(router)
	}
	tfOpsBuilder.EnableDutUI(true)
	originalTfOps := tfOpsBuilder.Build()

	func(ctx context.Context) {
		// Assert WiFi is up.
		// TODO(b/279663413): Tests should not manually initialize the wifi test fixture class.
		tf, err := wificell.NewTestFixture(ctx, ctx, originalTfOps)
		if err != nil {
			s.Fatal("Failed to set up test fixture: ", err)
		}
		defer func(ctx context.Context) {
			if err := tf.Close(ctx); err != nil {
				s.Error("Failed to properly take down test fixture: ", err)
			}
		}(ctx)
		ctx, cancel := tf.ReserveForClose(ctx)
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

		// Disable Bluetooth and assert Bluetooth is down.
		chromeService, err := wifiutil.NewChromeServiceClient(ctx, r.Conn, credKey)
		if err != nil {
			s.Fatal("Failed to create new chrome service client: ", err)
		}
		defer func(ctx context.Context) {
			_, err := chromeService.Close(ctx, &empty.Empty{})
			if err != nil {
				s.Error("Failed to close chrome service client: ", err)
			}
		}(ctx)
		bluetoothService, err := wifiutil.NewBluetoothServiceClient(ctx, r.Conn)
		if err != nil {
			s.Fatal("Failed to create new bluetooth service client: ", err)
		}
		s.Logf("Setting bluetooth enabled on boot setting to %t", false)
		if _, err := bluetoothService.SetEnabledOnBoot(ctx, &bluetooth.SetEnabledOnBootRequest{
			AdapterEnabledOnBoot: false,
		}); err != nil {
			s.Fatal("Failed to set bluetooth disabled on boot: ", err)
		}
		s.Log("Disabling bluetooth")
		if _, err := bluetoothService.Disable(ctx, &empty.Empty{}); err != nil {
			s.Fatal("Failed to disable bluetooth: ", err)
		}
		if err := wifiutil.AssertBluetoothEnabledState(ctx, bluetoothService, false); err != nil {
			s.Fatal("Failed verify bluetooth was disabled: ", err)
		}
	}(ctx)

	// Reboot the DUT.
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot DUT: ", err)
	}

	// Reinitialize gRPC connection with DUT after reboot as the current session is now stale.
	d := s.DUT()
	r, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect rpc: ", err)
	}
	defer r.Close(ctx)

	// Assert Bluetooth is down.
	chromeService, err := wifiutil.NewChromeServiceClient(ctx, r.Conn, credKey)
	if err != nil {
		s.Fatal("Failed to create new chrome service client: ", err)
	}
	defer func(ctx context.Context) {
		_, err := chromeService.Close(ctx, &empty.Empty{})
		if err != nil {
			s.Error("Failed to close chrome service client: ", err)
		}
	}(ctx)
	bluetoothService, err := wifiutil.NewBluetoothServiceClient(ctx, r.Conn)
	if err != nil {
		s.Fatal("Failed to create new bluetooth service client")
	}
	if err := wifiutil.AssertBluetoothEnabledState(ctx, bluetoothService, false); err != nil {
		s.Fatal("Failed to assert that bluetooth is disabled: ", err)
	}

	// Assert WiFi is up.
	// TODO(b/279663413): Tests should not manually initialize the wifi test fixture class.
	tfOpsBuilder = wificell.NewTFOptionsBuilder()
	tfOpsBuilder.DutTarget(d, s.RPCHint())
	tfOpsBuilder.PrimaryRouterTargets(originalTfOps.PrimaryRouterTargets...)
	tfOpsBuilder.EnableDutUI(originalTfOps.EnableDutUI)
	tf, err := wificell.NewTestFixture(ctx, ctx, tfOpsBuilder.Build())
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
}
