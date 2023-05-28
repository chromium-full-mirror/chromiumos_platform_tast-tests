// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package floss

import (
	"context"
	"encoding/hex"
	"fmt"
	"strings"

	"github.com/godbus/dbus/v5"

	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/errors"
)

// BluetoothDevice refers to a floss bluetooth device by its address and name.
// Note: Most methods only actually require and fill the address, leaving name
// as name unset.
type BluetoothDevice struct {
	Address string
	Name    string
}

func unmarshallBluetoothDevice(dbusDevice map[string]dbus.Variant) (*BluetoothDevice, error) {
	d := &BluetoothDevice{}
	address, ok := dbusDevice["address"]
	if !ok {
		return nil, errors.New("dbusDevice missing address field")
	}
	d.Address = address.Value().(string)
	name, ok := dbusDevice["name"]
	if !ok {
		return nil, errors.New("dbusDevice missing address field")
	}
	d.Name = name.Value().(string)
	return d, nil
}

// Marshall will return the BluetoothDevice in the format expected in floss
// D-Bus messages.
func (d *BluetoothDevice) Marshall() map[string]dbus.Variant {
	return map[string]dbus.Variant{
		"address": dbus.MakeVariant(d.Address),
		"name":    dbus.MakeVariant(d.Name),
	}
}

// String will return the BluetoothDevice as a string for logging purposes.
func (d *BluetoothDevice) String() string {
	return fmt.Sprintf(
		"BluetoothDevice{Address:%q,Name:%q}",
		d.Address,
		d.Name,
	)
}

func unmarshall128BitUUID(uuid []byte) (string, error) {
	if len(uuid) != 16 {
		return "", errors.Errorf("expected 16 bytes, got %d", len(uuid))
	}
	unmarshalledUUID := fmt.Sprintf(
		"%02x%02x%02x%02x-%02x%02x-%02x%02x-%02x%02x-%02x%02x%02x%02x%02x%02x",
		uuid[0], uuid[1], uuid[2], uuid[3], uuid[4], uuid[5], uuid[6], uuid[7],
		uuid[8], uuid[9], uuid[10], uuid[11], uuid[12], uuid[13], uuid[14], uuid[15],
	)
	return unmarshalledUUID, nil
}

func marshall128BitUUID(uuid string) ([]byte, error) {
	uuid = strings.ReplaceAll(uuid, "-", "")
	if len(uuid) != 32 {
		return nil, errors.Errorf("invalid 128 bit UUID %q", uuid)
	}
	marshalledUUID, err := hex.DecodeString(uuid)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to decode bytes from hex string %q", uuid)
	}
	return marshalledUUID, nil
}

func dbusCallFor128BitUUIDs(ctx context.Context, dbusObj *dbusutil.DBusObject, method string, args ...interface{}) ([]string, error) {
	c := dbusObj.Call(ctx, method, args...)
	if c.Err != nil {
		return nil, c.Err
	}
	var marshalled128BitUUIDs [][]byte
	if err := c.Store(&marshalled128BitUUIDs); err != nil {
		return nil, errors.Wrapf(err, "failed to store response of method %q as [][]byte", method)
	}
	var unmarshalledUUIDs []string
	for _, marshalledUUID := range marshalled128BitUUIDs {
		unmarshalledUUID, err := unmarshall128BitUUID(marshalledUUID)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to unmarshall UUIDs of response of method %q", method)
		}
		unmarshalledUUIDs = append(unmarshalledUUIDs, unmarshalledUUID)
	}
	return unmarshalledUUIDs, nil
}
