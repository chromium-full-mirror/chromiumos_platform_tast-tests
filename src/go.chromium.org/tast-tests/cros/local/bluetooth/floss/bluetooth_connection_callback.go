// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import "github.com/godbus/dbus/v5"

// BluetoothConnectionCallbackObserver describes callbacks for the Device Connection
// interface.
//
// Implement this to observe these callbacks when exporting callbacks via
// AdapterClient.RegisterConnectionCallbackObserver.
type BluetoothConnectionCallbackObserver interface {
	// OnDeviceConnected is called when a device has completed HCI connection.
	OnDeviceConnected(remoteDevice *BluetoothDevice) error

	// OnDeviceDisconnected is called when a device has completed HCI
	// disconnection.
	OnDeviceDisconnected(remoteDevice *BluetoothDevice) error
}

// exportedBluetoothConnectionCallback is meant to be used as the single export
// of the BluetoothConnectionCallback interface, with each method being directed
// to BluetoothConnectionCallbackObserver implementations.
type exportedBluetoothConnectionCallback struct {
	observers   map[string]BluetoothConnectionCallbackObserver
	dbusObjPath dbus.ObjectPath
	callbackID  uint32
}

// newExportedBluetoothConnectionCallback initialized a new exportedBluetoothConnectionCallback.
func newExportedBluetoothConnectionCallback(adapterHCI int) *exportedBluetoothConnectionCallback {
	return &exportedBluetoothConnectionCallback{
		observers:   map[string]BluetoothConnectionCallbackObserver{},
		dbusObjPath: buildAdapterSpecificObjectPath(adapterHCI, flossBluetoothConnectionCallbackObjectPathSuffix),
	}
}

// DBusObjectPath returns the dbus object path set for this object.
func (e *exportedBluetoothConnectionCallback) DBusObjectPath() dbus.ObjectPath {
	return e.dbusObjPath
}

// SetCallbackID sets the connection callback ID.
func (e *exportedBluetoothConnectionCallback) SetCallbackID(callbackID uint32) {
	e.callbackID = callbackID
}

// CallbackID returns the stored connection callback ID.
func (e *exportedBluetoothConnectionCallback) CallbackID() uint32 {
	return e.callbackID
}

// AddObserver adds a named observer. If an observer already exists with the
// same name, it will be replaced.
func (e *exportedBluetoothConnectionCallback) AddObserver(name string, observer BluetoothConnectionCallbackObserver) {
	e.observers[name] = observer
}

// RemoveObserver removes a named observer. If an observer does not exist with
// the provided name, no observer will be removed nor will an error occur.
func (e *exportedBluetoothConnectionCallback) RemoveObserver(name string) {
	delete(e.observers, name)
}

// HasObservers returns true if there is at least one observer.
func (e *exportedBluetoothConnectionCallback) HasObservers() bool {
	return len(e.observers) > 0
}

// Export will export the event handler methods to D-Bus.
func (e *exportedBluetoothConnectionCallback) Export(dbusConn *dbus.Conn) error {
	return dbusConn.ExportMethodTable(
		map[string]interface{}{
			"OnDeviceConnected":    e.onDeviceConnected,
			"OnDeviceDisconnected": e.onDeviceDisconnected,
		},
		e.dbusObjPath,
		flossBluetoothConnectionCallbackInterface,
	)
}

// Unexport will clear the exported the event handler methods from D-Bus.
func (e *exportedBluetoothConnectionCallback) Unexport(dbusConn *dbus.Conn) error {
	return dbusConn.Export(nil, e.dbusObjPath, flossBluetoothConnectionCallbackInterface)
}

// onDeviceConnected calls BluetoothCallbackObserver.onDeviceConnected for
// each observer.
func (e *exportedBluetoothConnectionCallback) onDeviceConnected(dbusRemoteDevice map[string]dbus.Variant) *dbus.Error {
	var firstErr *error
	remoteDevice, err := unmarshallBluetoothDevice(dbusRemoteDevice)
	if err != nil {
		firstErr = &err
	} else {
		for _, observer := range e.observers {
			if err := observer.OnDeviceConnected(remoteDevice); err != nil && firstErr == nil {
				firstErr = &err
			}
		}
	}
	if firstErr != nil {
		return dbus.MakeFailedError(*firstErr)
	}
	return nil
}

// onDeviceDisconnected calls BluetoothCallbackObserver.onDeviceDisconnected for
// each observer.
func (e *exportedBluetoothConnectionCallback) onDeviceDisconnected(dbusRemoteDevice map[string]dbus.Variant) *dbus.Error {
	var firstErr *error
	remoteDevice, err := unmarshallBluetoothDevice(dbusRemoteDevice)
	if err != nil {
		firstErr = &err
	} else {
		for _, observer := range e.observers {
			if err := observer.OnDeviceDisconnected(remoteDevice); err != nil && firstErr == nil {
				firstErr = &err
			}
		}
	}
	if firstErr != nil {
		return dbus.MakeFailedError(*firstErr)
	}
	return nil
}
