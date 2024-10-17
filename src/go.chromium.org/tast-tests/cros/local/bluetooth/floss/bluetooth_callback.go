// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import (
	"github.com/godbus/dbus/v5"
)

// BluetoothCallbackObserver describes callbacks for the Adapter interface.
//
// Implement this to observe these callbacks when exporting callbacks via
// AdapterClient.RegisterCallbackObserver.
type BluetoothCallbackObserver interface {
	// OnAddressChanged is called when an adapter address is changed.
	OnAddressChanged(address string) error

	// OnDeviceFound is called when a device is found via discovery.
	OnDeviceFound(remoteDevice *BluetoothDevice) error

	// OnDiscoveringChanged is called when the discovering state has changed.
	OnDiscoveringChanged(discovering bool) error

	// OnSspRequest is called when a Simple secure pairing request is sent for
	// the agent to reply.
	//
	// remoteDevice is the address of the remote device that is being paired (TODO confirm).
	// classOfDevice is the class of device as described in HCI spec.
	// variant is the SSP variant.
	// passkey is the passkey to display so that the user can confirm or type it.
	OnSspRequest(remoteDevice *BluetoothDevice, classOfDevice uint32, variant BtSspVariant, passkey uint32) error

	// OnBondStateChanged is called when the bonding state has changed for a
	// device.
	//
	// status is the success (0) or failure reason for bonding.
	// deviceAddress is device address (BDADDR notification).
	// state is the new bonding state.
	OnBondStateChanged(status BtStatus, deviceAddress string, state BtBondState) error
}

// exportedBluetoothCallback is meant to be used as the single export of the
// BluetoothCallback interface, with each method being directed to
// BluetoothCallbackObserver implementations.
type exportedBluetoothCallback struct {
	observers   map[string]BluetoothCallbackObserver
	dbusObjPath dbus.ObjectPath
}

// newExportedBluetoothCallback initialized a new exportedBluetoothCallback.
func newExportedBluetoothCallback(adapterHCI int) *exportedBluetoothCallback {
	return &exportedBluetoothCallback{
		observers:   map[string]BluetoothCallbackObserver{},
		dbusObjPath: buildAdapterSpecificObjectPath(adapterHCI, flossBluetoothCallbackObjectPathSuffix),
	}
}

// DBusObjectPath returns the dbus object path set for this object.
func (e *exportedBluetoothCallback) DBusObjectPath() dbus.ObjectPath {
	return e.dbusObjPath
}

// AddObserver adds a named observer. If an observer already exists with the
// same name, it will be replaced.
func (e *exportedBluetoothCallback) AddObserver(name string, observer BluetoothCallbackObserver) {
	e.observers[name] = observer
}

// RemoveObserver removes a named observer. If an observer does not exist with
// the provided name, no observer will be removed nor will an error occur.
func (e *exportedBluetoothCallback) RemoveObserver(name string) {
	delete(e.observers, name)
}

// HasObservers returns true if there is at least one observer.
func (e *exportedBluetoothCallback) HasObservers() bool {
	return len(e.observers) > 0
}

// Export will export the event handler methods to D-Bus.
func (e *exportedBluetoothCallback) Export(dbusConn *dbus.Conn) error {
	return dbusConn.ExportMethodTable(
		map[string]interface{}{
			"OnAddressChanged":     e.onAddressChanged,
			"OnDeviceFound":        e.onDeviceFound,
			"OnDiscoveringChanged": e.onDiscoveringChanged,
			"OnSspRequest":         e.onSspRequest,
			"OnBondStateChanged":   e.onBondStateChanged,
		},
		e.dbusObjPath,
		flossBluetoothCallbackInterface,
	)
}

// Unexport will clear the exported the event handler methods from D-Bus.
func (e *exportedBluetoothCallback) Unexport(dbusConn *dbus.Conn) error {
	return dbusConn.Export(nil, e.dbusObjPath, flossBluetoothCallbackInterface)
}

// onAddressChanged calls BluetoothCallbackObserver.OnAddressChanged for each
// observer.
func (e *exportedBluetoothCallback) onAddressChanged(address string) *dbus.Error {
	var firstErr *error
	for _, observer := range e.observers {
		if err := observer.OnAddressChanged(address); err != nil && firstErr == nil {
			firstErr = &err
		}
	}
	if firstErr != nil {
		return dbus.MakeFailedError(*firstErr)
	}
	return nil
}

// onDeviceFound calls BluetoothCallbackObserver.OnDeviceFound for each
// observer.
func (e *exportedBluetoothCallback) onDeviceFound(dbusRemoteDevice map[string]dbus.Variant) *dbus.Error {
	var firstErr *error
	remoteDevice, err := unmarshallBluetoothDevice(dbusRemoteDevice)
	if err != nil {
		firstErr = &err
	} else {
		for _, observer := range e.observers {
			if err := observer.OnDeviceFound(remoteDevice); err != nil && firstErr == nil {
				firstErr = &err
			}
		}
	}
	if firstErr != nil {
		return dbus.MakeFailedError(*firstErr)
	}
	return nil
}

// onDiscoveringChanged calls BluetoothCallbackObserver.OnDiscoveringChanged for
// each observer.
func (e *exportedBluetoothCallback) onDiscoveringChanged(discovering bool) *dbus.Error {
	var firstErr *error
	for _, observer := range e.observers {
		if err := observer.OnDiscoveringChanged(discovering); err != nil && firstErr == nil {
			firstErr = &err
		}
	}
	if firstErr != nil {
		return dbus.MakeFailedError(*firstErr)
	}
	return nil
}

// onSspRequest calls BluetoothCallbackObserver.OnSspRequest for each observer.
func (e *exportedBluetoothCallback) onSspRequest(dbusRemoteDevice map[string]dbus.Variant, classOfDevice uint32, variant BtSspVariant, passkey uint32) *dbus.Error {
	var firstErr *error
	remoteDevice, err := unmarshallBluetoothDevice(dbusRemoteDevice)
	if err != nil {
		firstErr = &err
	} else {
		for _, observer := range e.observers {
			if err := observer.OnSspRequest(remoteDevice, classOfDevice, variant, passkey); err != nil && firstErr == nil {
				firstErr = &err
			}
		}
	}
	if firstErr != nil {
		return dbus.MakeFailedError(*firstErr)
	}
	return nil
}

// onBondStateChanged calls BluetoothCallbackObserver.OnBondStateChanged for
// each observer.
func (e *exportedBluetoothCallback) onBondStateChanged(status BtStatus, deviceAddress string, state BtBondState) *dbus.Error {
	var firstErr *error
	for _, observer := range e.observers {
		if err := observer.OnBondStateChanged(status, deviceAddress, state); err != nil && firstErr == nil {
			firstErr = &err
		}
	}
	if firstErr != nil {
		return dbus.MakeFailedError(*firstErr)
	}
	return nil
}
