// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/durationpb"
	"google.golang.org/protobuf/types/known/emptypb"

	cbt "go.chromium.org/tast-tests/cros/common/chameleon/devices/common/bluetooth"
	"go.chromium.org/tast-tests/cros/remote/bluetooth"
	bts "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ForgetDevice,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Verify that user can forget paired/connected and paired/disconnected devices by clicking the 'forget' button",
		Contacts: []string{
			// "cros-connectivity@google.com",
			// "cienet-development@googlegroups.com",
			"cj.tsai@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth", "bluetooth_btpeers_1"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps: []string{
			"tast.cros.bluetooth.BluetoothUIService",
			"tast.cros.ui.AutomationService",
			"tast.cros.ui.ChromeUIService",
		},
		Params: []testing.Param{
			{
				Name:      "forget_connected_device__floss_disabled",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       true, /* expectDeviceToBeConnected */
			}, {
				Name:              "forget_connected_device__floss_enabled",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               true, /* expectDeviceToBeConnected */
			}, {
				Name:      "forget_disconnected_device__floss_disabled",
				Fixture:   "chromeLoggedInWith1BTPeerFlossDisabled",
				ExtraAttr: []string{"bluetooth_flaky"},
				Val:       false, /* expectDeviceToBeConnected */

			}, {
				Name:              "forget_disconnected_device__floss_enabled",
				Fixture:           "chromeLoggedInWith1BTPeerFlossEnabled",
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
				Val:               false, /* expectDeviceToBeConnected */
			},
		},
	})
}

// ForgetDevice verifies that user can forget paired/connected and paired/disconnected devices by clicking the "forget" button.
func ForgetDevice(ctx context.Context, s *testing.State) {
	fv := s.FixtValue().(*bluetooth.FixtValue)
	expectDeviceToBeConnected := s.Param().(bool)

	device, err := bluetooth.NewEmulatedBTPeerDevice(ctx, fv.BTPeers[0], &bluetooth.EmulatedBTPeerDeviceConfig{
		DeviceType: cbt.DeviceTypeKeyboard,
	})
	if err != nil {
		s.Fatal("Failed to configure btpeer: ", err)
	}

	if _, err = fv.BluetoothUIService.PairDeviceWithQuickSettings(ctx, &bts.PairDeviceWithQuickSettingsRequest{
		AdvertisedName: device.AdvertisedName(),
	}); err != nil {
		s.Fatal("Failed to pair device with UI: ", err)
	}

	if !expectDeviceToBeConnected {
		// Power off the Bluetooth device to disconnect it from ChromeOS.
		if err := device.RPC().AdapterPowerOff(ctx); err != nil {
			s.Fatal("Failed to power off the device: ", err)
		}
	}

	// Verify that the state of the device is as expected.
	// The OS-Settings may not ready immediately after the connection status of the
	// Bluetooth device changes.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		resp, err := fv.BluetoothUIService.CollectDeviceList(ctx, &emptypb.Empty{})
		if err != nil {
			return errors.Wrap(err, "failed to get the list of Bluetooth devices")
		}

		for _, foundDevice := range resp.Devices {
			if foundDevice.Name == device.AdvertisedName() {
				if foundDevice.IsConnected != expectDeviceToBeConnected {
					return errors.Errorf("failed to verify the connected status of %q; got: %t, want: %t", device.AdvertisedName(), foundDevice.IsConnected, expectDeviceToBeConnected)
				}
				return nil
			}
		}
		return errors.New("failed to find expected device")
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 3 * time.Second}); err != nil {
		s.Fatal("Failed to verify the state of the device: ", err)
	}

	// Verify that user have a way to forget both connected and not-connected Bluetooth device.
	if _, err := fv.BluetoothUIService.ForgetBluetoothDevice(ctx, &bts.ForgetBluetoothDeviceRequest{
		DeviceName: device.AdvertisedName(),
	}); err != nil {
		s.Fatalf("Failed to forget Bluetooth device %q: %v", device.AdvertisedName(), err)
	}

	if err := verifyDisconnectedPromptBehavior(ctx, fv.DUTRPCClient.Conn, device.AdvertisedName(), expectDeviceToBeConnected); err != nil {
		s.Fatal("Failed to verify disconnected prompt behavior: ", err)
	}

	resp, err := fv.BluetoothUIService.CollectDeviceList(ctx, &emptypb.Empty{})
	if err != nil {
		s.Fatal("Failed to get the list of Bluetooth devices: ", err)
	}

	for _, foundDevice := range resp.Devices {
		// Verifying the Bluetooth device is indeed forgotten.
		if foundDevice.Name == device.AdvertisedName() {
			s.Fatalf("Failed to verify Bluetooth device %q is forgotten: device is still remembered", device)
		}
	}

}

func verifyDisconnectedPromptBehavior(ctx context.Context, conn *grpc.ClientConn, deviceName string, forgetFromConnectedDevice bool) (retErr error) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		if retErr != nil {
			faillog := ui.NewChromeUIServiceClient(conn)
			faillog.DumpUITreeWithScreenshotToFile(ctx, &ui.DumpUITreeWithScreenshotToFileRequest{
				FilePrefix: fmt.Sprintf("%s_disconnected_prompt", deviceName),
			})
		}
	}(cleanupCtx)

	uiSvc := ui.NewAutomationServiceClient(conn)
	toastOverlay := ui.Node().HasClass("ToastOverlay").Finder()
	disconnectedPrompt := ui.Node().Name(fmt.Sprintf("%s disconnected", deviceName)).Role(ui.Role_ROLE_STATIC_TEXT).Ancestor(toastOverlay)

	// The disconnected prompt should popup only when forgetting from a connected device.
	if forgetFromConnectedDevice {
		_, err := uiSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{
			Finder: disconnectedPrompt.Finder(),
		})
		return err
	}
	_, err := uiSvc.EnsureGone(ctx, &ui.EnsureGoneRequest{
		Finder:  disconnectedPrompt.Finder(),
		Timeout: &durationpb.Duration{Seconds: 5},
	})
	return err
}
