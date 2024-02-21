// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/common"
	"go.chromium.org/tast-tests/cros/local/bluetooth/floss"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const flossManagerDaemonJob = "btmanagerd"

// GetFlossEnabled returns the enabled state of floss.
func GetFlossEnabled(ctx context.Context) (bool, error) {
	manager, err := floss.DefaultManagerClient(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to get default floss manager client")
	}
	isFlossEnabled, err := manager.GetFlossEnabled(ctx)
	if err != nil {
		return false, errors.Wrap(err, "failed to call GetFlossEnabled with floss manager client")
	}
	return isFlossEnabled, nil
}

// SetFlossEnabled will attempt to start the floss manager daemon and set it as
// the provided enabled state.
//
// If enabled is false and the daemon fails to start, no error is returned as
// it assumes that floss is not supported on the device.
func SetFlossEnabled(ctx context.Context, enabled bool) error {
	testing.ContextLogf(ctx, "Setting floss enabled to %t", enabled)

	// Check if floss is supported on the DUT and exit early if not.
	flossSupported := upstart.JobExists(ctx, flossManagerDaemonJob)
	if !flossSupported {
		if enabled {
			return errors.Errorf("DUT does not support floss (%q job does not exist)", flossManagerDaemonJob)
		}
		return nil
	}

	// Ensure floss manager daemon is running.
	testing.ContextLogf(ctx, "Checking for floss manager daemon job %q", flossManagerDaemonJob)
	if err := upstart.CheckJob(ctx, flossManagerDaemonJob); err != nil {
		testing.ContextLogf(ctx, "Floss manager daemon job %q not running, restarting job and waiting for floss D-Bus service %q to come up", flossManagerDaemonJob, floss.DBusFlossManagerService)
		if err := upstart.RestartJobAndWaitForDbusService(ctx, flossManagerDaemonJob, floss.DBusFlossManagerService); err != nil {
			return errors.Wrapf(err, "failed to start floss manager daemon when trying to set floss enabled to %t", enabled)
		}
	}

	testing.ContextLog(ctx, "Retrieving current floss enabled state with floss manager client")
	manager, err := floss.DefaultManagerClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get default floss manager client")
	}
	isEnabled, err := manager.GetFlossEnabled(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to call GetFlossEnabled with floss manager client")
	}
	if isEnabled != enabled {
		testing.ContextLogf(ctx, "Setting floss enabled to %t with floss manager client", enabled)
		if err := manager.SetFlossEnabled(ctx, enabled); err != nil {
			return errors.Wrapf(err, "failed to call SetFlossEnabled(%t) with floss manager client", enabled)
		}
	} else {
		testing.ContextLogf(ctx, "Floss enabled already set to %t, no need to update it with floss manager client", enabled)
	}

	// Update CRAS with new floss state.
	testing.ContextLog(ctx, "Updating CRAS with floss enabled state")
	crasClient, err := audio.NewCras(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize CRAS client")
	}
	if err := crasClient.SetFlossEnabled(ctx, enabled); err != nil {
		return errors.Wrapf(err, "failed to call SetFlossEnabled(%t) with CRAS client", enabled)
	}

	testing.ContextLogf(ctx, "Successfully set floss enabled to %t", enabled)
	return nil
}

// BluetoothFlossFacade is an implementation of the BluetoothFacade interface
// for the floss bluetooth stack.
type BluetoothFlossFacade struct {
	flossEnabled      bool
	adapterEnabled    bool
	adapterHCI        int
	managerClient     *floss.ManagerClient
	adapterClient     *floss.AdapterClient
	discoveryObserver *discoveryObserver
	mockIsPairable    bool
}

// NewBluetoothFlossFacade initializes a new BluetoothFlossFacade and enables
// the floss bluetooth stack.
func NewBluetoothFlossFacade(ctx context.Context) (*BluetoothFlossFacade, error) {
	b := &BluetoothFlossFacade{}
	if err := SetFlossEnabled(ctx, true); err != nil {
		return nil, errors.Wrap(err, "failed to set floss as enabled")
	}
	if err := b.initializeManagerClient(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

// StackType returns the BluetoothStackType the facade uses.
func (b *BluetoothFlossFacade) StackType() common.BluetoothStackType {
	return common.BluetoothStackTypeFloss
}

// IsAlive checks if the bluetooth is alive by checking relative job/service.
func (b *BluetoothFlossFacade) IsAlive(ctx context.Context) (bool, error) {
	if !upstart.JobExists(ctx, flossManagerDaemonJob) {
		return false, nil
	}
	return upstart.IsServiceAvailable(ctx, floss.DBusFlossManagerService)
}

// Enable will turn on the bluetooth daemons and power on adapter.
func (b *BluetoothFlossFacade) Enable(ctx context.Context) error {
	b.flossEnabled = false
	if err := SetFlossEnabled(ctx, true); err != nil {
		return errors.Wrap(err, "failed to set floss as enabled")
	}
	if err := b.initializeManagerClient(ctx); err != nil {
		return err
	}
	if err := b.SetPowered(ctx, true); err != nil {
		testing.ContextLog(ctx, "Failed to enable floss adapter on first attempt after enabling floss: ", err)
		if err := b.SetPowered(ctx, true); err != nil {
			return errors.Wrap(err, "failed to enable floss adapter on second attempt after enabling floss")
		}
	}
	b.flossEnabled = true
	return nil
}

func (b *BluetoothFlossFacade) initializeManagerClient(ctx context.Context) error {
	var err error
	b.managerClient, err = floss.DefaultManagerClient(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize floss manager client")
	}
	return nil
}

func (b *BluetoothFlossFacade) initializeAdapterSpecificDBusClients(ctx context.Context) error {
	if err := b.assertManagerClientExists(); err != nil {
		return err
	}
	var err error
	b.adapterHCI = b.managerClient.DefaultAdapterHCI()
	b.adapterClient, err = floss.NewAdapterClient(ctx, b.adapterHCI)
	if err != nil {
		return err
	}
	return nil
}

// Disable will turn off the bluetooth daemons and power off adapter.
func (b *BluetoothFlossFacade) Disable(ctx context.Context) error {
	// Clear client-dependent objects.
	if b.discoveryObserver != nil {
		if err := b.discoveryObserver.Close(ctx); err != nil {
			return err
		}
		b.discoveryObserver = nil
	}

	// Power off adapter.
	if err := b.SetPowered(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable floss adapter")
	}

	// Clear enabled state and clients.
	b.flossEnabled = false
	b.adapterEnabled = false
	b.managerClient = nil
	b.adapterClient = nil

	// Configure DUT to not use Floss.
	if err := SetFlossEnabled(ctx, false); err != nil {
		return errors.Wrap(err, "failed to set floss as not enabled")
	}

	return nil
}

func (b *BluetoothFlossFacade) assertManagerClientExists() error {
	if b.managerClient == nil {
		return errors.New("Enable must be called again prior to using this method")
	}
	return nil
}

func (b *BluetoothFlossFacade) assertEnabled() error {
	if !b.flossEnabled {
		return errors.New("Enable must be called prior to using this method")
	}
	if !b.adapterEnabled {
		return errors.New("SetPowered must be called to power on adapter prior to using this method")
	}
	return nil
}

// Reset resets the default adapter and turns it back on if powerOn is true.
func (b *BluetoothFlossFacade) Reset(ctx context.Context, powerOn bool) error {
	testing.ContextLog(ctx, "Starting reset of floss bluetooth adapter state")
	if err := b.assertEnabled(); err != nil {
		return err
	}

	// Make sure bluetooth is on, then remove all bonded devices.
	powered, err := b.IsPoweredOn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check if floss adapter is enabled")
	}
	if !powered {
		if err := b.SetPowered(ctx, true); err != nil {
			return errors.Wrap(err, "failed to enable floss adapter")
		}
	}
	if b.discoveryObserver != nil {
		testing.ContextLog(ctx, "Closing preexisting discovery observer")
		if err := b.discoveryObserver.Close(ctx); err != nil {
			return err
		}
		b.discoveryObserver = nil
	}
	testing.ContextLog(ctx, "Checking for known devices")
	deviceAddresses, err := b.Devices(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to fetch bonded devices")
	}
	testing.ContextLogf(ctx, "Found %d known devices", len(deviceAddresses))
	removedDevices := false
	for _, address := range deviceAddresses {
		testing.ContextLogf(ctx, "Removing device with address %q", address)
		if err := b.RemoveDevice(ctx, address); err != nil {
			return errors.Wrapf(err, "failed to remove bonded device with address %q", address)
		}
		removedDevices = true
	}
	if removedDevices {
		const removeDevicesTimeout = 5 * time.Second
		testing.ContextLogf(ctx, "Confirming all devices have been removed (%s timeout)", removeDevicesTimeout)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			deviceAddresses, err = b.Devices(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to fetch bonded devices")
			}
			if len(deviceAddresses) != 0 {
				return errors.Errorf("failed to remove all known devices, still found %d known devices after removal", len(deviceAddresses))
			}
			return nil
		}, &testing.PollOptions{
			Interval: time.Second,
			Timeout:  removeDevicesTimeout,
		}); err != nil {
			return errors.Wrap(err, "failed to confirm all devices were removed")
		}
		testing.ContextLog(ctx, "All known devices have been removed")
	}

	// Turn bluetooth off, then turn it back on if desired.
	// An additional power cycle is added if devices were removed to validate
	// their removal.
	if err := b.SetPowered(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable floss adapter")
	}
	if powerOn || removedDevices {
		if err := b.SetPowered(ctx, true); err != nil {
			return errors.Wrap(err, "failed to enable floss adapter")
		}
		testing.ContextLog(ctx, "Validating that floss adapter still does not have any known devices after power cycle")
		deviceAddresses, err = b.Devices(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to fetch bonded devices")
		}
		if len(deviceAddresses) != 0 {
			return errors.Errorf("found %d known devices after power cycle", len(deviceAddresses))
		}
		testing.ContextLog(ctx, "No known devices found")
		if !powerOn {
			if err := b.SetPowered(ctx, false); err != nil {
				return errors.Wrap(err, "failed to disable floss adapter")
			}
		}
	}
	testing.ContextLog(ctx, "Completed reset of floss bluetooth adapter state")
	return nil
}

// AdapterProperties collects and returns the default adapter's properties as
// a BluetoothAdapterProperties object.
func (b *BluetoothFlossFacade) AdapterProperties(ctx context.Context) (*common.BluetoothAdapterProperties, error) {
	props := &common.BluetoothAdapterProperties{}
	var err error
	props.Address, err = b.Address(ctx)
	if err != nil {
		return nil, err
	}
	props.Name, err = b.Name(ctx)
	if err != nil {
		return nil, err
	}
	props.Powered, err = b.IsPoweredOn(ctx)
	if err != nil {
		return nil, err
	}
	props.Discovering, err = b.IsDiscovering(ctx)
	if err != nil {
		return nil, err
	}
	props.Discoverable, err = b.IsDiscoverable(ctx)
	if err != nil {
		return nil, err
	}
	props.Pairable, err = b.IsPairable(ctx)
	if err != nil {
		return nil, err
	}
	return props, nil
}

// IsPoweredOn returns true if the default adapter is enabled.
func (b *BluetoothFlossFacade) IsPoweredOn(ctx context.Context) (bool, error) {
	if b.managerClient == nil {
		if err := b.initializeManagerClient(ctx); err != nil {
			return false, err
		}
	}
	return b.managerClient.GetAdapterEnabled(ctx, b.adapterHCI)
}

// SetPowered sets the default adapter's enabled state.
func (b *BluetoothFlossFacade) SetPowered(ctx context.Context, powered bool) error {
	if err := b.assertManagerClientExists(); err != nil {
		return err
	}
	if powered {
		testing.ContextLogf(ctx, "Starting floss adapter with HCI %d", b.adapterHCI)
		if err := b.managerClient.Start(ctx, b.adapterHCI); err != nil {
			return err
		}
		testing.ContextLogf(ctx, "Waiting for floss adapter with HCI %d to be enabled", b.adapterHCI)
		if err := b.waitForAdapterEnabledState(ctx, true); err != nil {
			return errors.Wrapf(err, "failed to wait for floss adapter with HCI %d to be enabled", b.adapterHCI)
		}
		if err := b.initializeAdapterSpecificDBusClients(ctx); err != nil {
			return errors.Wrap(err, "failed to initialize dbus clients after starting adapter")
		}
		b.adapterEnabled = true
		testing.ContextLogf(ctx, "Successfully started and enabled floss adapter with HCI %d", b.adapterHCI)
	} else {
		testing.ContextLogf(ctx, "Stopping floss adapter with HCI %d", b.adapterHCI)
		if err := b.managerClient.Stop(ctx, b.adapterHCI); err != nil {
			return err
		}
		testing.ContextLogf(ctx, "Waiting for floss adapter with HCI %d to be disabled", b.adapterHCI)
		if err := b.waitForAdapterEnabledState(ctx, false); err != nil {
			return errors.Wrapf(err, "failed to wait for floss adapter with HCI %d to be disabled", b.adapterHCI)
		}
		b.adapterEnabled = false
		testing.ContextLogf(ctx, "Successfully stopped and disabled floss adapter with HCI %d", b.adapterHCI)
	}
	return nil
}

func (b *BluetoothFlossFacade) waitForAdapterEnabledState(ctx context.Context, desiredEnabledState bool) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		enabled, err := b.managerClient.GetAdapterEnabled(ctx, b.adapterHCI)
		if err != nil {
			return err
		}
		if enabled != desiredEnabledState {
			return errors.Errorf("adapter with HCI %d enabled state is %t, want %t", b.adapterHCI, enabled, desiredEnabledState)
		}
		return nil
	}, &testing.PollOptions{
		Interval: 100 * time.Millisecond,
		Timeout:  30 * time.Second,
	}); err != nil {
		return errors.Wrapf(err, "failed to wait for adapter with HCI %d enabled state to be %t", b.adapterHCI, desiredEnabledState)
	}
	return nil
}

// StartDiscovery starts the discovery of remote devices.
func (b *BluetoothFlossFacade) StartDiscovery(ctx context.Context) error {
	if err := b.assertEnabled(); err != nil {
		return err
	}
	if b.discoveryObserver != nil {
		if err := b.discoveryObserver.Close(ctx); err != nil {
			return err
		}
		b.discoveryObserver = nil
	}
	var err error
	b.discoveryObserver, err = newDiscoveryObserver(ctx, b.adapterClient)
	if err != nil {
		return err
	}
	return b.adapterClient.StartDiscovery(ctx)
}

// StopDiscovery stops the discovery of remote devices.
func (b *BluetoothFlossFacade) StopDiscovery(ctx context.Context) error {
	if err := b.assertEnabled(); err != nil {
		return err
	}
	if b.discoveryObserver != nil {
		if err := b.discoveryObserver.Close(ctx); err != nil {
			return err
		}
		b.discoveryObserver = nil
	}
	return b.adapterClient.CancelDiscovery(ctx)
}

// IsDiscovering checks if the adapter is discovering.
func (b *BluetoothFlossFacade) IsDiscovering(ctx context.Context) (bool, error) {
	if err := b.assertEnabled(); err != nil {
		return false, err
	}
	return b.adapterClient.IsDiscovering(ctx)
}

// IsDiscoverable returns true if the default adapter is discoverable.
func (b *BluetoothFlossFacade) IsDiscoverable(ctx context.Context) (bool, error) {
	if err := b.assertEnabled(); err != nil {
		return false, err
	}
	return b.adapterClient.GetDiscoverable(ctx)
}

// SetDiscoverable sets whether the default adapter is discoverable.
func (b *BluetoothFlossFacade) SetDiscoverable(ctx context.Context, discoverable bool) error {
	return b.SetDiscoverableWithTimeout(ctx, discoverable, 60)
}

// SetDiscoverableWithTimeout sets whether the default adapter is discoverable.
// If discoverable is true, it will be set as discoverable for the provided
// timeout in seconds.
func (b *BluetoothFlossFacade) SetDiscoverableWithTimeout(ctx context.Context, discoverable bool, timeoutSeconds uint32) error {
	if err := b.assertEnabled(); err != nil {
		return err
	}
	var mode floss.BtDiscoverableMode
	if discoverable {
		mode = floss.BtDiscoverableModeGeneral
	} else {
		mode = floss.BtDiscoverableModeNon
	}
	return b.adapterClient.SetDiscoverable(ctx, mode, timeoutSeconds)
}

// IsPairable returns true if the default adapter is pairable.
// Note: For floss this is just a mock variable set only by SetPairable and
// has no influence on the adapter itself.
func (b *BluetoothFlossFacade) IsPairable(ctx context.Context) (bool, error) {
	if err := b.assertEnabled(); err != nil {
		return false, err
	}
	return b.mockIsPairable, nil
}

// SetPairable sets whether the default adapter is pairable.
// Note: For floss this is just a mock variable set only by this function and
// has no influence on the adapter itself.
func (b *BluetoothFlossFacade) SetPairable(ctx context.Context, pairable bool) error {
	if err := b.assertEnabled(); err != nil {
		return err
	}
	b.mockIsPairable = pairable
	return nil
}

// Address returns the address of the default adapter.
func (b *BluetoothFlossFacade) Address(ctx context.Context) (string, error) {
	if err := b.assertEnabled(); err != nil {
		return "", err
	}
	return b.adapterClient.GetAddress(ctx)
}

// Name returns the name of the default adapter.
func (b *BluetoothFlossFacade) Name(ctx context.Context) (string, error) {
	if err := b.assertEnabled(); err != nil {
		return "", err
	}
	return b.adapterClient.GetName(ctx)
}

// UUIDs returns the UUIDs of the default adapter.
func (b *BluetoothFlossFacade) UUIDs(ctx context.Context) ([]string, error) {
	if err := b.assertEnabled(); err != nil {
		return nil, err
	}
	return b.adapterClient.GetUuids(ctx)
}

// Devices returns the addresses of all known devices.
func (b *BluetoothFlossFacade) Devices(ctx context.Context) ([]string, error) {
	if err := b.assertEnabled(); err != nil {
		return nil, err
	}
	devices, err := b.adapterClient.GetBondedDevices(ctx)
	if err != nil {
		return nil, err
	}
	var deviceAddresses []string
	for _, device := range devices {
		deviceAddresses = append(deviceAddresses, device.Address)
	}
	return deviceAddresses, nil
}

// HasDevice checks if the adapter knows the device with the given address.
// A device is known if is paired or if it has been discovered by the adapter.
func (b *BluetoothFlossFacade) HasDevice(ctx context.Context, address string) (bool, error) {
	if err := b.assertEnabled(); err != nil {
		return false, err
	}
	if b.discoveryObserver != nil {
		device := b.discoveryObserver.FoundDeviceByAddress(address)
		if device != nil {
			return true, nil
		}
	}
	bondedDevices, err := b.adapterClient.GetBondedDevices(ctx)
	if err != nil {
		return false, err
	}
	for _, device := range bondedDevices {
		if device.Address == address {
			return true, nil
		}
	}
	return false, nil
}

// RemoveDevice removes/forgets a known device with the given address.
func (b *BluetoothFlossFacade) RemoveDevice(ctx context.Context, address string) error {
	if err := b.assertEnabled(); err != nil {
		return err
	}
	device := &floss.BluetoothDevice{Address: address}
	return b.adapterClient.RemoveBond(ctx, device)
}

// ConnectDevice connects to a device with the given address.
func (b *BluetoothFlossFacade) ConnectDevice(ctx context.Context, address string) error {
	if err := b.assertEnabled(); err != nil {
		return err
	}
	device := &floss.BluetoothDevice{Address: address}
	return b.adapterClient.ConnectAllEnabledProfiles(ctx, device)
}

// DisconnectDevice disconnects the device with the given address.
func (b *BluetoothFlossFacade) DisconnectDevice(ctx context.Context, address string) error {
	if err := b.assertEnabled(); err != nil {
		return err
	}
	device := &floss.BluetoothDevice{Address: address}
	return b.adapterClient.DisconnectAllEnabledProfiles(ctx, device)
}

// DeviceProperties collects and returns a known device's properties as a
// BluetoothDeviceProperties.
func (b *BluetoothFlossFacade) DeviceProperties(ctx context.Context, address string) (*common.BluetoothDeviceProperties, error) {
	props := &common.BluetoothDeviceProperties{
		Address: address,
	}
	var err error
	props.Name, err = b.DeviceName(ctx, address)
	if err != nil {
		return nil, err
	}
	props.Alias, err = b.DeviceAlias(ctx, address)
	if err != nil {
		return nil, err
	}
	props.Connected, err = b.DeviceIsConnected(ctx, address)
	if err != nil {
		return nil, err
	}
	props.Paired, err = b.DeviceIsPaired(ctx, address)
	if err != nil {
		return nil, err
	}
	return props, nil
}

// DeviceIsConnected checks whether the device with the given address is
// connected.
func (b *BluetoothFlossFacade) DeviceIsConnected(ctx context.Context, address string) (bool, error) {
	if err := b.assertEnabled(); err != nil {
		return false, err
	}
	device := &floss.BluetoothDevice{Address: address}
	return b.adapterClient.GetRemoteConnected(ctx, device)
}

// DeviceIsPaired checks whether the device with the given address is paired.
func (b *BluetoothFlossFacade) DeviceIsPaired(ctx context.Context, address string) (bool, error) {
	if err := b.assertEnabled(); err != nil {
		return false, err
	}
	device := &floss.BluetoothDevice{Address: address}
	bondState, err := b.adapterClient.GetBondState(ctx, device)
	if err != nil {
		return false, err
	}
	return bondState == floss.BtBondStateBonded, nil
}

// DeviceAddress returns the address of a known device with the given name.
func (b *BluetoothFlossFacade) DeviceAddress(ctx context.Context, name string) (string, error) {
	if err := b.assertEnabled(); err != nil {
		return "", err
	}
	if b.discoveryObserver != nil {
		device := b.discoveryObserver.FoundDeviceByName(name)
		if device != nil {
			return device.Address, nil
		}
	}
	bondedDevices, err := b.adapterClient.GetBondedDevices(ctx)
	if err != nil {
		return "", err
	}
	for _, device := range bondedDevices {
		if device.Name == name {
			return device.Address, nil
		}
	}
	return "", errors.Errorf("device with name %q not known by floss adapter", name)
}

// DeviceName returns the name of a known device with the given address.
func (b *BluetoothFlossFacade) DeviceName(ctx context.Context, address string) (string, error) {
	if err := b.assertEnabled(); err != nil {
		return "", err
	}
	device := &floss.BluetoothDevice{Address: address}
	return b.adapterClient.GetRemoteName(ctx, device)
}

// DeviceAlias returns the alias of a known device with the given address.
func (b *BluetoothFlossFacade) DeviceAlias(ctx context.Context, address string) (string, error) {
	if err := b.assertEnabled(); err != nil {
		return "", err
	}
	device := &floss.BluetoothDevice{Address: address}
	return b.adapterClient.GetRemoteAlias(ctx, device)
}

// DiscoverDevice will start discovery, wait until a device is found, and then
// stop discovery. A non-nil error return indicates that the adapter was able
// to successfully discover the device before the timeout was reached.
func (b *BluetoothFlossFacade) DiscoverDevice(ctx context.Context, address string, discoveryTimeout time.Duration) error {
	return common.DiscoverDevice(ctx, b, address, discoveryTimeout)
}

// PairDevice pairs a peer device with the given address and authentication
// pin.
//
// DiscoverDevice should be called prior to PairDevice to ensure that the
// adapter knows of the device for pairing.
func (b *BluetoothFlossFacade) PairDevice(ctx context.Context, address, pin string) error {
	if err := b.assertEnabled(); err != nil {
		return err
	}
	if address == "" {
		return errors.New("non-empty address is required")
	}
	testing.ContextLogf(ctx, "Pairing and connecting bluetooth device with address %q with the floss bluetooth adapter", address)
	pairingObserver, err := newPairingObserver(ctx, b.adapterClient, address)
	if err != nil {
		return errors.Wrap(err, "failed to initialize pairing observer")
	}
	defer func() {
		if err := pairingObserver.Close(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to Close pairingObserver for device with address %q: %v", address, err)
		}
	}()
	device := &floss.BluetoothDevice{Address: address}
	bondState, err := b.adapterClient.GetBondState(ctx, device)
	if err != nil {
		return errors.Wrapf(err, "failed to get bonded state of device with address %q", address)
	}
	if bondState == floss.BtBondStateNotBonded {
		testing.ContextLogf(ctx, "Creating new bond for device with address %q", address)
		if err := b.adapterClient.CreateBond(ctx, device, floss.BtTransportAuto); err != nil {
			return errors.Wrapf(err, "failed to create bond for device with address %q", address)
		}
	} else if bondState == floss.BtBondStateBonded {
		testing.ContextLogf(ctx, "Device with with address %q is already bonded", address)
		pairingObserver.SetTargetDeviceBondState(bondState)
		connected, err := b.adapterClient.GetRemoteConnected(ctx, device)
		if err != nil {
			return errors.Wrapf(err, "failed to get connected state of already bonded device with address %q", address)
		}
		if connected {
			testing.ContextLogf(ctx, "Device with with address %q is already connected", address)
			pairingObserver.SetTargetDeviceConnected(connected)
		} else if err := b.adapterClient.ConnectAllEnabledProfiles(ctx, device); err != nil {
			return errors.Wrapf(err, "failed to connect already bonded device with address %q", address)
		}
	}
	testing.ContextLogf(ctx, "Waiting for confirmation of bond and connection of device with address %q", address)
	if err := pairingObserver.WaitForBondedAndConnected(1 * time.Minute); err != nil {
		return errors.Wrapf(err, "failed to wait for device with address %q to be bonded and connected", address)
	}

	// If pairing and hci connection is complete, also trigger all
	// profile connections here. This is necessary because device
	// connection doesn't always imply profile connection.
	testing.ContextLogf(ctx, "Connecting all enabled profiles for device with address %q", address)
	if err := b.adapterClient.ConnectAllEnabledProfiles(ctx, device); err != nil {
		return errors.Wrapf(err, "failed to connect all enabled profiles of bonded device with address %q", address)
	}

	testing.ContextLogf(ctx, "Successfully paired and connected bluetooth device with address %q with the floss bluetooth adapter", address)
	return nil
}

// ManagerClient returns the current floss ManagerClient.
func (b *BluetoothFlossFacade) ManagerClient() (*floss.ManagerClient, error) {
	if err := b.assertEnabled(); err != nil {
		return nil, err
	}
	return b.managerClient, nil
}

// AdapterClient returns the current floss AdapterClient for the default
// adapter.
func (b *BluetoothFlossFacade) AdapterClient() (*floss.AdapterClient, error) {
	if err := b.assertEnabled(); err != nil {
		return nil, err
	}
	return b.adapterClient, nil
}

// EnabledOnBoot returns the value of the system setting that determines if
// the bluetooth adapter is enabled on boot.
// Note: This requires the chrome ui to have been loaded with the signin
// profile test extension key.
func (b *BluetoothFlossFacade) EnabledOnBoot(ctx context.Context) (bool, error) {
	return common.EnabledOnBoot(ctx)
}

// SetEnabledOnBoot sets the value of the system setting that determines if
// the bluetooth adapter is enabled on boot.
// Note: This requires the chrome ui to have been loaded with the signin
// profile test extension key.
func (b *BluetoothFlossFacade) SetEnabledOnBoot(ctx context.Context, adapterEnabledOnBoot bool) error {
	return common.SetEnabledOnBoot(ctx, adapterEnabledOnBoot)
}

// SetDebugLogLevels sets the level for verbose debug log. For Floss, this has not been implemented.
func (b *BluetoothFlossFacade) SetDebugLogLevels(ctx context.Context, level uint32) error {
	return errors.New("setting log level in floss is not supported")
}

// IsWBSSupported returns true if the adapter supports Wide-Band Speech (WBS).
func (b *BluetoothFlossFacade) IsWBSSupported(ctx context.Context) (bool, error) {
	if err := b.assertEnabled(); err != nil {
		return false, err
	}
	return b.adapterClient.IsWbsSupported(ctx)
}

// IsSWBSupported returns true if the adapter supports Super Wide-Band (SWB).
func (b *BluetoothFlossFacade) IsSWBSupported(ctx context.Context) (bool, error) {
	if err := b.assertEnabled(); err != nil {
		return false, err
	}
	return b.adapterClient.IsSwbSupported(ctx)
}
