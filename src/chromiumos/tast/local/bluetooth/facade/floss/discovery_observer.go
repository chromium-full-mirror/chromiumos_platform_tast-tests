// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import (
	"context"
	"sync"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/bluetooth/floss"
	"chromiumos/tast/testing"
)

const discoveryObserverName = "DiscoveryObserver"

// discoveryObserver is a BluetoothCallbackObserver that will keep a record
// of discovered devices and log observed changes.
type discoveryObserver struct {
	ctx                   context.Context
	adapterClient         *floss.AdapterClient
	discovering           bool
	foundDevicesByAddress map[string]*floss.BluetoothDevice
	stateMutex            sync.Mutex
}

// newDiscoveryObserver initializes and registers a new discoveryObserver.
func newDiscoveryObserver(ctx context.Context, adapterClient *floss.AdapterClient) (*discoveryObserver, error) {
	o := &discoveryObserver{
		ctx:                   ctx,
		adapterClient:         adapterClient,
		discovering:           false,
		foundDevicesByAddress: map[string]*floss.BluetoothDevice{},
	}
	if err := o.adapterClient.RegisterCallbackObserver(ctx, discoveryObserverName, o); err != nil {
		return nil, errors.Wrapf(err, "failed to register callback observer %q", discoveryObserverName)
	}
	return o, nil
}

// Close will unregister the observer, making it no longer usable. Call when
// the observer is no longer needed.
func (o *discoveryObserver) Close(ctx context.Context) error {
	if err := o.adapterClient.UnregisterCallbackObserver(ctx, discoveryObserverName); err != nil {
		return errors.Wrapf(err, "failed to register callback observer %q", discoveryObserverName)
	}
	return nil
}

// OnAddressChanged implements BluetoothCallbackObserver.OnAddressChanged.
func (o *discoveryObserver) OnAddressChanged(address string) error {
	// No-op.
	return nil
}

// OnSspRequest implements BluetoothCallbackObserver.OnSspRequest.
func (o *discoveryObserver) OnSspRequest(remoteDevice *floss.BluetoothDevice, classOfDevice uint32, variant floss.BtSspVariant, passkey uint32) error {
	// No-op.
	return nil
}

// OnBondStateChanged implements BluetoothCallbackObserver.OnBondStateChanged.
func (o *discoveryObserver) OnBondStateChanged(status floss.BtStatus, deviceAddress string, state floss.BtBondState) error {
	// No-op.
	return nil
}

// OnDeviceFound implements BluetoothCallbackObserver.OnDeviceFound.
func (o *discoveryObserver) OnDeviceFound(remoteDevice *floss.BluetoothDevice) error {
	o.stateMutex.Lock()
	defer o.stateMutex.Unlock()
	if _, ok := o.foundDevicesByAddress[remoteDevice.Address]; !ok {
		testing.ContextLogf(o.ctx, "Floss discovery observer found new device with address %q", remoteDevice.Address)
	}
	o.foundDevicesByAddress[remoteDevice.Address] = remoteDevice
	return nil
}

// OnDiscoveringChanged implements
// BluetoothCallbackObserver.OnDiscoveringChanged.
func (o *discoveryObserver) OnDiscoveringChanged(discovering bool) error {
	o.stateMutex.Lock()
	defer o.stateMutex.Unlock()
	if o.discovering != discovering {
		testing.ContextLogf(o.ctx, "Floss bluetooth adapter discovering state changed to %t", discovering)
	}
	o.discovering = discovering
	return nil
}

// FoundDeviceByAddress returns the corresponding device if one has been found
// with a matching address, or nil if no device matches.
func (o *discoveryObserver) FoundDeviceByAddress(address string) *floss.BluetoothDevice {
	o.stateMutex.Lock()
	defer o.stateMutex.Unlock()
	var matchingFoundDevice *floss.BluetoothDevice
	device, ok := o.foundDevicesByAddress[address]
	if ok {
		matchingFoundDevice = device
	}
	return matchingFoundDevice
}

// FoundDeviceByName returns the corresponding device if one has been found with
// a matching name, or nil if no device matches.
func (o *discoveryObserver) FoundDeviceByName(name string) *floss.BluetoothDevice {
	o.stateMutex.Lock()
	defer o.stateMutex.Unlock()
	var foundDevice *floss.BluetoothDevice
	for _, device := range o.foundDevicesByAddress {
		if device.Name == name {
			foundDevice = device
			break
		}
	}
	return foundDevice
}
