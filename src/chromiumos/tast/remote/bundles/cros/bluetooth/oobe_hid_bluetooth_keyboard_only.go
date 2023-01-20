// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	cbt "chromiumos/tast/common/chameleon/devices/common/bluetooth"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/bluetooth"
	util "chromiumos/tast/remote/bundles/cros/bluetooth/bluetoothutil"
	crui "chromiumos/tast/remote/cros/ui"
	oobeui "chromiumos/tast/remote/cros/ui/oobeui"
	"chromiumos/tast/services/cros/ui"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OobeHidBluetoothKeyboardOnly,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that a bluetooth keyboard can be used to complete OOBE",
		Contacts: []string{
			"cros-connectivity@google.com",
			"tjohnsonkanu@google.com",
		},
		VarDeps:      []string{"servo"},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr: []string{
			"group:bluetooth",
			"bluetooth_btpeers_1",
			"bluetooth_flaky",
		},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.ui.AutomationService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.bluetooth.BTTestService",
		},
		Fixture:      "chromeOobeWith1BTPeer",
		HardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebox, hwdep.Chromebit)),
	})
}

// OobeHidBluetoothKeyboardOnly tests that a single Blueooth keyboard is connected to during OOBE.
func OobeHidBluetoothKeyboardOnly(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
	defer cancel()

	uiautoSvc := ui.NewAutomationServiceClient(fv.DUTRPCClient.Conn)
	crUISvc := ui.NewChromeUIServiceClient(fv.DUTRPCClient.Conn)

	defer func() {
		if !s.HasError() {
			return
		}
		if _, err := crUISvc.DumpUITree(cleanupCtx, &emptypb.Empty{}); err != nil {
			testing.ContextLog(cleanupCtx, "Failed to dump UI tree: ", err)
		}
	}()

	util.TurnOffServoKeyboardIfOn(ctx, s)

	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.SearchingForKeyboardNodeName); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Discover btPeer as a keyboard.
	keyboardDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeKeyboard,
	})
	if err != nil {
		s.Fatalf("Failed to configure btpeer as a %s device: %s", keyboardDevice.DeviceType(), err)
	}

	// Verify keyboard device is pairing.
	// TODO(b/254524000): use approraite authentication method.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.PairingKeyboardNodeName); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	if _, err := keyboardDevice.RPC().AdapterPowerOff(ctx); err != nil {
		s.Fatal("Failed to turn of btPeer adapter: ", err)
	}

	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.SearchingForKeyboardNodeName); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Turn on keyboard device and check that keyboard device is paired to.
	if _, err := keyboardDevice.RPC().AdapterPowerOn(ctx); err != nil {
		s.Fatal("Failed to power on btPeer adapter: ", err)
	}

	// Verify keyboard device is pairing.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.PairingKeyboardNodeName); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// TODO(b/254524000): Navigate to welcome screen.
}
