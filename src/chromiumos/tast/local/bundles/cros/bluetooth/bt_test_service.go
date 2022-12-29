// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"sort"
	"strings"
	"time"

	"google.golang.org/grpc"
	"google.golang.org/protobuf/types/known/emptypb"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/bluetooth"
	"chromiumos/tast/local/bluetooth/bluez"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/ossettings"
	"chromiumos/tast/local/chrome/uiauto/quicksettings"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/common"
	pb "chromiumos/tast/services/cros/bluetooth"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddService(&testing.Service{
		Register: func(srv *grpc.Server, s *testing.ServiceState) {
			pb.RegisterBTTestServiceServer(srv, &BTTestService{
				s:            s,
				sharedObject: common.SharedObjectsForServiceSingleton,
			})
		},
	})
}

// BTTestService implements tast.cros.bluetooth.BTTestService.
type BTTestService struct {
	s            *testing.ServiceState
	bluezAdapter *bluez.Adapter
	sharedObject *common.SharedObjectsForService
}

// EnableBluetoothAdapter powers on the bluetooth adapter and waits for it to
// be enabled.
func (bts *BTTestService) EnableBluetoothAdapter(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	testing.ContextLog(ctx, "Enabling bluetooth adapter")
	if err := bluez.Enable(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to enable bluetooth adapter")
	}
	if err := bluez.PollForBTEnabled(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for bluetooth adapter to be enabled")
	}
	if adapters, err := bluez.Adapters(ctx); err == nil {
		bts.bluezAdapter = adapters[0]
	} else {
		return nil, errors.Wrap(err, "failed to get bluetooth adapters")
	}
	return &emptypb.Empty{}, nil
}

// DisableBluetoothAdapter powers off the bluetooth adapter.
func (bts *BTTestService) DisableBluetoothAdapter(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	testing.ContextLog(ctx, "Disabling bluetooth adapter")
	if err := bluez.Disable(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to disable bluetooth adapter")
	}
	if err := bluez.PollForBTDisabled(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for bluetooth adapter to be disabled")
	}
	bts.bluezAdapter = nil
	return &emptypb.Empty{}, nil
}

// DisconnectAllDevices disconnects all connected bluetooth devices.
func (bts *BTTestService) DisconnectAllDevices(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	testing.ContextLog(ctx, "Disconnecting all bluetooth devices from DUT")
	if err := bluez.DisconnectAllDevices(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to disconnect all bluetooth devices")
	}
	return &emptypb.Empty{}, nil
}

// DiscoverDevice confirms that the DUT can discover the provided bluetooth
// device. Fails if the device is not found or if the discovered matching
// device's attributes do not match those provided.
func (bts *BTTestService) DiscoverDevice(ctx context.Context, request *pb.DiscoverDeviceRequest) (*emptypb.Empty, error) {
	if request.Device == nil || request.Device.AdvertisedName == "" ||
		request.Device.MacAddress == "" {
		return nil, errors.New("incomplete DiscoverDevice request")
	}
	if _, err := bts.discoverDeviceByAddress(ctx, request.Device.MacAddress, request.Device.AdvertisedName); err != nil {
		return nil, err
	}
	return &emptypb.Empty{}, nil
}

func (bts *BTTestService) discoverDeviceByAddress(ctx context.Context, targetDeviceAddress, expectedDeviceName string) (*bluez.Device, error) {
	if bts.bluezAdapter == nil {
		return nil, errors.New("bluetooth adapter not initialized, call EnableBluetoothAdapter first")
	}

	// Attempt to discover device with matching address.
	if err := bts.bluezAdapter.StartDiscovery(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to start discovery")
	}
	var btDevice *bluez.Device
	testing.ContextLogf(ctx, "Polling for discovery of device with address %q", targetDeviceAddress)
	if pollErr := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		btDevice, err = bluez.DeviceByAddress(ctx, targetDeviceAddress)
		if err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  60 * time.Second,
		Interval: 250 * time.Millisecond,
	}); pollErr != nil {
		baseErr := errors.Wrapf(pollErr, "timeout waiting for discover device with address %q", targetDeviceAddress)
		// Failed to find the specific device. Attempt to include a list of devices that were found in the error message.
		devices, err := bts.discoverDevices(ctx)
		if err != nil {
			return nil, baseErr
		}
		devicesStr := make([]string, len(devices))
		for i, device := range devices {
			devicesStr[i] = device.String()
		}
		sort.Strings(devicesStr)
		return nil, errors.Wrapf(pollErr,
			"timeout waiting for discover device with address %q. Found %d other devices (%v)",
			targetDeviceAddress,
			len(devices),
			strings.Join(devicesStr, ", "))
	}
	if err := bts.bluezAdapter.StopDiscovery(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to start discovery")
	}

	// Validate discovered device is intended device.
	btDeviceAddr, err := btDevice.Address(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get address of discovered device")
	}
	if btDeviceAddr != targetDeviceAddress {
		return nil, errors.Errorf("discovered device with address %q does not match expected address %q", btDeviceAddr, targetDeviceAddress)
	}
	btDeviceName, err := btDevice.Name(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get name of discovered device")
	}
	if btDeviceName != expectedDeviceName {
		return nil, errors.Errorf("discovered device with address %q and name %q does not match expected name %q", btDeviceAddr, btDeviceName, expectedDeviceName)
	}

	testing.ContextLogf(ctx, "Discovered device with address %q and name %q at dbus path %q", btDeviceAddr, btDeviceName, btDevice.Path())
	return btDevice, nil
}

func (bts *BTTestService) discoverDevices(ctx context.Context) ([]*pb.Device, error) {
	foundDevices, err := bluez.Devices(ctx)
	if err != nil {
		return nil, err
	}
	var devices = make([]*pb.Device, len(foundDevices))
	for i, foundDevice := range foundDevices {
		name, err := foundDevice.Name(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get name of found device")
		}
		macAddress, err := foundDevice.Address(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get mac address of found device")
		}
		devices[i] = &pb.Device{
			AdvertisedName: name,
			MacAddress:     macAddress,
		}
	}
	return devices, nil
}

// RemoveAllDevices removes all bluetooth devices.
func (bts *BTTestService) RemoveAllDevices(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	testing.ContextLog(ctx, "Removing all bluetooth devices from DUT")
	devices, err := bluez.Devices(ctx)
	if err != nil {
		return nil, err
	}
	for _, device := range devices {
		if err := bts.bluezAdapter.RemoveDevice(ctx, device.Path()); err != nil {
			return nil, errors.Wrapf(err, "failed to remove device at dbus path %q", device.Path())
		}
	}
	return &emptypb.Empty{}, nil
}

// PairAndConnectDevice pairs and connects to the specified Device.
func (bts *BTTestService) PairAndConnectDevice(ctx context.Context, request *pb.PairAndConnectDeviceRequest) (*emptypb.Empty, error) {
	if request.Device == nil || request.Device.AdvertisedName == "" ||
		request.Device.MacAddress == "" {
		return nil, errors.New("incomplete PairAndConnectDevice request")
	}

	// Attempt to discover device.
	btDevice, err := bts.discoverDeviceByAddress(ctx, request.Device.MacAddress, request.Device.AdvertisedName)
	if err != nil {
		return nil, err
	}

	// Ensure bluetooth device is paired.
	// Unpairs and pairs again if ForceNewPair is true and device was already
	// paired.
	isPaired, err := btDevice.Paired(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check if device is paired")
	}
	if isPaired && request.ForceNewPair {
		testing.ContextLogf(ctx, "Removing and rediscovering already paired device at dbus path %q", btDevice.Path())
		if err := bts.bluezAdapter.RemoveDevice(ctx, btDevice.Path()); err != nil {
			return nil, errors.Wrapf(err, "failed to remove paired device at dbus path %q", btDevice.Path())
		}
		btDevice, err = bts.discoverDeviceByAddress(ctx, request.Device.MacAddress, request.Device.AdvertisedName)
		if err != nil {
			return nil, errors.Wrap(err, "failed rediscover device after removal")
		}
		isPaired = false
	}
	if isPaired {
		testing.ContextLogf(ctx, "Skipping pairing step as device at dbus path %q is already paired", btDevice.Path())
	} else {
		// Pair the device.
		if request.Device.HasPinCode {
			// Prepare to handle pin authorization, if requested by the btpeer.
			testing.ContextLogf(ctx, "Preparing to authorize pairing with pin code %q", request.Device.PinCode)
			agentManagers, err := bluez.AgentManagers(ctx)
			if err != nil {
				return nil, errors.Wrap(err, "failed to get AgentManager")
			}
			if len(agentManagers) == 0 {
				return nil, errors.New("no AgentManager found")
			}
			agentManager := agentManagers[0]
			testing.ContextLogf(ctx, "Using authentication AgentManager at dbus path %q", agentManager.DBusObject().ObjectPath())
			agent, err := bluez.NewAgent(ctx, "")
			if err != nil {
				return nil, errors.Wrap(err, "failed to create new authentication Agent")
			}
			if err := agent.ExportAgentDelegate(bluez.NewSimplePinAgentDelegate(ctx, request.Device.PinCode)); err != nil {
				return nil, errors.Wrap(err, "failed to export AgentDelegate")
			}
			if err := agentManager.RegisterAgent(ctx, agent.DBusObject().ObjectPath(), "KeyboardDisplay"); err != nil {
				return nil, errors.Wrapf(err, "failed to register Agent %q with AgentManager %q", agent.DBusObject().ObjectPath(), agentManager.DBusObject().ObjectPath())
			}
			if err := agentManager.RequestDefaultAgent(ctx, agent.DBusObject().ObjectPath()); err != nil {
				return nil, errors.Wrapf(err, "failed to register Agent %q with AgentManager %q as default", agent.DBusObject().ObjectPath(), agentManager.DBusObject().ObjectPath())
			}
			testing.ContextLogf(ctx, "Using new authentication Agent to provide pin code %q at dbus path %q", request.Device.PinCode, agent.DBusObject().ObjectPath())

			// Attempt paring.
			testing.ContextLogf(ctx, "Pairing device at dbus path %q", btDevice.Path())
			if err := btDevice.Pair(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to pair bluetooth device")
			}

			// Cleanup pin authentication handling.
			testing.ContextLogf(ctx, "Removing authentication Agent at dbus path %q", agent.DBusObject().ObjectPath())
			if err := agent.ClearExportedAgentDelegate(); err != nil {
				return nil, errors.Wrapf(err, "failed to clear exported AgentDelegate for Agent at %q", agent.DBusObject().ObjectPath())
			}
			if err := agentManager.UnregisterAgent(ctx, agent.DBusObject().ObjectPath()); err != nil {
				return nil, errors.Wrapf(err, "failed to unregister Agent %q with AgentManager %q", agent.DBusObject().ObjectPath(), agentManager.DBusObject().ObjectPath())
			}
		} else {
			testing.ContextLogf(ctx, "Pairing device at dbus path %q", btDevice.Path())
			if err := btDevice.Pair(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to pair bluetooth device")
			}
		}
	}

	// Get connected status of BT device and connect if not already connected.
	testing.ContextLogf(ctx, "Connecting to device at dbus path %q", btDevice.Path())
	isConnected, err := btDevice.Connected(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device connected status")
	}
	if isConnected {
		if request.ForceNewConnect {
			testing.ContextLogf(ctx, "Disconnecting already connected device at dbus path %q", btDevice.Path())
			if err := btDevice.Disconnect(ctx); err != nil {
				return nil, errors.Wrap(err, "failed to disconnect bluetooth device")
			}
			isConnected = false
		} else {
			testing.ContextLogf(ctx, "Skipping connect step as device at dbus path %q is already connected", btDevice.Path())
		}
	}
	if !isConnected {
		testing.ContextLogf(ctx, "Connecting to device at dbus path %q", btDevice.Path())
		if err := btDevice.Connect(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to connect bluetooth device")
		}
	}

	return &emptypb.Empty{}, nil
}

// DeviceStatus checks for a given device and returns whether it has been
// discovered by and paired to the DUT.
func (bts *BTTestService) DeviceStatus(ctx context.Context, request *pb.DeviceStatusRequest) (*pb.DeviceStatusResponse, error) {
	if request.Device == nil || request.Device.MacAddress == "" || request.Device.AdvertisedName == "" {
		return nil, errors.New("incomplete DeviceStatus request")
	}

	// Find the first device that matches the address.
	devices, err := bluez.Devices(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device list")
	}
	var matchingDevice *bluez.Device
	for _, d := range devices {
		ad, err := d.Address(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get device address property")
		}
		if ad == request.Device.MacAddress {
			matchingDevice = d
			break
		}
	}
	if matchingDevice == nil {
		// No matching device found.
		return &pb.DeviceStatusResponse{
			IsDiscovered: false,
			IsPaired:     false,
		}, nil
	}

	// Validate the name of the device matches too.
	deviceName, err := matchingDevice.Name(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get device name property")
	}
	if deviceName != request.Device.AdvertisedName {
		return nil, errors.Errorf("found a matching device with address %q, but its name, %q, does not match the expected name %q", request.Device.MacAddress, deviceName, request.Device.AdvertisedName)
	}

	// Check pairing status.
	isPaired, err := matchingDevice.Paired(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to check if device is paired")
	}

	return &pb.DeviceStatusResponse{
		IsDiscovered: true,
		IsPaired:     isPaired,
	}, nil
}

// PairWithFastPairNotification will attempt to pair a fast pair device with
// the fast pair notification. The |request| contains a Protocol which must be
// either Initial or Subsequent for this function.
func (bts *BTTestService) PairWithFastPairNotification(ctx context.Context, request *pb.PairWithFastPairNotificationRequest) (*emptypb.Empty, error) {
	cr := bts.sharedObject.Chrome
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
		return nil, errors.New("Wrong protocol requested; only initial and subsequent scenarios are supported")
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
func (bts *BTTestService) CloseNotifications(ctx context.Context, empty *emptypb.Empty) (*emptypb.Empty, error) {
	testing.ContextLog(ctx, "Closing all notifications on DUT")
	cr := bts.sharedObject.Chrome
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

// ConfirmSavedDevicesState will attempt to confirm the state of Saved Devices on the Saved
// Devices subpage. The array of devices should be in the expected order. Fails if the list
// of Saved Devices doesn't match the one provided.
func (bts *BTTestService) ConfirmSavedDevicesState(ctx context.Context, request *pb.ConfirmSavedDevicesStateRequest) (*emptypb.Empty, error) {
	cr := bts.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	app, err := ossettings.NavigateToBluetoothSavedDevicesSubpage(ctx, tconn, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to navigate to Bluetooth Saved Devices subpage")
	}

	defer app.Close(ctx)
	testing.ContextLog(ctx, "Opened Bluetooth Saved Devices subpage")

	ui := uiauto.New(tconn)

	if len(request.DeviceNames) == 0 {
		// Enforce that no devices are saved on the Saved Devices subpage.
		if err := ui.WaitUntilExists(ossettings.SavedDevicesNoDevicesText)(ctx); err != nil {
			return nil, errors.Wrap(err, "found a non-empty Saved Devices subpage")
		}
	} else {
		// Enforce that the devices are displayed on the Saved Devices subpage in the
		// order that they were passed.
		for i, name := range request.DeviceNames {
			if err := ui.WaitUntilExists(nodewith.NameContaining(name).Ancestor(ossettings.SavedDeviceRows.Nth(i)))(ctx); err != nil {
				return nil, errors.Wrapf(err, "failed to find an expected saved device with name %s", name)
			}
		}
	}

	testing.ContextLogf(ctx, "Confirmed the state of the Saved Devices subpage with %d devices", len(request.DeviceNames))
	return &emptypb.Empty{}, nil
}

// RemoveAllSavedDevices will attempt to remove all of the devices from the Saved Devices subpage.
func (bts *BTTestService) RemoveAllSavedDevices(ctx context.Context, request *emptypb.Empty) (*emptypb.Empty, error) {
	cr := bts.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	app, err := ossettings.NavigateToBluetoothSavedDevicesSubpage(ctx, tconn, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to navigate to Bluetooth Saved Devices subpage")
	}

	defer app.Close(ctx)
	testing.ContextLog(ctx, "Opened Bluetooth Saved Devices subpage")

	ui := uiauto.New(tconn)

	// The Saved Devices page waits for a network call to resolve to update the UI,
	// so we poll for devices. If there are no saved devices, return early.
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

// PairDeviceWithQuickSettings will attempt to pair with the Bluetooth device described in the request using the Quick Settings UI.
// This method will ensure that any windows it had opened are closed before returning.
func (bts *BTTestService) PairDeviceWithQuickSettings(ctx context.Context, req *pb.PairDeviceWithQuickSettingsRequest) (*emptypb.Empty, error) {
	cr := bts.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	if err := quicksettings.NavigateToBluetoothDetailedView(ctx, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to navigate to the detailed Bluetooth view")
	}

	defer quicksettings.Hide(ctx, tconn)

	ui := uiauto.New(tconn)
	if err := ui.LeftClickUntil(quicksettings.BluetoothDetailedViewPairNewDeviceButton,
		ui.Exists(quicksettings.BluetoothPairNewDeviceDialog))(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to open the pairing dialog")
	}

	defer func() {
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
	}()

	// TODO(b/261885619): Investigate why this delay before selecting the device improves how
	// likely we are to successfully pair with the device.
	testing.Sleep(ctx, 5*time.Second)

	deviceFinder := nodewith.NameContaining(req.AdvertisedName).Ancestor(quicksettings.BluetoothPairNewDeviceDialog).Role(role.Button)
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
	}, &testing.PollOptions{Timeout: time.Minute, Interval: time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to pair with the device")
	}

	return &emptypb.Empty{}, nil
}

// ForgetBluetoothDevice will attempt to navigate to the Device Details subpage for the device specified in the request, then
// click "Forget" to forget the device.
func (bts *BTTestService) ForgetBluetoothDevice(ctx context.Context, request *pb.ForgetBluetoothDeviceRequest) (*emptypb.Empty, error) {
	cr := bts.sharedObject.Chrome
	if cr == nil {
		return nil, errors.New("Chrome has not been started")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get sign-in profile test API conn")
	}

	app, err := ossettings.NavigateToBluetoothDeviceDetailsPage(ctx, tconn, request.DeviceName)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to navigate to Bluetooth Device Details subpage for device %s", request.DeviceName)
	}
	defer app.Close(ctx)

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
