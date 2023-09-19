// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	pb "go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterBluetoothUIServiceServer(srv, &BtUIService{
				s:            s,
				sharedObject: common.SharedObjectsForServiceSingleton,
			})
		},
		// GuaranteeCompatibility allows non-Tast test harness clients to call this service.
		GuaranteeCompatibility: true,
	})
}

// BtUIService implements tast.cros.bluetooth.BluetoothUIService.
type BtUIService struct {
	s            *testing.ServiceState
	sharedObject *common.SharedObjectsForService
}

// PairWithFastPairNotification will attempt to pair a fast pair device with
// the fast pair notification. The |request| contains a Protocol which must be
// either Initial or Subsequent for this function.
func (bui *BtUIService) PairWithFastPairNotification(ctx context.Context, request *pb.PairWithFastPairNotificationRequest) (*emptypb.Empty, error) {
	cr := bui.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tConn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	// The Initial and Subsequent scenarios have slightly different notifications.
	expectedNotificationID := ""
	if request.Protocol == pb.FastPairProtocol_FAST_PAIR_PROTOCOL_INITIAL {
		expectedNotificationID = bluetooth.NotificationIDFastPairDiscoveryUser
	} else if request.Protocol == pb.FastPairProtocol_FAST_PAIR_PROTOCOL_SUBSEQUENT {
		expectedNotificationID = bluetooth.NotificationIDFastPairSubsequentPair
	} else {
		return nil, errors.New("wrong protocol requested; only initial and subsequent scenarios are supported")
	}

	// Wait for the discovery notification.
	testing.ContextLog(ctx, "Waiting for fast pair discovery notification")
	_, err = ash.WaitForNotification(
		ctx,
		tConn,
		30*time.Second,
		ash.WaitIDContains(expectedNotificationID),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for fast pair discovery notification to appear")
	}

	// Click the connect button on the notification.
	testing.ContextLog(ctx, "Starting fast pair pairing process")
	connectBtn := nodewith.Role(role.Button).Name("Connect")
	ac := uiauto.New(tConn)
	if err := ac.DoDefault(connectBtn)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to click connect button on fast pair discovery notification")
	}

	// Wait for pairing notification to appear and disappear.
	_, err = ash.WaitForNotification(
		ctx,
		tConn,
		1*time.Minute,
		ash.WaitIDContains(bluetooth.NotificationIDFastPairPairing),
	)
	if err != nil {
		return nil, errors.Wrap(err, "failed to wait for fast pair pairing notification to appear")
	}
	if err := ash.WaitUntilNotificationGone(
		ctx,
		tConn,
		1*time.Minute,
		ash.WaitIDContains(bluetooth.NotificationIDFastPairPairing),
	); err != nil {
		return nil, errors.Wrap(err, "failed to wait for fast pair pairing notification to disappear")
	}

	// Check to make sure error notification does not appear.
	fastPairErrorNotification, err := ash.WaitForNotification(
		ctx,
		tConn,
		2*time.Second,
		ash.WaitIDContains(bluetooth.NotificationIDFastPairError),
	)
	if err == nil {
		return nil, errors.Errorf("fast pair pairing error notification found: title=%q, message=%q", fastPairErrorNotification.Title, fastPairErrorNotification.Message)
	}

	testing.ContextLog(ctx, "Completed fast pair pairing process")
	return &emptypb.Empty{}, nil
}

// CloseNotifications closes all open notifications.
func (bui *BtUIService) CloseNotifications(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	testing.ContextLog(ctx, "Closing all notifications on DUT")
	cr := bui.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tConn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}
	if err := ash.CloseNotifications(ctx, tConn); err != nil {
		return nil, errors.Wrap(err, "failed to close all notifications")
	}
	return &emptypb.Empty{}, nil
}

// ConfirmSavedDevicesState will attempt to confirm the state of Saved Devices
// on the Saved Devices subpage. The array of devices should be in the expected
// order. Fails if the list of Saved Devices doesn't match the one provided.
func (bui *BtUIService) ConfirmSavedDevicesState(ctx context.Context, request *pb.ConfirmSavedDevicesStateRequest) (_ *emptypb.Empty, retErr error) {
	cr := bui.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	app, err := ossettings.NavigateToBluetoothSavedDevicesSubpage(ctx, tconn, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to navigate to Bluetooth Saved Devices subpage")
	}
	defer app.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "confirm_saved_devices_state")

	testing.ContextLog(ctx, "Opened Bluetooth Saved Devices subpage")

	ui := uiauto.New(tconn)

	if len(request.DeviceNames) == 0 {
		// Enforce that no devices are saved on the Saved Devices subpage.
		if err := ui.WaitUntilExists(ossettings.SavedDevicesNoDevicesText)(ctx); err != nil {
			return nil, errors.Wrap(err, "found a non-empty Saved Devices subpage")
		}
	} else {
		// Enforce that the devices are displayed on the Saved Devices subpage in
		// the order that they were passed.
		for i, name := range request.DeviceNames {
			if err := ui.WaitUntilExists(nodewith.NameContaining(name).Ancestor(ossettings.SavedDeviceRows.Nth(i)))(ctx); err != nil {
				return nil, errors.Wrapf(err, "failed to find an expected saved device with name %s", name)
			}
		}
	}

	testing.ContextLogf(ctx, "Confirmed the state of the Saved Devices subpage with %d devices", len(request.DeviceNames))
	return &emptypb.Empty{}, nil
}

// RemoveAllSavedDevices will attempt to remove all the devices from the Saved Devices subpage.
func (bui *BtUIService) RemoveAllSavedDevices(ctx context.Context, request *emptypb.Empty) (_ *emptypb.Empty, retErr error) {
	cr := bui.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	app, err := ossettings.NavigateToBluetoothSavedDevicesSubpage(ctx, tconn, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to navigate to Bluetooth Saved Devices subpage")
	}
	defer app.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "remove_all_saved_devices")

	testing.ContextLog(ctx, "Opened Bluetooth Saved Devices subpage")

	ui := uiauto.New(tconn)

	// The Saved Devices page waits for a network call to resolve to update the
	// UI, so we poll for devices. If there are no saved devices, return early.
	opts := testing.PollOptions{Timeout: 5 * time.Second, Interval: 300 * time.Millisecond}
	if err := ui.WithPollOpts(opts).WaitUntilExists(ossettings.SavedDeviceRows.First())(ctx); err != nil {
		testing.ContextLog(ctx, "Saved Devices subpage contains no devices")
		return &emptypb.Empty{}, nil
	}

	devices, err := ui.NodesInfo(ctx, ossettings.SavedDeviceRows)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get info for Saved Device rows")
	}

	count := 0
	for count < len(devices) {
		device := ossettings.SavedDeviceRows.First()

		// We have to refresh node info after every removal since the name changes
		// depending on the device's order in the list.
		info, err := ui.Info(ctx, device)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get info for first Saved Device row")
		}

		// We use a different finder to confirm the device is gone after clicking
		// "remove" since First() can match to the next device in the list.
		if err := uiauto.Combine("Find the first saved device, remove it, and wait for it to disappear from the subpage.",
			ui.LeftClick(ossettings.SavedDeviceMoreActionsBtn.Ancestor(device)),
			ui.LeftClick(ossettings.SavedDeviceRemoveMenuItem),
			ui.LeftClick(ossettings.SavedDeviceConfirmRemovalBtn),
			ui.WaitUntilGone(ossettings.SavedDeviceRows.Name(info.Name)),
		)(ctx); err != nil {
			return nil, errors.Wrapf(err, "failed to remove a saved device, removed %d of %d devices", count, len(devices))
		}

		count++
	}

	testing.ContextLogf(ctx, "Removed %d of %d saved devices from Saved Devices subpage", count, len(devices))
	return &emptypb.Empty{}, nil
}

// PairDeviceWithQuickSettings will attempt to pair with the Bluetooth device
// described in the request using the Quick Settings UI.
//
// This method will ensure that any windows it had opened are closed before returning.
func (bui *BtUIService) PairDeviceWithQuickSettings(ctx context.Context, req *pb.PairDeviceWithQuickSettingsRequest) (_ *emptypb.Empty, retErr error) {
	cr := bui.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := quicksettings.NavigateToBluetoothDetailedView(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to navigate to the detailed Bluetooth view")
	}
	defer quicksettings.Hide(cleanupCtx, tconn)
	// Capturing the state before closing the QuickSettings.
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "quick_settings_bluetooth_detailed_view_ui_dump")

	ui := uiauto.New(tconn)
	// Pre-QsRevamp there are two buttons labeled "Pair new device" (the whole
	// HoverHighlightView and the plus icon), so click the first one.
	// Post-QsRevamp there is only one button labeled "Pair new device".
	if err := ui.LeftClickUntil(quicksettings.BluetoothDetailedViewPairNewDeviceButton.First(),
		ui.Exists(quicksettings.BluetoothPairNewDeviceDialog))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to open the pairing dialog")
	}

	defer func(ctx context.Context) {
		// Capturing the state before closing the Bluetooth pair new device dialog.
		faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(ctx, func() bool { return retErr != nil }, tconn, "bluetooth_pair_new_device_dialog_ui_dump")

		found, err := ui.IsNodeFound(ctx, quicksettings.BluetoothPairNewDeviceDialog)
		if err != nil {
			testing.ContextLog(ctx, "Failed to determine if the pairing dialog was still open")
			return
		}
		if !found {
			return
		}
		if err := ui.LeftClickUntil(nodewith.Name("Cancel").HasClass("cancel-button").Ancestor(quicksettings.BluetoothPairNewDeviceDialog),
			ui.Gone(quicksettings.BluetoothPairNewDeviceDialog))(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to close the pairing dialog")
		}
	}(cleanupCtx)

	// GoBigSleepLint: Include a short delay before attempting to pair with the Bluetooth
	// peripheral since attempting to pair immediately results in flaky behavior where
	// the device will disappear/reappear sporadically.
	testing.Sleep(ctx, 5*time.Second)

	deviceFinder := nodewith.NameContaining(req.AdvertisedName).Ancestor(quicksettings.BluetoothPairNewDeviceDialog).Role(role.Button).First()
	toastFinder := nodewith.NameContaining(req.AdvertisedName + " connected").Ancestor(nodewith.HasClass("ToastOverlay"))

	// The device we want to pair with may disappear and reappear in the pairing dialog.
	// To mitigate this flaky behavior we continue to click the device while waiting for
	// the "device connected" toast to appear for up to 1 minute.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := ui.Exists(toastFinder)(ctx); err == nil {
			return nil
		}
		if err := uiauto.Combine("find and click the device",
			ui.Exists(deviceFinder),
			ui.LeftClick(deviceFinder))(ctx); err != nil {
			return errors.Wrap(err, "failed to find and click the device")
		}
		return errors.New("failed to pair with the device, retrying")
	}, &testing.PollOptions{Timeout: time.Minute, Interval: 5 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to pair with the device")
	}

	return &emptypb.Empty{}, nil
}

// ForgetBluetoothDevice will attempt to navigate to the Device Details subpage
// for the device specified in the request, then click "Forget" to forget the
// device.
func (bui *BtUIService) ForgetBluetoothDevice(ctx context.Context, request *pb.ForgetBluetoothDeviceRequest) (_ *emptypb.Empty, retErr error) {
	cr := bui.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	app, err := ossettings.NavigateToBluetoothDeviceDetailsPage(ctx, tconn, request.DeviceName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to navigate to Bluetooth Device Details subpage for device %s", request.DeviceName)
	}
	defer app.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "forget_bt_device")

	testing.ContextLogf(ctx, "Opened Bluetooth Device Details subpage for device %s", request.DeviceName)

	ui := uiauto.New(tconn)

	if err := uiauto.Combine("Focus and click the Forget device buttons in the forget flow",
		ui.FocusAndWait(ossettings.BluetoothForgetDeviceButton),
		ui.LeftClick(ossettings.BluetoothForgetDeviceButton),
		ui.FocusAndWait(ossettings.BluetoothConfirmForgetButton),
		ui.LeftClick(ossettings.BluetoothConfirmForgetButton),
	)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to forget device from Bluetooth Device Details subpage")
	}

	testing.ContextLogf(ctx, "Successfully forgot device %s", request.DeviceName)
	return &emptypb.Empty{}, nil
}
