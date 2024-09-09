// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package amber

import (
	"reflect"

	"go.chromium.org/tast/core/errors"
)

// generalResLength is the length of the general response.
// The general response contains the following component:
// - command number: 2 bytes
// - response length: 2 bytes
// - counter: 4 bytes
// - respond status: 1 byte
const generalResLength = 9

// generalOperationStatus is the general operation status shared by multiple commands.
// The |Respond| field indicates whether the operation is successful.
// 0 indicates the operation fails, 1 indicates the operation succeeds.
// The names of the fields should align with the name of the data defined in arm_data.go.
type generalOperationStatus struct {
	CmdNo   uint16
	Length  uint16
	Counter uint32
	Respond uint8
}

// newGeneralOperationStatus transforms a slice of armData into a generalOperationStatus object.
func newGeneralOperationStatus(dataSlice []*armData) (*generalOperationStatus, error) {
	status := &generalOperationStatus{}
	if err := parseStatus(dataSlice, status); err != nil {
		return nil, errors.Wrap(err, "failed to parse the general operation status")
	}
	return status, nil
}

// newGeneralResponse generates a new general response.
func newGeneralResponse() []*armData {
	var res []*armData
	resComponents := [][]*armData{
		newCommonData(), newRespondData(),
	}
	for _, data := range resComponents {
		res = append(res, data...)
	}
	return res
}

// parseStatus parses the values in |dataSlice| to the corresponding field of the |status| object.
func parseStatus(dataSlice []*armData, s any) error {
	for _, data := range dataSlice {
		field := reflect.ValueOf(s).Elem().FieldByName(data.name)
		if !field.CanSet() {
			return errors.Errorf("cannot set %s field for %+v", data.name, s)
		}
		value := reflect.ValueOf(data.value)
		field.Set(value)
	}
	return nil
}
