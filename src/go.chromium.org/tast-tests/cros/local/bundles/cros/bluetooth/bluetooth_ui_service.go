// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"golang.org/x/sync/errgroup"
	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/common"
	"go.chromium.org/tast-tests/cros/local/input"
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
	_, tConn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
	}

	// The Initial, Subsequent, and Retroactive scenarios have slightly different notifications and buttons.
	expectedNotificationID := ""
	buttonName := ""
	if request.Protocol == pb.FastPairProtocol_FAST_PAIR_PROTOCOL_INITIAL {
		expectedNotificationID = bluetooth.NotificationIDFastPairDiscoveryUser
		buttonName = "Connect"
	} else if request.Protocol == pb.FastPairProtocol_FAST_PAIR_PROTOCOL_SUBSEQUENT {
		expectedNotificationID = bluetooth.NotificationIDFastPairSubsequentPair
		buttonName = "Connect"
	} else if request.Protocol == pb.FastPairProtocol_FAST_PAIR_PROTOCOL_RETROACTIVE {
		expectedNotificationID = bluetooth.NotificationIDFastPairRetroactivePair
		buttonName = "Save"
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

	// Click the connect (initial/subsequent) or save (retroactive) button on the notification.
	testing.ContextLog(ctx, "Starting fast pair pairing process")
	connectBtn := nodewith.Role(role.Button).Name(buttonName)
	ac := uiauto.New(tConn)
	if err := ac.DoDefault(connectBtn)(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to click %q button on fast pair discovery notification", buttonName)
	}

	// For retroactive pair, there's no more expected notifications
	if request.Protocol == pb.FastPairProtocol_FAST_PAIR_PROTOCOL_RETROACTIVE {
		testing.ContextLog(ctx, "Completed fast pair save process")
		return &emptypb.Empty{}, nil
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
	_, tConn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
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
	cr, tconn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
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
	cr, tconn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
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
	_, tconn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
	}

	ui := uiauto.New(tconn)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := quicksettings.EnsureBluetoothPairingDialogIsOpened(tconn)(ctx); err != nil {
		return nil, err
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

	pairingTimeout := 2 * time.Minute
	ensureDeviceIsPairingCtx, cancel := context.WithTimeout(ctx, pairingTimeout)
	defer cancel()

	// The message that indicates Bluetooth device is connected will last for only 5 seconds whereas
	// the entire checking action can easily takes over 5 seconds and leads to a timing issue,
	// so ensure Bluetooth is pairing and check the message simultaneously is necessary.
	eg, ensureDeviceIsPairingCtx := errgroup.WithContext(ensureDeviceIsPairingCtx)
	eg.Go(func() error {
		var lastErr error
		for {
			select {
			case <-time.After(5 * time.Second):
				// The device we want to pair with may disappear and reappear in the pairing dialog.
				// To mitigate this flaky behavior we continue to click the device while waiting for
				// the "device connected" toast to appear for up to 2 minutes.
				lastErr = bui.ensureDeviceIsPairing(ensureDeviceIsPairingCtx, req.AdvertisedName, tconn)
			case <-ensureDeviceIsPairingCtx.Done():
				if lastErr != nil {
					return errors.Wrap(lastErr, "failed to ensure the Bluetooth device is pairing")
				}
				return nil
			}
		}
	})

	toastFinder := nodewith.NameContaining(req.AdvertisedName + " connected").Ancestor(nodewith.HasClass("ToastOverlay"))
	if err := ui.WithTimeout(pairingTimeout).WaitUntilExists(toastFinder)(ctx); err != nil {
		return nil, errors.Wrap(eg.Wait(), "failed to wait for connected message to appear")
	}

	return &emptypb.Empty{}, nil
}

// ensureDeviceIsPairing ensure the device in the Bluetooth Pair New Device Dialog is pairing.
func (bui *BtUIService) ensureDeviceIsPairing(ctx context.Context, deviceName string, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	// pairingFinder is only shown after the specified Bluetooth device is selected
	// in the Bluetooth Pair New Device dialog, and pairing is successfully started.
	pairingFinder := nodewith.NameStartingWith(fmt.Sprintf("Pairing to %s", deviceName)).HasClass("text-row").Ancestor(quicksettings.BluetoothPairNewDeviceDialog)
	if deviceIsPairing, err := ui.IsNodeFound(ctx, pairingFinder); err != nil {
		return errors.Wrap(err, "failed to verify whether the device is pairing")
	} else if deviceIsPairing {
		// No action required if the Bluetooth device is already pairing.
		return nil
	}

	deviceFinder := nodewith.NameContaining(deviceName).Ancestor(quicksettings.BluetoothPairNewDeviceDialog).Role(role.Button).First()
	return uiauto.Combine("attempt to pair with the device",
		// Re-open dialog if it's closed.
		quicksettings.EnsureBluetoothPairingDialogIsOpened(tconn),
		ui.LeftClick(deviceFinder),
	)(ctx)
}

// ForgetBluetoothDevice will attempt to navigate to the Device Details subpage
// for the device specified in the request, then click "Forget" to forget the
// device.
func (bui *BtUIService) ForgetBluetoothDevice(ctx context.Context, request *pb.ForgetBluetoothDeviceRequest) (_ *emptypb.Empty, retErr error) {
	_, tconn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
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

// CollectDeviceList will attempt to collect and list all available Bluetooth devices
// in the Bluetooth page.
func (bui *BtUIService) CollectDeviceList(ctx context.Context, _ *emptypb.Empty) (_ *pb.CollectDeviceListResponse, retErr error) {
	_, tconn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	settings, err := ossettings.LaunchAtPage(ctx, tconn, ossettings.Bluetooth)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch OS accessibility settings")
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "bt_devices_page")

	// Regular expression for finding a device entry.
	//
	// These are examples of the target label:
	// 	Device 1 of 3 named KEYBD_REF. Device is connected. Device type is unknown. Device has 99% battery.
	// 	Device 2 of 3 named MOUSE_REF. Device is not connected. Device type is unknown.
	// 	Device 3 of 3 named RenamedBT. Device is not connected. Device is a mouse.
	r := regexp.MustCompile(`^Device \d+ of \d+ named (.*)\. Device is (connected|not connected)\. Device (type is|is a) \w+\.( Device has (\d+)% battery\.)?$`)
	bluetoothDeviceItem := nodewith.NameRegex(r).HasClass("list-item")

	if err := settings.WaitUntilExists(bluetoothDeviceItem.First())(ctx); err != nil {
		if !nodewith.IsNodeNotFoundErr(err) {
			return nil, errors.Wrap(err, "failed to wait for Bluetooth device item")
		}
		bui.s.Log("No Bluetooth devices were found in the Bluetooth page in OS-Settings")
	}

	infos, err := settings.NodesInfo(ctx, bluetoothDeviceItem)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get nodes information")
	}

	devices := make([]*pb.Device, 0, len(infos))
	for _, info := range infos {
		ss := r.FindStringSubmatch(info.Name)
		// Expecting 6 sub-matches which are
		// 	0: entire match
		// 	1: device name
		// 	2: connected state
		// 	3: device type information
		// 	4: battery information
		// 	5: battery level value
		if ss == nil || len(ss) != 6 {
			return nil, errors.Errorf("failed to extract string sub match by %q from %q, sub matches: %d", r.String(), info.Name, len(ss))
		}
		device := &pb.Device{
			Name:        ss[1],
			IsConnected: ss[2] == "connected",
		}

		// Battery level may not be presented, it's only available for connected BLE devices.
		if ss[4] != "" {
			device.Battery = &pb.Battery{ValueInPercentage: ss[4]}
			if !device.IsConnected {
				return nil, errors.Errorf("unexpected device info %+v, battery information should be available iff the BLE device is connected", device)
			}
		}

		devices = append(devices, device)
	}

	return &pb.CollectDeviceListResponse{Devices: devices}, nil
}

// BluetoothDeviceDetail will attempt to navigate to the Device Detail subpage for the device
// specified in the request, then retrieves the information of this particular device.
func (bui *BtUIService) BluetoothDeviceDetail(ctx context.Context, req *pb.BluetoothDeviceDetailRequest) (_ *pb.BluetoothDeviceDetailResponse, retErr error) {
	_, tconn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	settings, err := ossettings.NavigateToBluetoothDeviceDetailsPage(ctx, tconn, req.Name)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to navigate to Bluetooth Device Details subpage for device %s", req.Name)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "bt_device_detail_page")

	// Regular expression for finding the device heading.
	//
	// This is an example of the target label:
	// 	Connected to KEYBD_REF
	var deviceHeadingRegexp *regexp.Regexp

	// Match the connection state if client specifies an option to match before this RPC can respond.
	if req.MatchOption != nil {
		switch req.GetMatchOption() {
		case pb.BluetoothDeviceDetailRequest_MATCH_OPTION_CONNECTED:
			deviceHeadingRegexp = regexp.MustCompile(fmt.Sprintf(`^(Connected to) %s$`, req.Name))
		case pb.BluetoothDeviceDetailRequest_MATCH_OPTION_DISCONNECTED:
			deviceHeadingRegexp = regexp.MustCompile(fmt.Sprintf(`^(Disconnected from) %s$`, req.Name))
		default:
			// Match both as other options are irrelevant regarding the connection state.
			deviceHeadingRegexp = regexp.MustCompile(fmt.Sprintf(`^(Connected to|Disconnected from) %s$`, req.Name))
		}
	}

	deviceHeading := nodewith.NameRegex(deviceHeadingRegexp)
	// Waiting for the heading of the device detail page before proceed on extract device detail.
	if err := settings.WaitUntilExists(deviceHeading)(ctx); err != nil {
		return nil, errors.Wrapf(err, "failed to wait until the connection status of device of Bluetooth device %s detail page as expected", req.Name)
	}

	// Extract device connected state from the heading.
	info, err := settings.Info(ctx, deviceHeading)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get node information")
	}
	ss := deviceHeadingRegexp.FindStringSubmatch(info.Name)
	// Expecting 2 sub-matches which are
	// 	0: entire match
	// 	1: connected state
	if ss == nil || len(ss) != 2 {
		return nil, errors.Errorf("failed to extract string sub match by %q from %q, sub matches: %d", deviceHeadingRegexp.String(), info.Name, len(ss))
	}
	device := &pb.Device{
		Name:        req.Name,
		IsConnected: ss[1] == "Connected to",
	}

	// Regular expression for finding the device heading.
	//
	// This is an example of the target label:
	// 	Battery level 99%
	batteryLevelRegexp := regexp.MustCompile(`^Battery level (\d+)%$`)
	label := nodewith.NameRegex(batteryLevelRegexp)

	// Match the battery info is reported or not if client specifies an option to match before this RPC can respond.
	if req.MatchOption != nil {
		switch req.GetMatchOption() {
		case pb.BluetoothDeviceDetailRequest_MATCH_OPTION_NOT_REPORT_BATTERY:
			if err := settings.EnsureGoneFor(label, 5*time.Second)(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to wait for the battery information to be gone")
			}
			return &pb.BluetoothDeviceDetailResponse{Device: device}, nil
		case pb.BluetoothDeviceDetailRequest_MATCH_OPTION_REPORT_BATTERY:
			if err := settings.WaitUntilExists(label)(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to wait for the battery information to be reported")
			}
		default:
			// Report the battery info if it is present as other options are irrelevant
			// regarding the battery info is reported or not.
		}
	}

	// Extract battery level if it's present.
	found, err := settings.IsNodeFound(ctx, label)
	if err != nil {
		return nil, errors.Wrap(err, "failed to find the battery level node")
	}
	if found {
		batteryInfo, err := settings.Info(ctx, label)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get node information")
		}
		ss := batteryLevelRegexp.FindStringSubmatch(batteryInfo.Name)
		// Expecting 2 sub-matches which are
		// 	0: entire match
		// 	1: battery level value
		if ss == nil || len(ss) != 2 {
			return nil, errors.Errorf("failed to extract string sub match by %q from %q, sub matches: %d", batteryLevelRegexp.String(), batteryInfo.Name, len(ss))
		}
		if ss[1] != "" {
			device.Battery = &pb.Battery{ValueInPercentage: ss[1]}
		}
	}

	return &pb.BluetoothDeviceDetailResponse{Device: device}, nil
}

// RenameBluetoothDevice renames the Bluetooth device within OS Settings Bluetooth device detail page.
func (bui *BtUIService) RenameBluetoothDevice(ctx context.Context, req *pb.RenameBluetoothDeviceRequest) (_ *emptypb.Empty, retErr error) {
	if req.GetCustomName() == "" {
		return &emptypb.Empty{}, errors.New("invalid custom name")
	}

	_, tconn, err := bui.crAndTestAPIConn(ctx)
	if err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to obtain the Chrome instance and Test API connection")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	deviceName := req.Device.GetName()
	settings, err := ossettings.NavigateToBluetoothDeviceDetailsPage(ctx, tconn, deviceName)
	if err != nil {
		return &emptypb.Empty{}, errors.Wrapf(err, "failed to navigate to Bluetooth device %q detail page", deviceName)
	}
	defer settings.Close(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotWithTestAPIOnErrorToContextOutDir(cleanupCtx, func() bool { return retErr != nil }, tconn, "rename_bluetooth_device")

	kb, err := input.Keyboard(ctx)
	if err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to get a keyboard event writer")
	}
	defer kb.Close(cleanupCtx)

	deviceNameDialog := nodewith.Name("Device name").Role(role.Dialog)
	if err := settings.LeftClickUntil(
		nodewith.Name(fmt.Sprintf("Change device name for %s", deviceName)).Role(role.Button),
		settings.WithTimeout(3*time.Second).WaitUntilExists(deviceNameDialog),
	)(ctx); err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to launch the change Bluetooth device name dialog")
	}
	defer func(ctx context.Context) {
		// Clean up the dialog when the error occurs.
		if retErr != nil {
			kb.TypeKey(ctx, input.KEY_ESC)
		}
	}(cleanupCtx)

	textField := nodewith.Name("Device name").Role(role.TextField).Ancestor(deviceNameDialog)
	if err := settings.EnsureFocused(textField)(ctx); err != nil {
		return &emptypb.Empty{}, errors.Wrap(err, "failed to focus the textfield of the Bluetooth device name dialog")
	}

	selectAllAndType := func(str string) uiauto.Action {
		return uiauto.Combine("enter custom name",
			kb.AccelAction("Ctrl+A"),
			kb.TypeAction(str),
		)
	}

	ui := uiauto.New(tconn)
	return &emptypb.Empty{}, uiauto.Combine("rename the Bluetooth device",
		// Keep retrying typing until the text matches the expected input as ChromeOS might reset it during typing action (b/329191341).
		ui.RetryUntil(
			selectAllAndType(req.GetCustomName()),
			settings.WithTimeout(3*time.Second).WaitUntilExists(nodewith.Name(req.GetCustomName()).Role(role.InlineTextBox).Ancestor(textField)),
		),
		settings.LeftClick(nodewith.Name("Done").Role(role.Button).Ancestor(deviceNameDialog)),
		settings.WaitUntilGone(deviceNameDialog),
		settings.WaitUntilExists(nodewith.Name(req.GetCustomName()).Role(role.Heading).First()),
	)(ctx)
}

// crAndTestAPIConn obtains/returns the (*chrome.Chrome) instance and the TestAPI connection.
// It also asserts the (*chrome.Chrome) instance and throws an error if (*chrome.Chrome) instance isn't available.
func (bui *BtUIService) crAndTestAPIConn(ctx context.Context) (*chrome.Chrome, *chrome.TestConn, error) {
	// Leverage the UseTconn to do:
	// 1. Obtain the TestAPI connection, this allows a more flexible usage as
	//    the TestAPI when Chrome is no logged in is different than usual.
	// 2. Assert if Chrome is instantiated, provides a safer resource accessing.
	tconn, err := common.UseTconn(ctx, bui.sharedObject, func(tconn *chrome.TestConn) (*chrome.TestConn, error) {
		return tconn, nil
	})
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to obtain Test API connection")
	}

	return bui.sharedObject.Chrome, tconn, nil
}
