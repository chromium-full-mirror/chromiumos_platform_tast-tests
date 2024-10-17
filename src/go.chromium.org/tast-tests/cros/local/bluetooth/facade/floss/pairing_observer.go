// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/local/bluetooth/floss"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type pairingObserver struct {
	ctx                        context.Context
	adapterClient              *floss.AdapterClient
	observerName               string
	targetDeviceAddress        string
	targetDeviceBondState      floss.BtBondState
	targetDeviceBondStatus     floss.BtStatus
	targetDeviceConnected      bool
	stateMutex                 sync.Mutex
	checkForBondedAndConnected chan interface{}
}

// newPairingObserver initializes and registers a new pairingObserver.
func newPairingObserver(ctx context.Context, adapterClient *floss.AdapterClient, address string) (*pairingObserver, error) {
	o := &pairingObserver{
		ctx:                        ctx,
		adapterClient:              adapterClient,
		observerName:               "PairingObserver_" + address,
		targetDeviceAddress:        address,
		checkForBondedAndConnected: make(chan interface{}),
		targetDeviceBondStatus:     floss.BtStatusSuccess,
	}
	if err := o.adapterClient.RegisterCallbackObserver(ctx, o.observerName, o); err != nil {
		return nil, errors.Wrapf(err, "failed to register callback observer %q", o.observerName)
	}
	if err := o.adapterClient.RegisterConnectionCallbackObserver(ctx, o.observerName, o); err != nil {
		return nil, errors.Wrapf(err, "failed to register connection callback observer %q", o.observerName)
	}
	return o, nil
}

// Close will unregister the observer, making it no longer usable. Call when
// the observer is no longer needed.
func (o *pairingObserver) Close(ctx context.Context) error {
	if err := o.adapterClient.UnregisterCallbackObserver(ctx, o.observerName); err != nil {
		return errors.Wrapf(err, "failed to register callback observer %q", o.observerName)
	}
	if err := o.adapterClient.UnregisterConnectionCallbackObserver(ctx, o.observerName); err != nil {
		return errors.Wrapf(err, "failed to register connection callback observer %q", o.observerName)
	}
	return nil
}

// OnAddressChanged implements BluetoothCallbackObserver.OnAddressChanged.
func (o *pairingObserver) OnAddressChanged(address string) error {
	// No-op.
	return nil
}

// OnDeviceFound implements BluetoothCallbackObserver.OnDeviceFound.
func (o *pairingObserver) OnDeviceFound(remoteDevice *floss.BluetoothDevice) error {
	// No-op.
	return nil
}

// OnDiscoveringChanged implements BluetoothCallbackObserver.OnDiscoveringChanged.
func (o *pairingObserver) OnDiscoveringChanged(discovering bool) error {
	// No-op.
	return nil
}

// OnSspRequest implements BluetoothCallbackObserver.OnSspRequest.
// No-op if the request is not for this observer's target device.
// Logs the SSP request and also approves consent requests.
func (o *pairingObserver) OnSspRequest(remoteDevice *floss.BluetoothDevice, classOfDevice uint32, variant floss.BtSspVariant, passkey uint32) error {
	if remoteDevice.Address != o.targetDeviceAddress {
		return nil
	}
	testing.ContextLogf(
		o.ctx,
		"Floss bluetooth adapter SSP request observed for target device: remoteDevice=%s, classOfDevice=%d, variant=%d, passkey=%d",
		remoteDevice.String(),
		classOfDevice,
		variant,
		passkey,
	)
	if variant == floss.BtSspVariantPasskeyConfirmation || variant == floss.BtSspVariantConsent {
		device := &floss.BluetoothDevice{Address: o.targetDeviceAddress}
		if err := o.adapterClient.SetPairingConfirmation(o.ctx, device, true); err != nil {
			return err
		}
	}
	return nil
}

// OnBondStateChanged implements BluetoothCallbackObserver.OnBondStateChanged.
func (o *pairingObserver) OnBondStateChanged(status floss.BtStatus, deviceAddress string, state floss.BtBondState) error {
	if deviceAddress == o.targetDeviceAddress {
		o.stateMutex.Lock()
		if o.targetDeviceBondState != state || o.targetDeviceBondStatus != status {
			testing.ContextLogf(
				o.ctx,
				"Floss bluetooth adapter bond state changed for target device: status=%d, deviceAddress=%s, state=%d",
				status,
				deviceAddress,
				state,
			)
		}
		o.targetDeviceBondStatus = status
		o.targetDeviceBondState = state
		o.stateMutex.Unlock()
		o.checkForBondedAndConnected <- nil
	}
	return nil
}

// OnDeviceConnected implements
// BluetoothConnectionCallbackObserver.OnDeviceConnected.
func (o *pairingObserver) OnDeviceConnected(remoteDevice *floss.BluetoothDevice) error {
	if remoteDevice.Address == o.targetDeviceAddress {
		o.stateMutex.Lock()
		if !o.targetDeviceConnected {
			testing.ContextLogf(
				o.ctx,
				"Floss bluetooth adapter connected to target device: remoteDevice=%s",
				remoteDevice.String(),
			)
			o.targetDeviceConnected = true
		}
		o.stateMutex.Unlock()
		o.checkForBondedAndConnected <- nil
	}
	return nil
}

// OnDeviceDisconnected implements
// BluetoothConnectionCallbackObserver.OnDeviceDisconnected.
func (o *pairingObserver) OnDeviceDisconnected(remoteDevice *floss.BluetoothDevice) error {
	if remoteDevice.Address == o.targetDeviceAddress {
		o.stateMutex.Lock()
		if o.targetDeviceConnected {
			testing.ContextLogf(
				o.ctx,
				"Floss bluetooth adapter disconnected from target device: remoteDevice=%s",
				remoteDevice.String(),
			)
		}
		o.targetDeviceConnected = false
		o.stateMutex.Unlock()
	}
	return nil
}

// SetTargetDeviceBondState sets the stored bond state.
func (o *pairingObserver) SetTargetDeviceBondState(state floss.BtBondState) {
	o.stateMutex.Lock()
	defer o.stateMutex.Unlock()
	o.targetDeviceBondState = state
}

// SetTargetDeviceConnected sets the stored connected state.
func (o *pairingObserver) SetTargetDeviceConnected(connected bool) {
	o.stateMutex.Lock()
	defer o.stateMutex.Unlock()
	o.targetDeviceConnected = connected
}

// IsBondedAndConnected returns true if the target device is known to be
// bonded and connected.
func (o *pairingObserver) IsBondedAndConnected() bool {
	o.stateMutex.Lock()
	defer o.stateMutex.Unlock()
	bondedAndConnected := (o.targetDeviceBondState == floss.BtBondStateBonded) && o.targetDeviceConnected
	return bondedAndConnected
}

// WaitForBondedAndConnected will block until IsBondedAndConnected returns
// true or the timeout is reached. A non-nil error will be returned if the
// latter occurs or if the bonding failed.
func (o *pairingObserver) WaitForBondedAndConnected(timeout time.Duration) error {
	if o.IsBondedAndConnected() {
		return nil
	}
	waitCtx, cancelWaitCtx := context.WithTimeout(o.ctx, timeout)
	var err error
	for {
		select {
		case <-waitCtx.Done():
			err = waitCtx.Err()
		case <-o.checkForBondedAndConnected:
		}
		if o.IsBondedAndConnected() {
			break
		}
		if o.targetDeviceBondStatus != floss.BtStatusSuccess {
			err = errors.Errorf("bonding failed with status %d", o.targetDeviceBondStatus)
			break
		}
	}
	cancelWaitCtx()
	return err
}
