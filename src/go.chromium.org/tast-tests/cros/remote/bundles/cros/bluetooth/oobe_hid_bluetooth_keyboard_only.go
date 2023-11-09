// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast/core/errors"
	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	util "go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/bluetoothutil"
	crui "go.chromium.org/tast-tests/cros/remote/cros/ui"
	oobeui "go.chromium.org/tast-tests/cros/remote/cros/ui/oobeui"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
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
		},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.ui.AutomationService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.bluetooth.BluetoothService",
		},
		HardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebase, hwdep.Chromebox, hwdep.Chromebit)),
		TestBedDeps:  []string{tbdep.WorkingBluetoothPeers(1)},
		Params: []testing.Param{
			{
				Name:      "floss_disabled",
				Fixture:   "chromeOobeWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
			},
			{
				Name:              "floss_enabled",
				Fixture:           "chromeOobeWith1BTPeerFlossEnabled",
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
			},
		},
		Timeout: time.Minute * 15,
	})
}

// OobeHidBluetoothKeyboardOnly tests that a single Blueooth keyboard is connected to during OOBE.
func OobeHidBluetoothKeyboardOnly(ctx context.Context, s *testing.State) {
	// This test waits for UI elements to become visible that frequently take more than the default of 15 seconds.
	const defaultTimeout time.Duration = time.Second * 90

	// Bluetooth peers have been observed to take longer than |defaultTimeout| to become ready and be found.
	const searchingTimeout time.Duration = time.Second * 300

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

	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.SearchingForKeyboardNodeName, searchingTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Discover btpeer as a keyboard.
	keyboardDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeKeyboard,
	})
	if err != nil {
		s.Fatalf("Failed to configure btpeer as a %s device: %s", keyboardDevice.DeviceType(), err)
	}

	pollForPairedAndConnectedDevice := func() {
		testing.ContextLog(ctx, "Waiting for Bluetooth keyboard device to be paired and connected")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			resp, err := fv.BluetoothService.DeviceIsPaired(ctx, &bts.DeviceIsPairedRequest{
				DeviceAddress: keyboardDevice.LocalBluetoothAddress(),
			})
			if err != nil {
				return errors.Wrap(err, "failed to check if keyboard device is paired")
			}
			if !resp.DeviceIsPaired {
				return errors.Wrap(err, "keyboard device not paired as expected")
			}

			isConnectedResp, err := fv.BluetoothService.DeviceIsConnected(ctx, &bts.DeviceIsConnectedRequest{
				DeviceAddress: keyboardDevice.LocalBluetoothAddress(),
			})

			if err != nil {
				return errors.Wrap(err, "failed to check if keyboard device is connected")
			}
			if !isConnectedResp.DeviceIsConnected {
				return errors.New("keyboard device not connected as expected")
			}

			return nil
		}, &testing.PollOptions{
			Timeout:  defaultTimeout,
			Interval: 5000 * time.Millisecond,
		}); err != nil {
			s.Fatal("Keyboard device not paired or connected: ", err)
		}
	}

	pollForPairedAndConnectedDevice()

	pairedNodeName := fmt.Sprintf("\"%s Keyboard\" paired", keyboardDevice.AdvertisedName())

	testing.ContextLog(ctx, "Checking that Bluetooth Keyboard was found")

	// Verify keyboard device is pairing.
	// TODO(b/254524000): use approraite authentication method.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, pairedNodeName, searchingTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	testing.ContextLog(ctx, "Turning off Bluetooth adapter")

	if _, err := keyboardDevice.RPC().AdapterPowerOff(ctx); err != nil {
		s.Fatal("Failed to turn of btpeer adapter: ", err)
	}

	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.SearchingForKeyboardNodeName, searchingTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	testing.ContextLog(ctx, "Turning on and initializing btpeer adapter")

	// Turn Bluetooth adapter on.
	if _, err := keyboardDevice.RPC().AdapterPowerOn(ctx); err != nil {
		s.Fatal("Failed to turn of btpeer adapter: ", err)
	}

	// Initialize BTPeer adapter and ensure adapter is in the correct state before pairing.
	if err := keyboardDevice.RPC().Init(ctx, false); err != nil {
		s.Fatal("Failed to set init to true: ", err)
	}

	pollForPairedAndConnectedDevice()

	testing.ContextLog(ctx, "Checking that paired Bluetooth keyboard node is found")

	// Verify keyboard device is paired.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, pairedNodeName, searchingTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	testing.ContextLog(ctx, "Checking that continue button is enabled")

	// Wait for continue button to become enabled.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if res, err := uiautoSvc.Info(
			ctx, &ui.InfoRequest{Finder: oobeui.ContinueButtonFinder}); err != nil {
			return errors.Wrap(err, " failed to get restriction of continue button")
		} else if res.NodeInfo.Restriction != ui.Restriction_RESTRICTION_NONE {
			return errors.Wrap(err, " continue button is disabled")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  15 * time.Second,
		Interval: 1000 * time.Millisecond,
	}); err != nil {
		s.Fatal("Continue button is not enabled: ", err)
	}

	testing.ContextLog(ctx, "Clicking the continue button")

	// Navigate to welcome screen.
	if _, err := uiautoSvc.LeftClick(
		ctx, &ui.LeftClickRequest{Finder: oobeui.ContinueButtonFinder}); err != nil {
		s.Fatal("Failed to click continue button: ", err)
	}

	testing.ContextLog(ctx, "Waiting for welcome screen")

	if _, err := crUISvc.WaitForWelcomeScreen(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to enter welcome page: ", err)
	}
}
