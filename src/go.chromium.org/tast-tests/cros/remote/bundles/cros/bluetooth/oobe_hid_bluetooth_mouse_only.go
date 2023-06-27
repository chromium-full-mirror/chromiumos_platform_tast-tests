// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	crui "go.chromium.org/tast-tests/cros/remote/cros/ui"
	oobeui "go.chromium.org/tast-tests/cros/remote/cros/ui/oobeui"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OobeHidBluetoothMouseOnly,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that a bluetooth mouse is connected to in OOBE",
		Contacts: []string{
			"cros-connectivity@google.com",
			"tjohnsonkanu@google.com",
		},
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
		Timeout: time.Minute * 2,
	})
}

// OobeHidBluetoothMouseOnly tests that a single Bluetooth mouse is connected to during OOBE.
func OobeHidBluetoothMouseOnly(ctx context.Context, s *testing.State) {
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

	// Verify pointer device is not found.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.SearchingForPointerNodeName, defaultTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Discover btPeer as a mouse.
	mouseDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeMouse,
	})
	if err != nil {
		s.Fatalf("Failed to configure btpeer as a %s device: %s", mouseDevice.DeviceType(), err)
	}

	if result, err := mouseDevice.RPC().AdapterPowerOn(ctx); err != nil || !result {
		s.Fatal("Failed to power on btPeer adapter: ", err)
	}

	// Verify pointer device is found.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.FoundPointerNodeName, searchingTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Turn off mouse device and check that DUT is searching for mouse.
	if result, err := mouseDevice.RPC().AdapterPowerOff(ctx); err != nil || !result {
		s.Fatal("Failed to turn of btPeer adapter: ", err)
	}

	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.SearchingForPointerNodeName, defaultTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Turn on mouse device and check that mouse device is paired to.
	if result, err := mouseDevice.RPC().AdapterPowerOn(ctx); err != nil || !result {
		s.Fatal("Failed to power on btPeer adapter: ", err)
	}

	// Verify pointer device is found.
	if err := crui.CheckNodeWithNameExists(ctx, uiautoSvc, oobeui.FoundPointerNodeName, searchingTimeout); err != nil {
		s.Fatal("Failed to find node: ", err)
	}

	// Navigate to welcome screen.
	if _, err := uiautoSvc.LeftClick(
		ctx, &ui.LeftClickRequest{Finder: oobeui.ContinueButtonFinder}); err != nil {
		s.Fatal("Failed to click continue button: ", err)
	}

	if _, err := crUISvc.WaitForWelcomeScreen(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to enter welcome page")
	}
}
