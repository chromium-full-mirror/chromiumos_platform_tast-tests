// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package common

import (
	"context"
	"encoding/json"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/localstate"
	localCommon "go.chromium.org/tast-tests/cros/local/common"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const ashSystemBluetoothAdapterEnabled = "ash.system.bluetooth.adapter_enabled"

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

	// BluetoothStackTypeUnknown is the undefined BluetoothStackType.
	BluetoothStackTypeUnknown BluetoothStackType = "unknown"
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

	// DeviceRSSI returns the RSSI of a known device with the given address.
	DeviceRSSI(ctx context.Context, address string) (int16, error)

	// DiscoverDevice will start discovery, wait until a device is found, and then
	// stop discovery. A non-nil error return indicates that the adapter was able
	// to successfully discover the device before the timeout was reached.
	DiscoverDevice(ctx context.Context, address string, discoveryTimeout time.Duration) error

	// DiscoverDeviceAndSampleRSSI will start discovery, wait until a device is
	// found and reports a valid RSSI, sample its RSSI while discovery is active,
	// and then stop discovery.
	DiscoverDeviceAndSampleRSSI(ctx context.Context, address string, discoveryTimeout time.Duration) (int16, error)

	// PairDevice pairs a peer device with the given address and authentication
	// pin.
	//
	// DiscoverDevice should be called prior to PairDevice to ensure that the
	// adapter knows of the device for pairing.
	PairDevice(ctx context.Context, address, pin string) error

	// EnabledOnBoot returns the value of the system setting that determines if
	// the bluetooth adapter is enabled on boot.
	// Note: This requires the chrome ui to have been loaded with the signin
	// profile test extension key.
	EnabledOnBoot(ctx context.Context) (bool, error)

	// SetEnabledOnBoot sets the value of the system setting that determines if
	// the bluetooth adapter is enabled on boot.
	// Note: This requires the chrome ui to have been loaded with the signin
	// profile test extension key.
	SetEnabledOnBoot(ctx context.Context, adapterEnabledOnBoot bool) error

	// SetDebugLogLevels sets the level for verbose debug log.
	SetDebugLogLevels(ctx context.Context, level uint32) error

	// IsWBSSupported returns true if the adapter supports Wide-Band Speech (WBS).
	IsWBSSupported(ctx context.Context) (bool, error)

	// IsSWBSupported returns true if the adapter supports Super Wide-Band (SWB).
	IsSWBSupported(ctx context.Context) (bool, error)

	// GetPasskey monitors the bonding process for specified address and returns passkey
	// for keyboard pairing.
	//
	// This blocks until the passkey is returned or timeout is hit.
	GetPasskey(ctx context.Context, address string) (uint32, error)
}

// InvalidRSSI is the sentinel value (127) returned by Floss GetRemoteRSSI when
// a device's RSSI is unknown or unavailable.
const InvalidRSSI int16 = 127

// discoverDevice is a shared helper for DiscoverDevice and DiscoverDeviceAndSampleRSSI.
func discoverDevice(ctx context.Context, facade BluetoothFacade, address string, discoveryTimeout time.Duration, sampleRSSI bool) (int16, error) {
	if discoveryTimeout == 0 {
		discoveryTimeout = 60 * time.Second
	}
	testing.ContextLog(ctx, "Starting bluetooth discovery")
	if err := facade.StartDiscovery(ctx); err != nil {
		return 0, errors.Wrap(err, "failed to start discovery")
	}
	stopDiscoveryCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 500*time.Millisecond)
	defer cancel()
	defer func() {
		testing.ContextLog(stopDiscoveryCtx, "Stopping bluetooth discovery")
		if err := facade.StopDiscovery(stopDiscoveryCtx); err != nil {
			testing.ContextLog(stopDiscoveryCtx, "Failed to stop discovery: ", err)
		}
	}()

	if sampleRSSI {
		testing.ContextLogf(ctx, "Waiting for bluetooth adapter to discover and sample RSSI for device with address %q (%s timeout)", address, discoveryTimeout.String())
	} else {
		testing.ContextLogf(ctx, "Waiting for bluetooth adapter to discover device with address %q (%s timeout)", address, discoveryTimeout.String())
	}

	var sampledRSSI int16
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		hasDevice, err := facade.HasDevice(ctx, address)
		var pollErr error
		if err != nil {
			pollErr = errors.Wrapf(err, "failed to check if bluetooth adapter has device with address %q", address)
		} else if !hasDevice {
			pollErr = errors.Errorf("bluetooth adapter does not have device with address %q", address)
		} else if sampleRSSI {
			rssi, err := facade.DeviceRSSI(ctx, address)
			if err != nil {
				pollErr = errors.Wrapf(err, "failed to get RSSI for device with address %q", address)
			} else if rssi == InvalidRSSI {
				pollErr = errors.Errorf("device with address %q reported invalid RSSI (%d)", address, rssi)
			} else {
				sampledRSSI = rssi
				return nil
			}
		} else {
			return nil // Device found, and we don't need RSSI
		}

		isDiscovering, err := facade.IsDiscovering(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to check if bluetooth adapter is still discovering")
		}
		if !isDiscovering {
			if sampleRSSI {
				testing.ContextLog(ctx, "Restarting bluetooth discovery, device or valid RSSI not yet found")
			} else {
				testing.ContextLog(ctx, "Restarting bluetooth discovery, device not yet found")
			}
			if err := facade.StartDiscovery(ctx); err != nil {
				return errors.Wrap(err, "failed to restart discovery")
			}
		}
		return pollErr
	}, &testing.PollOptions{
		Interval: 100 * time.Millisecond,
		Timeout:  discoveryTimeout,
	}); err != nil {
		if sampleRSSI {
			return 0, errors.Wrapf(err, "failed to wait until bluetooth adapter discovered and sampled RSSI for device with address %q", address)
		}
		return 0, errors.Wrapf(err, "failed to wait until bluetooth adapter discovered device with address %q", address)
	}

	if sampleRSSI {
		testing.ContextLogf(ctx, "Successfully discovered bluetooth device with address %q with RSSI %d dBm", address, sampledRSSI)
	} else {
		testing.ContextLogf(ctx, "Successfully discovered bluetooth device with address %q", address)
	}
	return sampledRSSI, nil
}

// DiscoverDevice will start discovery, wait until a device is found, and then stop discovery.
func DiscoverDevice(ctx context.Context, facade BluetoothFacade, address string, discoveryTimeout time.Duration) error {
	_, err := discoverDevice(ctx, facade, address, discoveryTimeout, false)
	return err
}

// DiscoverDeviceAndSampleRSSI will start discovery, wait until a device is found and reports a valid RSSI, sample its RSSI while discovery is active, and then stop discovery.
func DiscoverDeviceAndSampleRSSI(ctx context.Context, facade BluetoothFacade, address string, discoveryTimeout time.Duration) (int16, error) {
	return discoverDevice(ctx, facade, address, discoveryTimeout, true)
}

// EnabledOnBoot returns the value of the system setting that determines if
// the bluetooth adapter is enabled on boot.
// Note: This requires the chrome ui to have been loaded with the signin
// profile test extension key.
func EnabledOnBoot(ctx context.Context) (bool, error) {
	signinProfileConn, err := signinProfileTestAPIConn(ctx)
	if err != nil {
		return false, err
	}
	return chromeSettingsPrivateGetPref(ctx, signinProfileConn, ashSystemBluetoothAdapterEnabled)
}

// SetEnabledOnBoot sets the value of the system setting that determines if
// the bluetooth adapter is enabled on boot.
// Note: This requires the chrome ui to have been loaded with the signin
// profile test extension key.
func SetEnabledOnBoot(ctx context.Context, adapterEnabled bool) error {
	signinProfileConn, err := signinProfileTestAPIConn(ctx)
	if err != nil {
		return err
	}

	// Set preference.
	if err := chromeSettingsPrivateSetPref(ctx, signinProfileConn, ashSystemBluetoothAdapterEnabled, adapterEnabled); err != nil {
		return err
	}

	// Verify that the boot setting is set properly.
	enabledSettingValue, err := chromeSettingsPrivateGetPref(ctx, signinProfileConn, ashSystemBluetoothAdapterEnabled)
	if err != nil {
		return err
	}
	if enabledSettingValue != adapterEnabled {
		return errors.Errorf("failed to set boot preference (%s) to %t", ashSystemBluetoothAdapterEnabled, adapterEnabled)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		enabledSettingValue, err := localstate.UnmarshalPref("ash.system.bluetooth.adapter_enabled")
		if err != nil {
			return errors.Wrap(err, "failed to extract bluetooth status from Local State")
		}
		enabled, ok := enabledSettingValue.(bool)
		if !ok || (enabled != adapterEnabled) {
			return errors.Errorf("ash Bluetooth preference not updated properly: wanted %v, got %v", adapterEnabled, enabledSettingValue)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  40 * time.Second,
		Interval: time.Second,
	}); err != nil {
		return err
	}
	return nil
}

func signinProfileTestAPIConn(ctx context.Context) (*chrome.TestConn, error) {
	lc := localCommon.SharedObjectsForServiceSingleton
	if lc == nil || lc.Chrome == nil {
		return nil, errors.New("DUT not logged into chrome")
	}
	testConn, err := lc.Chrome.SigninProfileTestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create signin profile test API connection")
	}
	return testConn, nil
}

func chromeSettingsPrivateGetPref(ctx context.Context, signinProfileConn *chrome.TestConn, key string) (bool, error) {
	var out struct {
		Value bool `json:"value"`
	}
	if err := signinProfileConn.Call(ctx, &out, "tast.promisify(chrome.settingsPrivate.getPref)", key); err != nil {
		return false, errors.Wrapf(err, "failed to call chrome.settingsPrivate.getPref(%s)", key)
	}
	return out.Value, nil
}

func chromeSettingsPrivateSetPref(ctx context.Context, signinProfileConn *chrome.TestConn, key string, value bool) error {
	if err := signinProfileConn.Call(ctx, nil, "tast.promisify(chrome.settingsPrivate.setPref)", key, value); err != nil {
		return errors.Wrapf(err, "failed to call chrome.settingsPrivate.setPref(%s)", key)
	}
	return nil
}
