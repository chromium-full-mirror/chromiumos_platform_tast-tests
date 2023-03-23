// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"
	"encoding/json"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// BluetoothAdapterProperties is a collection of properties of a bluetooth
// adapter.
type BluetoothAdapterProperties struct {
	// Powered is true if the adapter is powered on.
	Powered bool

	// Discovering is true if the adapter is in the discovering state.
	Discovering bool

	// Discoverable is true if the adapter is discoverable externally.
	Discoverable bool

	// Pairable is true if devices may be paired with the adapter.
	Pairable bool

	// Address is the external bluetooth mac address of the adapter.
	Address string

	// Name is the external bluetooth name of the DUT.
	Name string
}

// String returns BluetoothAdapterProperties as a JSON string.
func (p *BluetoothAdapterProperties) String() string {
	marshalledJSON, err := json.Marshal(*p)
	if err != nil {
		panic(errors.Wrap(err, "failed to marshal BluetoothAdapterProperties object to JSON"))
	}
	return string(marshalledJSON)
}

// BluetoothDeviceProperties is a collection of properties of a bluetooth
// device.
type BluetoothDeviceProperties struct {
	// Address is the mac address of the bluetooth device.
	Address string

	// Name is the name of the bluetooth device.
	Name string

	// Alias is the alias of the bluetooth device.
	Alias string

	// Connected is true if the device is connected.
	Connected bool

	// Paired is true if the device is paired.
	Paired bool
}

// String returns BluetoothDeviceProperties as a JSON string.
func (p *BluetoothDeviceProperties) String() string {
	marshalledJSON, err := json.Marshal(*p)
	if err != nil {
		panic(errors.Wrap(err, "failed to marshal BluetoothDeviceProperties object to JSON"))
	}
	return string(marshalledJSON)
}

// BluetoothStackType refers to name of a bluetooth stack, and is used to
// differentiate implementations of BluetoothFacade for a given stack.
type BluetoothStackType string

const (
	// BluetoothStackTypeBluez is the BluetoothStackType for the bluez stack.
	BluetoothStackTypeBluez BluetoothStackType = "bluez"

	// BluetoothStackTypeFloss is the BluetoothStackType for the floss stack.
	BluetoothStackTypeFloss BluetoothStackType = "floss"
)

// BluetoothFacade is the generic interface for interacting with different
// bluetooth stacks. This interface should be used instead of using the direct
// facade for a given stack whenever possible so that usage of a bluetooth stack
// remains independent of the stack type.
//
// This is the Tast version of Autotest's BluetoothBaseFacadeLocal, with
// functionality added on an as-needed basis.
type BluetoothFacade interface {
	// StackType returns the BluetoothStackType the facade uses.
	StackType() BluetoothStackType

	// Enable will turn on the bluetooth daemons and power on adapter.
	Enable(ctx context.Context) error

	// Disable will turn off the bluetooth daemons and power off adapter.
	Disable(ctx context.Context) error

	// Reset resets the default adapter and turns it back on if powerOn is true.
	Reset(ctx context.Context, powerOn bool) error

	// AdapterProperties collects and returns the default adapter's properties as
	// a BluetoothAdapterProperties object.
	AdapterProperties(ctx context.Context) (*BluetoothAdapterProperties, error)

	// IsPoweredOn returns true if the default adapter is enabled.
	IsPoweredOn(ctx context.Context) (bool, error)

	// SetPowered sets the default adapter's enabled state.
	SetPowered(ctx context.Context, powered bool) error

	// StartDiscovery starts the discovery of remote devices.
	StartDiscovery(ctx context.Context) error

	// StopDiscovery stops the discovery of remote devices.
	StopDiscovery(ctx context.Context) error

	// IsDiscovering checks if the adapter is discovering.
	IsDiscovering(ctx context.Context) (bool, error)

	// IsDiscoverable returns true if the default adapter is discoverable.
	IsDiscoverable(ctx context.Context) (bool, error)

	// SetDiscoverable sets whether the default adapter is discoverable.
	SetDiscoverable(ctx context.Context, discoverable bool) error

	// SetDiscoverableWithTimeout sets whether the default adapter is
	// discoverable. If discoverable is true, it will be set as discoverable for
	// the provided timeout in seconds.
	SetDiscoverableWithTimeout(ctx context.Context, discoverable bool, timeoutSeconds uint32) error

	// IsPairable returns true if the default adapter is pairable.
	IsPairable(ctx context.Context) (bool, error)

	// SetPairable sets whether the default adapter is pairable.
	SetPairable(ctx context.Context, pairable bool) error

	// Address returns the address of the default adapter.
	Address(ctx context.Context) (string, error)

	// Name returns the name of the default adapter.
	Name(ctx context.Context) (string, error)

	// UUIDs returns the UUIDs of the default adapter.
	UUIDs(ctx context.Context) ([]string, error)

	// Devices returns the addresses of all known devices.
	Devices(ctx context.Context) ([]string, error)

	// HasDevice checks if the adapter knows the device with the given address.
	// A device is known if is paired or if it has been discovered by the adapter.
	HasDevice(ctx context.Context, address string) (bool, error)

	// RemoveDevice removes/forgets a known device with the given address.
	RemoveDevice(ctx context.Context, address string) error

	// ConnectDevice connects to a device with the given address.
	ConnectDevice(ctx context.Context, address string) error

	// DisconnectDevice disconnects the device with the given address.
	DisconnectDevice(ctx context.Context, address string) error

	// DeviceProperties collects and returns a known device's properties as a
	// BluetoothDeviceProperties.
	DeviceProperties(ctx context.Context, address string) (*BluetoothDeviceProperties, error)

	// DeviceIsConnected checks whether the device with the given address is
	// connected.
	DeviceIsConnected(ctx context.Context, address string) (bool, error)

	// DeviceIsPaired checks whether the device with the given address is paired.
	DeviceIsPaired(ctx context.Context, address string) (bool, error)

	// DeviceAddress returns the address of a known device with the given name.
	DeviceAddress(ctx context.Context, name string) (string, error)

	// DeviceName returns the name of a known device with the given address.
	DeviceName(ctx context.Context, address string) (string, error)

	// DeviceAlias returns the alias of a known device with the given address.
	DeviceAlias(ctx context.Context, address string) (string, error)

	// DiscoverDevice will start discovery, wait until a device is found, and then
	// stop discovery. A non-nil error return indicates that the adapter was able
	// to successfully discover the device before the timeout was reached.
	DiscoverDevice(ctx context.Context, address string, discoveryTimeout time.Duration) error

	// PairDevice pairs a peer device with the given address and authentication
	// pin.
	//
	// DiscoverDevice should be called prior to PairDevice to ensure that the
	// adapter knows of the device for pairing.
	PairDevice(ctx context.Context, address, pin string) error
}

// DiscoverDevice will start discovery, wait until a device is found, and then
// stop discovery. A non-nil error return indicates that the adapter was able
// to successfully discover the device before the timeout was reached.
func DiscoverDevice(ctx context.Context, facade BluetoothFacade, address string, discoveryTimeout time.Duration) error {
	if discoveryTimeout == 0 {
		discoveryTimeout = 60 * time.Second
	}
	testing.ContextLog(ctx, "Starting bluetooth discovery")
	if err := facade.StartDiscovery(ctx); err != nil {
		return errors.New("failed to start discovery")
	}
	stopDiscoveryCtx := ctx
	ctx, _ = ctxutil.Shorten(ctx, 500*time.Millisecond)
	defer func() {
		testing.ContextLog(stopDiscoveryCtx, "Stopping bluetooth discovery")
		if err := facade.StopDiscovery(stopDiscoveryCtx); err != nil {
			testing.ContextLog(stopDiscoveryCtx, "Failed to stop discovery: ", err)
		}
	}()
	testing.ContextLogf(ctx, "Waiting for bluetooth adapter to discover device with address %q (%s timeout)", address, discoveryTimeout.String())
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		hasDevice, err := facade.HasDevice(ctx, address)
		if err != nil {
			return errors.Wrapf(err, "failed to check if bluetooth adapter has device with address %q", address)
		}
		if hasDevice {
			return nil
		}
		isDiscovering, err := facade.IsDiscovering(ctx)
		if err != nil {
			return errors.New("failed to check if bluetooth adapter is still discovering")
		}
		if !isDiscovering {
			testing.ContextLog(ctx, "Restarting bluetooth discovery, device not yet found")
			if err := facade.StartDiscovery(ctx); err != nil {
				return errors.New("failed to restart discovery")
			}
		}
		return errors.Errorf("bluetooth adapter does not have device with address %q", address)
	}, &testing.PollOptions{
		Interval: 100 * time.Millisecond,
		Timeout:  discoveryTimeout,
	}); err != nil {
		return errors.Wrapf(err, "failed to wait until bluetooth adapter discovered device with address %q", address)
	}
	testing.ContextLogf(ctx, "Successfully discovered bluetooth device with address %q", address)
	return nil
}
