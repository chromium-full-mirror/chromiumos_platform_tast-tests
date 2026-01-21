// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// AdapterClient handles method calls to the floss adapter interface.
type AdapterClient struct {
	dbus                                *dbusutil.DBusObject
	adapterHCI                          int
	exportedBluetoothCallback           *exportedBluetoothCallback
	exportedBluetoothConnectionCallback *exportedBluetoothConnectionCallback
}

// NewAdapterClient initializes a new AdapterClient for a floss Bluetooth
// adapter object with the given adapterHCI.
func NewAdapterClient(ctx context.Context, adapterHCI int) (*AdapterClient, error) {
	path := buildAdapterSpecificObjectPath(adapterHCI, flossAdapterObjectPathSuffix)
	obj, err := NewFlossDBusObject(ctx, flossAdapterInterface, path)
	if err != nil {
		return nil, err
	}
	return &AdapterClient{
		dbus:       obj,
		adapterHCI: adapterHCI,
	}, nil
}

// DBusObject returns the D-Bus object wrapper for this object.
func (c *AdapterClient) DBusObject() *dbusutil.DBusObject {
	return c.dbus
}

// RegisterCallbackObserver will register the provided observer so that
// its event handlers are called when the corresponding D-Bus messages are
// observed. A unique name is also provided so that the observer may be later
// unregistered with UnregisterCallbackObserver
//
// Multiple observers may be registered at the same time, as only one
// BluetoothCallback is exported to D-Bus and simply passes the messages to
// every currently registered observer.
func (c *AdapterClient) RegisterCallbackObserver(ctx context.Context, name string, observer BluetoothCallbackObserver) error {
	testing.ContextLogf(ctx, "Registering new callback observer %q to floss adapter client", name)
	if c.exportedBluetoothCallback == nil {
		c.exportedBluetoothCallback = newExportedBluetoothCallback(c.adapterHCI)
		if err := c.exportedBluetoothCallback.Export(c.dbus.Conn()); err != nil {
			return errors.Wrap(err, "failed to export bluetooth callbacks")
		}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := c.dbus.Call(ctx, "RegisterCallback", c.exportedBluetoothCallback.DBusObjectPath()).Err; err != nil {
				if err.Error() == "Path, Interface, or Method does not exist" {
					return err
				}
				return testing.PollBreak(err)
			}
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: time.Second}); err != nil {
			return errors.Wrapf(err, "failed to call RegisterCallback for exportedBluetoothCallback with path %s", c.exportedBluetoothCallback.DBusObjectPath())
		}
	}
	c.exportedBluetoothCallback.AddObserver(name, observer)
	return nil
}

// UnregisterCallbackObserver unregisters the observer with the given name as
// specified with RegisterCallbackObserver. Once unregistered, the observer
// will no longer receive BluetoothCallback messages.
func (c *AdapterClient) UnregisterCallbackObserver(ctx context.Context, name string) error {
	testing.ContextLogf(ctx, "Unregistering callback observer %q from floss adapter client", name)
	if c.exportedBluetoothCallback == nil {
		return nil
	}
	c.exportedBluetoothCallback.RemoveObserver(name)
	if !c.exportedBluetoothCallback.HasObservers() {
		if err := c.exportedBluetoothCallback.Unexport(c.dbus.Conn()); err != nil {
			return errors.Wrap(err, "failed to unexport bluetooth callbacks")
		}
		c.exportedBluetoothCallback = nil
	}
	return nil
}

// RegisterConnectionCallbackObserver will register the provided observer so
// that its event handlers are called when the corresponding D-Bus messages are
// observed. A unique name is also provided so that the observer may be later
// unregistered with UnregisterConnectionCallbackObserver
//
// Multiple observers may be registered at the same time, as only one
// BluetoothConnectionCallback is exported to D-Bus and simply passes the
// messages to every currently registered observer.
func (c *AdapterClient) RegisterConnectionCallbackObserver(ctx context.Context, name string, observer BluetoothConnectionCallbackObserver) error {
	testing.ContextLogf(ctx, "Registering new connection callback observer %q to floss adapter client", name)
	if c.exportedBluetoothConnectionCallback == nil {
		c.exportedBluetoothConnectionCallback = newExportedBluetoothConnectionCallback(c.adapterHCI)
		if err := c.exportedBluetoothConnectionCallback.Export(c.dbus.Conn()); err != nil {
			return errors.Wrap(err, "failed to export bluetooth callbacks")
		}
		callbackID, err := c.dbus.CallForUInt32(ctx, "RegisterConnectionCallback", c.exportedBluetoothConnectionCallback.DBusObjectPath())
		if err != nil {
			return errors.Wrapf(err, "failed to call RegisterConnectionCallback for exportedBluetoothConnectionCallback with path %s", c.exportedBluetoothConnectionCallback.DBusObjectPath())
		}
		c.exportedBluetoothConnectionCallback.SetCallbackID(callbackID)
	}
	c.exportedBluetoothConnectionCallback.AddObserver(name, observer)
	return nil
}

// UnregisterConnectionCallbackObserver unregisters the observer with the given
// name as specified with UnregisterConnectionCallbackObserver. Once
// unregistered, the observer will no longer receive BluetoothConnectionCallback
// messages.
func (c *AdapterClient) UnregisterConnectionCallbackObserver(ctx context.Context, name string) error {
	testing.ContextLogf(ctx, "Unregistering connection callback observer %q from floss adapter client", name)
	if c.exportedBluetoothConnectionCallback == nil {
		return nil
	}
	c.exportedBluetoothConnectionCallback.RemoveObserver(name)
	if !c.exportedBluetoothConnectionCallback.HasObservers() {
		if err := c.exportedBluetoothConnectionCallback.Unexport(c.dbus.Conn()); err != nil {
			return errors.Wrap(err, "failed to unexport bluetooth connection callbacks")
		}
		if err := c.dbus.Call(ctx, "UnregisterConnectionCallback", c.exportedBluetoothConnectionCallback.CallbackID()).Err; err != nil {
			return errors.Wrapf(err, "failed to call UnregisterConnectionCallback for exportedBluetoothConnectionCallback with ID %d", c.exportedBluetoothConnectionCallback.CallbackID())
		}
		c.exportedBluetoothConnectionCallback = nil
	}
	return nil
}

// GetAddress calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) GetAddress(ctx context.Context) (string, error) {
	return c.dbus.CallForString(ctx, "GetAddress")
}

// GetUuids calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) GetUuids(ctx context.Context) ([]string, error) {
	return dbusCallFor128BitUUIDs(ctx, c.dbus, "GetUuids")
}

// GetName calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) GetName(ctx context.Context) (string, error) {
	return c.dbus.CallForString(ctx, "GetName")
}

// SetName calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) SetName(ctx context.Context, name string) error {
	return c.dbus.CallForSuccess(ctx, "SetName", name)
}

// GetBluetoothClass calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetBluetoothClass(ctx context.Context) (uint32, error) {
	return c.dbus.CallForUInt32(ctx, "GetBluetoothClass")
}

// SetBluetoothClass calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) SetBluetoothClass(ctx context.Context, bluetoothClass uint32) error {
	return c.dbus.CallForSuccess(ctx, "SetBluetoothClass", bluetoothClass)
}

// GetDiscoverable calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetDiscoverable(ctx context.Context) (bool, error) {
	return c.dbus.CallForBool(ctx, "GetDiscoverable")
}

// GetDiscoverableTimeout calls the floss D-Bus method with the same name for
// this object.
func (c *AdapterClient) GetDiscoverableTimeout(ctx context.Context) (uint32, error) {
	return c.dbus.CallForUInt32(ctx, "GetDiscoverableTimeout")
}

// SetDiscoverable calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) SetDiscoverable(ctx context.Context, mode BtDiscoverableMode, durationInSeconds uint32) error {
	return c.dbus.CallForSuccess(ctx, "SetDiscoverable", mode, durationInSeconds)
}

// IsMultiAdvertisementSupported calls the floss D-Bus method with the same name
// for this object.
func (c *AdapterClient) IsMultiAdvertisementSupported(ctx context.Context) (bool, error) {
	return c.dbus.CallForBool(ctx, "IsMultiAdvertisementSupported")
}

// IsLeExtendedAdvertisingSupported calls the floss D-Bus method with the same
// name for this object.
func (c *AdapterClient) IsLeExtendedAdvertisingSupported(ctx context.Context) (bool, error) {
	return c.dbus.CallForBool(ctx, "IsLeExtendedAdvertisingSupported")
}

// StartDiscovery calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) StartDiscovery(ctx context.Context) error {
	return c.dbus.CallForSuccess(ctx, "StartDiscovery")
}

// CancelDiscovery calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) CancelDiscovery(ctx context.Context) error {
	return c.dbus.CallForSuccess(ctx, "CancelDiscovery")
}

// IsDiscovering calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) IsDiscovering(ctx context.Context) (bool, error) {
	return c.dbus.CallForBool(ctx, "IsDiscovering")
}

// GetDiscoveryEndMillis calls the floss D-Bus method with the same name for
// this object.
func (c *AdapterClient) GetDiscoveryEndMillis(ctx context.Context) (uint64, error) {
	return c.dbus.CallForUInt64(ctx, "GetDiscoveryEndMillis")
}

// CreateBond calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) CreateBond(ctx context.Context, device *BluetoothDevice, transport BtTransport) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	btStatus, err := c.dbus.CallForUInt32(ctx, "CreateBond", device.Marshall(), transport)
	if err != nil {
		return err
	}
	if btStatus != uint32(BtStatusSuccess) {
		return errors.Errorf("create bond returned non-zero BtStatus %d", btStatus)
	}
	return nil
}

// CancelBondProcess calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) CancelBondProcess(ctx context.Context, device *BluetoothDevice) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	return c.dbus.CallForSuccess(ctx, "CancelBondProcess", device.Marshall())
}

// RemoveBond calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) RemoveBond(ctx context.Context, device *BluetoothDevice) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	return c.dbus.CallForSuccess(ctx, "RemoveBond", device.Marshall())
}

// GetBondedDevices calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetBondedDevices(ctx context.Context) ([]*BluetoothDevice, error) {
	call := c.dbus.Call(ctx, "GetBondedDevices")
	if call.Err != nil {
		return nil, call.Err
	}
	var dbusDevices []map[string]dbus.Variant
	if err := call.Store(&dbusDevices); err != nil {
		return nil, errors.Wrap(err, "failed to store result of GetBondedDevices")
	}
	var devices []*BluetoothDevice
	for _, dbusDevice := range dbusDevices {
		device, err := unmarshallBluetoothDevice(dbusDevice)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse stored result of GetBondedDevices: %v", dbusDevices)
		}
		devices = append(devices, device)
	}
	return devices, nil
}

// GetBondState calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) GetBondState(ctx context.Context, device *BluetoothDevice) (BtBondState, error) {
	if device == nil {
		return 0, errors.New("non-nil device required")
	}
	state, err := c.dbus.CallForUInt32(ctx, "GetBondState", device.Marshall())
	if err != nil {
		return 0, err
	}
	return BtBondState(state), nil
}

// SetPin calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) SetPin(ctx context.Context, device *BluetoothDevice, accept bool, pinCode []byte) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	return c.dbus.CallForSuccess(ctx, "SetPin", device.Marshall(), accept, pinCode)
}

// SetPasskey calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) SetPasskey(ctx context.Context, device *BluetoothDevice, accept bool, passkey []byte) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	return c.dbus.CallForSuccess(ctx, "SetPasskey", device.Marshall(), accept, passkey)
}

// SetPairingConfirmation calls the floss D-Bus method with the same name for
// this object.
func (c *AdapterClient) SetPairingConfirmation(ctx context.Context, device *BluetoothDevice, accept bool) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	return c.dbus.CallForSuccess(ctx, "SetPairingConfirmation", device.Marshall(), accept)
}

// GetRemoteName calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetRemoteName(ctx context.Context, device *BluetoothDevice) (string, error) {
	if device == nil {
		return "", errors.New("non-nil device required")
	}
	return c.dbus.CallForString(ctx, "GetRemoteName", device.Marshall())
}

// GetRemoteType calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetRemoteType(ctx context.Context, device *BluetoothDevice) (BtDeviceType, error) {
	if device == nil {
		return 0, errors.New("non-nil device required")
	}
	deviceType, err := c.dbus.CallForUInt32(ctx, "GetRemoteType", device.Marshall())
	if err != nil {
		return 0, err
	}
	return BtDeviceType(deviceType), nil
}

// GetRemoteAlias calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetRemoteAlias(ctx context.Context, device *BluetoothDevice) (string, error) {
	if device == nil {
		return "", errors.New("non-nil device required")
	}
	return c.dbus.CallForString(ctx, "GetRemoteAlias", device.Marshall())
}

// GetRemoteClass calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetRemoteClass(ctx context.Context, device *BluetoothDevice) (uint32, error) {
	if device == nil {
		return 0, errors.New("non-nil device required")
	}
	return c.dbus.CallForUInt32(ctx, "GetRemoteClass", device.Marshall())
}

// GetRemoteAppearance calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetRemoteAppearance(ctx context.Context, device *BluetoothDevice) (uint16, error) {
	if device == nil {
		return 0, errors.New("non-nil device required")
	}
	return c.dbus.CallForUInt16(ctx, "GetRemoteAppearance", device.Marshall())
}

// GetRemoteConnected calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetRemoteConnected(ctx context.Context, device *BluetoothDevice) (bool, error) {
	if device == nil {
		return false, errors.New("non-nil device required")
	}
	return c.dbus.CallForBool(ctx, "GetRemoteConnected", device.Marshall())
}

// GetRemoteWakeAllowed calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetRemoteWakeAllowed(ctx context.Context, device *BluetoothDevice) (bool, error) {
	if device == nil {
		return false, errors.New("non-nil device required")
	}
	return c.dbus.CallForBool(ctx, "GetRemoteWakeAllowed", device.Marshall())
}

// GetConnectedDevices calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetConnectedDevices(ctx context.Context) ([]*BluetoothDevice, error) {
	call := c.dbus.Call(ctx, "GetConnectedDevices")
	if call.Err != nil {
		return nil, call.Err
	}
	var dbusDevices []map[string]dbus.Variant
	if err := call.Store(&dbusDevices); err != nil {
		return nil, errors.Wrap(err, "failed to store result of GetConnectedDevices")
	}
	var devices []*BluetoothDevice
	for _, dbusDevice := range dbusDevices {
		device, err := unmarshallBluetoothDevice(dbusDevice)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse stored result of GetConnectedDevices: %v", dbusDevices)
		}
		devices = append(devices, device)
	}
	return devices, nil
}

// GetConnectionState calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetConnectionState(ctx context.Context, device *BluetoothDevice) (BtConnectionState, error) {
	if device == nil {
		return 0, errors.New("non-nil device required")
	}
	connectionState, err := c.dbus.CallForUInt32(ctx, "GetConnectionState", device.Marshall())
	if err != nil {
		return 0, err
	}
	return BtConnectionState(connectionState), nil
}

// GetRemoteUuids calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) GetRemoteUuids(ctx context.Context, device *BluetoothDevice) ([]string, error) {
	if device == nil {
		return nil, errors.New("non-nil device required")
	}
	return dbusCallFor128BitUUIDs(ctx, c.dbus, "GetRemoteUuids", device.Marshall())
}

// FetchRemoteUuids calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) FetchRemoteUuids(ctx context.Context, device *BluetoothDevice) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	return c.dbus.CallForSuccess(ctx, "FetchRemoteUuids", device.Marshall())
}

// SdpSearch calls the floss D-Bus method with the same name for this object.
func (c *AdapterClient) SdpSearch(ctx context.Context, device *BluetoothDevice, uuid string) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	marshalledUUID, err := marshall128BitUUID(uuid)
	if err != nil {
		return errors.Wrapf(err, "failed to marshal UUID %q", uuid)
	}
	return c.dbus.CallForSuccess(ctx, "SdpSearch", device.Marshall(), marshalledUUID)
}

// ConnectAllEnabledProfiles calls the floss D-Bus method with the same name for
// this object.
func (c *AdapterClient) ConnectAllEnabledProfiles(ctx context.Context, device *BluetoothDevice) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	btStatus, err := c.dbus.CallForUInt32(ctx, "ConnectAllEnabledProfiles", device.Marshall())
	if err != nil {
		return err
	}
	if btStatus != uint32(BtStatusSuccess) {
		return errors.Errorf("connect all enabled profiles returned non-zero BtStatus %d", btStatus)
	}
	return nil
}

// DisconnectAllEnabledProfiles calls the floss D-Bus method with the same name
// for this object.
func (c *AdapterClient) DisconnectAllEnabledProfiles(ctx context.Context, device *BluetoothDevice) error {
	if device == nil {
		return errors.New("non-nil device required")
	}
	return c.dbus.CallForSuccess(ctx, "DisconnectAllEnabledProfiles", device.Marshall())
}

// IsWbsSupported calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) IsWbsSupported(ctx context.Context) (bool, error) {
	return c.dbus.CallForBool(ctx, "IsWbsSupported")
}

// IsSwbSupported calls the floss D-Bus method with the same name for this
// object.
func (c *AdapterClient) IsSwbSupported(ctx context.Context) (bool, error) {
	return c.dbus.CallForBool(ctx, "IsSwbSupported")
}
