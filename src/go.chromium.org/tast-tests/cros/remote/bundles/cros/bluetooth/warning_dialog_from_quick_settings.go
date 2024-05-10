// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast/core/errors"
	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	util "go.chromium.org/tast-tests/cros/remote/bundles/cros/bluetooth/bluetoothutil"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           WarningDialogFromQuickSettings,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Checks that a bluetooth mouse can be used to complete OOBE",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		VarDeps:      []string{"servo"},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
		Attr:         []string{"group:bluetooth"},
		TestBedDeps:  []string{tbdep.Wificell, tbdep.BluetoothStateNormal, tbdep.WorkingBluetoothPeers(2)},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.ui.AutomationService",
			"tast.cros.ui.ChromeUIService",
			"tast.cros.bluetooth.BluetoothService",
			"tast.cros.bluetooth.BluetoothUIService",
			"tast.cros.chrome.uiauto.quicksettings.QuickSettingsService",
		},
		HardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromebase, hwdep.Chromebox, hwdep.Chromebit)),
		Params: []testing.Param{{
			Name:      "floss_disabled",
			Fixture:   "chromeLoggedInWith2BTPeersBluetoothWarningEnabledFlossDisabled",
			ExtraAttr: []string{"bluetooth_sa"},
			Val:       cbt.DeviceTypeLEMouse,
		}, {
			Name:              "floss_enabled",
			Fixture:           "chromeLoggedInWith2BTPeersBluetoothWarningEnabledFlossEnabled",
			ExtraAttr:         []string{"bluetooth_floss"},
			ExtraSoftwareDeps: []string{"bluetooth_floss"},
			Val:               cbt.DeviceTypeLEMouse,
		}},
		Timeout: time.Minute * 3,
	})
}

// WarningDialogFromQuickSettings tests that a user can successfully toggle the Bluetooth state using the Bluetooth
// feature pod icon button within the Quick Settings.
// TODO (b/330731210): Add Bluetooth test using keyboard when emulated keyboards are fixed.
func WarningDialogFromQuickSettings(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	deviceType := s.Param().(cbt.DeviceType)

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 5*time.Second)
	defer cancel()

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

	// Discover btpeer as a mouse.
	mouseDevice, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: deviceType,
	})
	if err != nil {
		s.Fatalf("Failed to configure btpeer as a %q device: %v", mouseDevice.DeviceType(), err)
	}

	if _, err := fv.BluetoothUIService.PairDeviceWithQuickSettings(ctx, &bts.PairDeviceWithQuickSettingsRequest{
		AdvertisedName: mouseDevice.AdvertisedName(),
	}); err != nil {
		s.Fatalf("Failed to pair the Bluetooth device %q with quick settings: %v", mouseDevice.String(), err)
	}

	// Enable Bluetooth
	fv.BluetoothService.Enable(ctx, &emptypb.Empty{})

	checkBluetoothState := func(expectedState bool) {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			powered, err := fv.BluetoothService.IsPoweredOn(ctx, &emptypb.Empty{})
			if err != nil {
				return errors.Wrap(err, "failed to get Bluetooth powered state")
			}

			if powered.IsPoweredOn != expectedState {
				return errors.Wrapf(err, "bluetooth powered state is not as expected, got %q was expecting %v", powered, expectedState)
			}
			return nil
		}, &testing.PollOptions{
			Timeout:  time.Second * 10,
			Interval: 2000 * time.Millisecond,
		}); err != nil {
			s.Fatal("Bluetooth is not in expected state: ", err)
		}
	}

	checkBluetoothState(true)

	if _, err := fv.QuickSettingsService.ToggleBluetoothAndSelectKeepOnInBluetoothWarningDialog(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to select keep on in warning dialog: ", err)
	}

	checkBluetoothState(true)

	if _, err := fv.QuickSettingsService.ToggleBluetoothAndSelectTurnOffInBluetoothWarningDialog(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to select keep off in warning dialog: ", err)
	}

	checkBluetoothState(false)

	if _, err := fv.QuickSettingsService.ToggleBluetoothFromFeaturePod(ctx, &emptypb.Empty{}); err != nil {
		s.Fatal("Failed to toggle Bluetooth state on from feature pod: ", err)
	}

	checkBluetoothState(true)
}
