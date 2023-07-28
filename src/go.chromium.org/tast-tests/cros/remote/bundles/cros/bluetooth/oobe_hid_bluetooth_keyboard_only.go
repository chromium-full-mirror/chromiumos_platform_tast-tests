// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	util "go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/bluetoothutil"
	crui "go.chromium.org/tast-tests/cros/remote/cros/ui"
	oobeui "go.chromium.org/tast-tests/cros/remote/cros/ui/oobeui"
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
		},
		HardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebase, hwdep.Chromebox, hwdep.Chromebit)),
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
		Timeout: time.Minute * 5,
	})
}

// OobeHidBluetoothKeyboardOnly tests that a single Blueooth keyboard is connected to during OOBE.
func OobeHidBluetoothKeyboardOnly(ctx context.Context, s *testing.State) {
	// This test waits for UI elements to become visible that frequently take more than the default of 15 seconds.
	const defaultTimeout time.Duration = time.Second * 30

	// Bluetooth peers have been observed to take longer than |defaultTimeout| to become ready and be found.
	const searchingTimeout time.Duration = time.Second * 90

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

	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.SearchingForKeyboardNodeName, defaultTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Discover btpeer as a keyboard.
	keyboardDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeKeyboard,
	})
	if err != nil {
		s.Fatalf("Failed to configure btpeer as a %s device: %s", keyboardDevice.DeviceType(), err)
	}

	pairedNodeName := fmt.Sprintf("\"%s Keyboard\" paired", keyboardDevice.AdvertisedName())

	// Verify keyboard device is pairing.
	// TODO(b/254524000): use approraite authentication method.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, pairedNodeName, searchingTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	if _, err := keyboardDevice.RPC().AdapterPowerOff(ctx); err != nil {
		s.Fatal("Failed to turn of btpeer adapter: ", err)
	}

	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.SearchingForKeyboardNodeName, defaultTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Turn on keyboard device and check that keyboard device is paired to.
	if _, err := keyboardDevice.RPC().AdapterPowerOn(ctx); err != nil {
		s.Fatal("Failed to power on btpeer adapter: ", err)
	}

	// Verify keyboard device is pairing.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, pairedNodeName, searchingTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	if res, err := uiautoSvc.Info(
		ctx, &ui.InfoRequest{Finder: oobeui.ContinueButtonFinder}); err != nil {
		s.Fatal("Failed to get restriction of continue button: ", err)
	} else {
		testing.ContextLog(ctx, "Continue button has restriction: ", res.NodeInfo.Restriction)
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
