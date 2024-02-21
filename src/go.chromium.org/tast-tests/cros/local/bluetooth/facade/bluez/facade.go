// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluez

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/bluetooth/facade/common"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const bluezDaemonJob = "bluetoothd"

// BluetoothBluezFacade is an implementation of the BluetoothFacade interface
// for the bluez bluetooth stack.
type BluetoothBluezFacade struct {
	// cachedDefaultAdapter is a cached bluez adapter object.
	//
	// This should not be used directly outside Enable or Disable. Instead, get
	// this value with defaultAdapter so that it can return an error message if
	// Enable has not been called before or since Disable has been called.
	cachedDefaultAdapter *bluez.Adapter
}

// NewBluetoothBluezFacade initializes the bluez adapter and returns a
// new BluetoothBluezFacade.
func NewBluetoothBluezFacade(ctx context.Context) (*BluetoothBluezFacade, error) {
	b := &BluetoothBluezFacade{}
	if err := b.initializeAdapter(ctx); err != nil {
		return nil, err
	}
	return b, nil
}

// StackType returns the BluetoothStackType the facade uses.
func (b *BluetoothBluezFacade) StackType() common.BluetoothStackType {
	return common.BluetoothStackTypeBluez
}

// IsAlive checks if the bluetooth is alive by checking relative job/service.
func (b *BluetoothBluezFacade) IsAlive(ctx context.Context) (bool, error) {
	if !upstart.JobExists(ctx, bluezDaemonJob) {
		return false, nil
	}
	return upstart.IsServiceAvailable(ctx, bluez.DBusBluezService)
}

// Enable will turn on the bluetooth daemons and power on adapter.
func (b *BluetoothBluezFacade) Enable(ctx context.Context) error {
	if b.cachedDefaultAdapter == nil {
		if err := b.initializeAdapter(ctx); err != nil {
			return err
		}
	}

	// Power on the adapter.
	if err := b.SetPowered(ctx, true); err != nil {
		return errors.Wrap(err, "failed to power on adapter")
	}
	return nil
}

func (b *BluetoothBluezFacade) initializeAdapter(ctx context.Context) error {
	// Ensure daemon is running.
	if err := upstart.CheckJob(ctx, bluezDaemonJob); err != nil {
		if err := upstart.RestartJobAndWaitForDbusService(ctx, bluezDaemonJob, bluez.DBusBluezService); err != nil {
			return errors.Wrap(err, "failed to start bluez")
		}
	}
	// Get and store the default adapter.
	defaultAdapter, err := bluez.DefaultAdapter(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get default bluez bluetooth adapter")
	}
	b.cachedDefaultAdapter = defaultAdapter
	return nil
}

// Disable will turn off the bluetooth daemons and power off adapter.
func (b *BluetoothBluezFacade) Disable(ctx context.Context) error {
	if err := b.SetPowered(ctx, false); err != nil {
		return errors.Wrap(err, "failed to disable bluez adapter")
	}
	b.cachedDefaultAdapter = nil
	return upstart.StopJob(ctx, bluezDaemonJob)
}

func (b *BluetoothBluezFacade) defaultAdapter() (*bluez.Adapter, error) {
	if b.cachedDefaultAdapter == nil {
		return nil, errors.New("Enable must be called again prior to using adapter")
	}
	return b.cachedDefaultAdapter, nil
}

// Reset removes remote devices and sets the adapter power state.
//
// Does not restart bluetoothd as this may incur a side effect. The unhappy
// chrome may disable the adapter randomly.
func (b *BluetoothBluezFacade) Reset(ctx context.Context, powerOn bool) error {
	// Make sure adapter is on so devices may be removed.
	powered, err := b.IsPoweredOn(ctx)
	if err != nil {
		return err
	}
	if !powered {
		if err := b.SetPowered(ctx, true); err != nil {
			return err
		}
	}

	// Remove all devices.
	deviceAddresses, err := b.Devices(ctx)
	if err != nil {
		return err
	}
	for _, address := range deviceAddresses {
		if err := b.RemoveDevice(ctx, address); err != nil {
			return err
		}
	}

	// Power off, then power back on if desired.
	if err := b.SetPowered(ctx, false); err != nil {
		return err
	}
	if powerOn {
		if err := b.SetPowered(ctx, true); err != nil {
			return err
		}
	}

	return nil
}

// AdapterProperties collects and returns the default adapter's properties as
// a BluetoothAdapterProperties object.
func (b *BluetoothBluezFacade) AdapterProperties(ctx context.Context) (*common.BluetoothAdapterProperties, error) {
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
func (b *BluetoothBluezFacade) IsPoweredOn(ctx context.Context) (bool, error) {
	if b.cachedDefaultAdapter == nil {
		if err := b.initializeAdapter(ctx); err != nil {
			return false, err
		}
	}
	return b.cachedDefaultAdapter.Powered(ctx)
}

// SetPowered sets the default adapter's enabled state.
func (b *BluetoothBluezFacade) SetPowered(ctx context.Context, powered bool) error {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return err
	}
	return adapter.SetPowered(ctx, powered)
}

// StartDiscovery starts the discovery of remote devices.
func (b *BluetoothBluezFacade) StartDiscovery(ctx context.Context) error {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return err
	}
	return adapter.StartDiscovery(ctx)
}

// StopDiscovery stops the discovery of remote devices.
func (b *BluetoothBluezFacade) StopDiscovery(ctx context.Context) error {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return err
	}
	return adapter.StopDiscovery(ctx)
}

// IsDiscovering checks if the adapter is discovering.
func (b *BluetoothBluezFacade) IsDiscovering(ctx context.Context) (bool, error) {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return false, err
	}
	return adapter.Discovering(ctx)
}

// IsDiscoverable returns true if the default adapter is discoverable.
func (b *BluetoothBluezFacade) IsDiscoverable(ctx context.Context) (bool, error) {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return false, err
	}
	return adapter.Discoverable(ctx)
}

// SetDiscoverable sets whether the default adapter is discoverable.
func (b *BluetoothBluezFacade) SetDiscoverable(ctx context.Context, discoverable bool) error {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return err
	}
	return adapter.SetDiscoverable(ctx, discoverable)
}

// SetDiscoverableWithTimeout sets whether the default adapter is discoverable. If
// discoverable is true, it will be set as discoverable for the provided
// timeout in seconds.
func (b *BluetoothBluezFacade) SetDiscoverableWithTimeout(ctx context.Context, discoverable bool, timeoutSeconds uint32) error {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return err
	}
	if err := adapter.SetDiscoverable(ctx, discoverable); err != nil {
		return err
	}
	if discoverable {
		if err := adapter.SetDiscoverableTimeout(ctx, timeoutSeconds); err != nil {
			return err
		}
	}
	return nil
}

// IsPairable returns true if the default adapter is pairable.
func (b *BluetoothBluezFacade) IsPairable(ctx context.Context) (bool, error) {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return false, err
	}
	return adapter.Pairable(ctx)
}

// SetPairable sets whether the default adapter is pairable.
func (b *BluetoothBluezFacade) SetPairable(ctx context.Context, pairable bool) error {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return err
	}
	return adapter.SetPairable(ctx, pairable)
}

// Address returns the address of the default adapter.
func (b *BluetoothBluezFacade) Address(ctx context.Context) (string, error) {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return "", err
	}
	return adapter.Address(ctx)
}

// Name returns the name of the default adapter.
func (b *BluetoothBluezFacade) Name(ctx context.Context) (string, error) {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return "", err
	}
	return adapter.Name(ctx)
}

// UUIDs returns the UUIDs of the default adapter.
func (b *BluetoothBluezFacade) UUIDs(ctx context.Context) ([]string, error) {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return nil, err
	}
	return adapter.UUIDs(ctx)
}

func (b *BluetoothBluezFacade) deviceByAddress(ctx context.Context, address string, assertExistence bool) (*bluez.Device, error) {
	devices, err := bluez.Devices(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get bluez device list")
	}
	for _, d := range devices {
		ad, err := d.Address(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get bluez device address property")
		}
		if ad == address {
			return d, nil
		}
	}
	if assertExistence {
		return nil, errors.Errorf("bluez device with the given address %s not found", address)
	}
	return nil, nil
}

func (b *BluetoothBluezFacade) deviceByName(ctx context.Context, name string, assertExistence bool) (*bluez.Device, error) {
	devices, err := bluez.Devices(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get bluez device list")
	}
	for _, d := range devices {
		n, err := d.Name(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get bluez device name property")
		}
		if n == name {
			return d, nil
		}
	}
	if assertExistence {
		return nil, errors.Errorf("bluez device with the given alias %s not found", name)
	}
	return nil, nil
}

// Devices returns the addresses of all known devices.
func (b *BluetoothBluezFacade) Devices(ctx context.Context) ([]string, error) {
	devices, err := bluez.Devices(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get bluez device list")
	}
	var addresses []string
	for _, d := range devices {
		address, err := d.Address(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get bluez device address property")
		}
		addresses = append(addresses, address)
	}
	return addresses, nil
}

// HasDevice checks if the adapter knows the device with the given address.
// A device is known if is paired or if it has been discovered by the adapter.
func (b *BluetoothBluezFacade) HasDevice(ctx context.Context, address string) (bool, error) {
	device, err := b.deviceByAddress(ctx, address, false)
	if err != nil {
		return false, err
	}
	return device != nil, nil
}

// RemoveDevice removes/forgets a known device with the given address.
func (b *BluetoothBluezFacade) RemoveDevice(ctx context.Context, address string) error {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return err
	}
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return err
	}
	return adapter.RemoveDevice(ctx, device.Path())
}

// ConnectDevice connects to a device with the given address.
func (b *BluetoothBluezFacade) ConnectDevice(ctx context.Context, address string) error {
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return err
	}
	return device.Connect(ctx)
}

// DisconnectDevice disconnects the device with the given address.
func (b *BluetoothBluezFacade) DisconnectDevice(ctx context.Context, address string) error {
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return err
	}
	return device.Disconnect(ctx)
}

// DeviceProperties collects and returns a known device's properties as a
// BluetoothDeviceProperties.
func (b *BluetoothBluezFacade) DeviceProperties(ctx context.Context, address string) (*common.BluetoothDeviceProperties, error) {
	props := &common.BluetoothDeviceProperties{
		Address: address,
	}
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return nil, err
	}
	props.Name, err = device.Name(ctx)
	if err != nil {
		return nil, err
	}
	props.Alias, err = device.Alias(ctx)
	if err != nil {
		return nil, err
	}
	props.Connected, err = device.Connected(ctx)
	if err != nil {
		return nil, err
	}
	props.Paired, err = device.Paired(ctx)
	if err != nil {
		return nil, err
	}
	return props, nil
}

// DeviceIsConnected checks whether the device with the given address is
// connected.
func (b *BluetoothBluezFacade) DeviceIsConnected(ctx context.Context, address string) (bool, error) {
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return false, err
	}
	return device.Connected(ctx)
}

// DeviceIsPaired checks whether the device with the given address is paired.
func (b *BluetoothBluezFacade) DeviceIsPaired(ctx context.Context, address string) (bool, error) {
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return false, err
	}
	return device.Connected(ctx)
}

// DeviceAddress returns the address of a known device with the given name.
func (b *BluetoothBluezFacade) DeviceAddress(ctx context.Context, name string) (string, error) {
	device, err := b.deviceByName(ctx, name, true)
	if err != nil {
		return "", err
	}
	return device.Address(ctx)
}

// DeviceName returns the name of a known device with the given address.
func (b *BluetoothBluezFacade) DeviceName(ctx context.Context, address string) (string, error) {
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return "", err
	}
	return device.Name(ctx)
}

// DeviceAlias returns the alias of a known device with the given address.
func (b *BluetoothBluezFacade) DeviceAlias(ctx context.Context, address string) (string, error) {
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return "", err
	}
	return device.Alias(ctx)
}

// DiscoverDevice will start discovery, wait until a device is found, and then
// stop discovery. A non-nil error return indicates that the adapter was able
// to successfully discover the device before the timeout was reached.
func (b *BluetoothBluezFacade) DiscoverDevice(ctx context.Context, address string, discoveryTimeout time.Duration) error {
	return common.DiscoverDevice(ctx, b, address, discoveryTimeout)
}

// PairDevice pairs a peer device with the given address and authentication
// pin.
//
// DiscoverDevice should be called prior to PairDevice to ensure that the
// adapter knows of the device for pairing.
func (b *BluetoothBluezFacade) PairDevice(ctx context.Context, address, pin string) error {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return err
	}
	device, err := b.deviceByAddress(ctx, address, true)
	if err != nil {
		return err
	}

	testing.ContextLogf(ctx, "Pairing and connecting bluetooth device with address %q with the bluez bluetooth adapter", address)

	// Unpair the device if it is already paired.
	isPaired, err := device.Paired(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to check if device is paired")
	}
	if isPaired {
		testing.ContextLogf(ctx, "Removing and rediscovering already paired device at dbus path %q", device.Path())
		if err := adapter.RemoveDevice(ctx, device.Path()); err != nil {
			return errors.Wrapf(err, "failed to remove paired device at dbus path %q", device.Path())
		}
		device, err = b.deviceByAddress(ctx, address, true)
		if err != nil {
			return errors.Wrap(err, "failed rediscover device after removal")
		}
	}

	// Pair the device.
	if pin == "" {
		testing.ContextLogf(ctx, "Pairing device at dbus path %q", device.Path())
		if err := device.Pair(ctx); err != nil {
			return errors.Wrap(err, "failed to pair bluetooth device")
		}
	} else {
		// Prepare to handle pin authorization, if requested by the btpeer.
		testing.ContextLogf(ctx, "Preparing to authorize pairing with pin code %q", pin)
		agentManagers, err := bluez.AgentManagers(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get AgentManager")
		}
		if len(agentManagers) != 1 {
			return errors.Errorf("expected to get exactly 1 AgentManager, but got %d", len(agentManagers))
		}
		agentManager := agentManagers[0]
		testing.ContextLogf(ctx, "Using authentication AgentManager at dbus path %q", agentManager.DBusObject().ObjectPath())
		agent, err := bluez.NewAgent(ctx, "")
		if err != nil {
			return errors.Wrap(err, "failed to create new authentication Agent")
		}
		if err := agent.ExportAgentDelegate(bluez.NewSimplePinAgentDelegate(ctx, pin)); err != nil {
			return errors.Wrap(err, "failed to export AgentDelegate")
		}
		if err := agentManager.RegisterAgent(ctx, agent.DBusObject().ObjectPath(), "KeyboardDisplay"); err != nil {
			return errors.Wrapf(err, "failed to register Agent %q with AgentManager %q", agent.DBusObject().ObjectPath(), agentManager.DBusObject().ObjectPath())
		}
		if err := agentManager.RequestDefaultAgent(ctx, agent.DBusObject().ObjectPath()); err != nil {
			return errors.Wrapf(err, "failed to register Agent %q with AgentManager %q as default", agent.DBusObject().ObjectPath(), agentManager.DBusObject().ObjectPath())
		}
		testing.ContextLogf(ctx, "Using new authentication Agent to provide pin code %q at dbus path %q", pin, agent.DBusObject().ObjectPath())

		// Attempt paring.
		testing.ContextLogf(ctx, "Pairing device at dbus path %q", device.Path())
		if err := device.Pair(ctx); err != nil {
			return errors.Wrap(err, "failed to pair bluetooth device")
		}

		// BlueZ requires calling Connect() to connect profiles after Pair().
		testing.ContextLogf(ctx, "Connecting to device at dbus path %q", device.Path())
		if err := device.Connect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect bluetooth device")
		}

		// Cleanup pin authentication handling.
		testing.ContextLogf(ctx, "Removing authentication Agent at dbus path %q", agent.DBusObject().ObjectPath())
		if err := agent.ClearExportedAgentDelegate(); err != nil {
			return errors.Wrapf(err, "failed to clear exported AgentDelegate for Agent at %q", agent.DBusObject().ObjectPath())
		}
		if err := agentManager.UnregisterAgent(ctx, agent.DBusObject().ObjectPath()); err != nil {
			return errors.Wrapf(err, "failed to unregister Agent %q with AgentManager %q", agent.DBusObject().ObjectPath(), agentManager.DBusObject().ObjectPath())
		}
	}

	// Get connected status of BT device and connect if not already connected.
	isConnected, err := device.Connected(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get device connected status")
	}
	if !isConnected {
		testing.ContextLogf(ctx, "Connecting to device at dbus path %q", device.Path())
		if err := device.Connect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect bluetooth device")
		}
	}

	testing.ContextLogf(ctx, "Successfully paired and connected bluetooth device with address %q with the bluez bluetooth adapter", address)
	return nil
}

// EnabledOnBoot returns the value of the system setting that determines if
// the bluetooth adapter is enabled on boot.
// Note: This requires the chrome ui to have been loaded with the signin
// profile test extension key.
func (b *BluetoothBluezFacade) EnabledOnBoot(ctx context.Context) (bool, error) {
	return common.EnabledOnBoot(ctx)
}

// SetEnabledOnBoot sets the value of the system setting that determines if
// the bluetooth adapter is enabled on boot.
// Note: This requires the chrome ui to have been loaded with the signin
// profile test extension key.
func (b *BluetoothBluezFacade) SetEnabledOnBoot(ctx context.Context, adapterEnabledOnBoot bool) error {
	return common.SetEnabledOnBoot(ctx, adapterEnabledOnBoot)
}

// SetDebugLogLevels sets the level for verbose debug log.
// level 0 means bluez: false, floss: false
// level 1 means bluez: true, floss: true
func (b *BluetoothBluezFacade) SetDebugLogLevels(ctx context.Context, level uint32) error {
	if level > 1 {
		return errors.New("invalid log level")
	}
	bluezLevel := 0
	kernelLevel := 0
	if level == 1 {
		bluezLevel = 1
		kernelLevel = 1
	}
	debugDbus, err := bluez.NewDebug(ctx)
	if err != nil {
		errors.Wrap(err, "fails to generate bluez dbus debug interface")
	}
	debugDbus.SetLevels(ctx, bluezLevel, kernelLevel)

	return nil
}

// IsWBSSupported returns true if the adapter supports Wide-Band Speech (WBS).
func (b *BluetoothBluezFacade) IsWBSSupported(ctx context.Context) (bool, error) {
	adapter, err := b.defaultAdapter()
	if err != nil {
		return false, err
	}
	capabilities, err := adapter.GetSupportedCapabilities(ctx)
	if err != nil {
		return false, err
	}
	return capabilities["wide band speech"] == true, nil
}

// IsSWBSupported returns true if the adapter supports Super Wide-Band (SWB).
//
// Since SWB is not supported by bluez on chromeos, this simply returns false.
func (b *BluetoothBluezFacade) IsSWBSupported(ctx context.Context) (bool, error) {
	return false, nil
}
