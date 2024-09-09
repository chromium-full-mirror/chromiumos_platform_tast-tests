// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package amber

import (
	"encoding/binary"
	"math"

	"go.chromium.org/tast/core/errors"
)

// armDataType represents the type of the value associated with a armData object.
type armDataType = string

const (
	typeUInt8   armDataType = "uint8"
	typeUInt16  armDataType = "uint16"
	typeUInt32  armDataType = "uint32"
	typeFloat32 armDataType = "float32"
)

// Convenient constants to convert the length of the bits to byte.
const (
	bytes8  = 1
	bytes16 = 2
	bytes32 = 4
)

// armData is the fundamental structure of the robotic arm data.
type armData struct {
	name     string
	value    any
	dataType armDataType
}

func (ad *armData) setValue(val any) error {
	var ok bool
	switch ad.dataType {
	case typeUInt8:
		_, ok = val.(uint8)
	case typeUInt16:
		_, ok = val.(uint16)
	case typeUInt32:
		_, ok = val.(uint32)
	case typeFloat32:
		_, ok = val.(float32)
	default:
		return errors.Errorf("unsupported data type: %s", ad.dataType)
	}
	if !ok {
		return errors.Errorf("unexpected value type: got: %T, want %s: ", val, ad.dataType)
	}
	ad.value = val
	return nil
}

func (ad *armData) readFromBytes(dataBytes []byte, start int) (end int, err error) {
	readFunc := func([]byte) (any, error) { return nil, nil }
	switch ad.dataType {
	case typeUInt8:
		readFunc = readUint8
		end = start + bytes8
	case typeUInt16:
		readFunc = readUint16
		end = start + bytes16
	case typeUInt32:
		readFunc = readUint32
		end = start + bytes32
	case typeFloat32:
		readFunc = readFloat32
		end = start + bytes32
	default:
		return 0, errors.Errorf("unsupported data type: %s", ad.dataType)
	}

	if end > len(dataBytes) {
		return 0, errors.Errorf("too much units for the response data: got %d, want %d", end, len(dataBytes))
	}
	val, err := readFunc(dataBytes[start:end])
	if err != nil {
		return 0, errors.Wrapf(err, "failed to read %s", ad.name)
	}
	ad.value = val
	return end, nil
}

func readUint8(data []byte) (any, error) {
	if len(data) != bytes8 {
		return 0, errors.Errorf("unexpected data length: got %d, want %d", len(data), bytes8)
	}
	// byte is an alias for uint8. Directly return the value.
	return data[0], nil
}

func readUint16(data []byte) (any, error) {
	if len(data) != bytes16 {
		return 0, errors.Errorf("unexpected data length: got %d, want %d", len(data), bytes16)
	}
	return binary.LittleEndian.Uint16(data), nil
}

func readUint32(data []byte) (any, error) {
	if len(data) != bytes32 {
		return 0, errors.Errorf("unexpected data length: got %d, want %d", len(data), bytes32)
	}
	return binary.LittleEndian.Uint32(data), nil
}

func readFloat32(data []byte) (any, error) {
	bits, err := readUint32(data)
	if err != nil {
		return 0, err
	}
	bitsType32, _ := bits.(uint32)
	return math.Float32frombits(bitsType32), nil
}

// setValuesToDataSlice sets the values to the data slice.
// It returns error if the length of the data does not match the length of the values.
func setValuesToDataSlice(data []*armData, values []any) error {
	if len(data) != len(values) {
		return errors.Errorf("unexpected value count: got %d, want %d", len(values), len(data))
	}
	for i, d := range data {
		if err := d.setValue(values[i]); err != nil {
			return errors.Wrapf(err, "failed to set value for %s", d.name)
		}
	}
	return nil
}

// newCommonData returns the common data shared by all commands and responses.
func newCommonData() []*armData {
	return []*armData{
		{
			name:     "CmdNo",
			dataType: typeUInt16,
		},
		{
			name:     "Length",
			dataType: typeUInt16,
		},
		{
			name:     "Counter",
			dataType: typeUInt32,
		},
	}
}

// newJointPosData returns the position data shared by movement and status commands and responses.
func newJointPosData() []*armData {
	return []*armData{
		{
			name:     "Joint1Pos",
			dataType: typeFloat32,
		},
		{
			name:     "Joint2Pos",
			dataType: typeFloat32,
		},
		{
			name:     "Joint3Pos",
			dataType: typeFloat32,
		},
		{
			name:     "Joint4Pos",
			dataType: typeFloat32,
		},
		{
			name:     "Joint5Pos",
			dataType: typeFloat32,
		},
		{
			name:     "Joint6Pos",
			dataType: typeFloat32,
		},
		{
			name:     "Joint7Pos",
			dataType: typeFloat32,
		},
		{
			name:     "GripperPos",
			dataType: typeFloat32,
		},
	}
}

// newJointSpeedData returns the speed data of the status response.
func newJointSpeedData() []*armData {
	return []*armData{
		{
			name:     "Joint1Speed",
			dataType: typeFloat32,
		},
		{
			name:     "Joint2Speed",
			dataType: typeFloat32,
		},
		{
			name:     "Joint3Speed",
			dataType: typeFloat32,
		},
		{
			name:     "Joint4Speed",
			dataType: typeFloat32,
		},
		{
			name:     "Joint5Speed",
			dataType: typeFloat32,
		},
		{
			name:     "Joint6Speed",
			dataType: typeFloat32,
		},
		{
			name:     "Joint7Speed",
			dataType: typeFloat32,
		},
		{
			name:     "GripperSpeed",
			dataType: typeFloat32,
		},
	}
}

// newCartesianPosData returns the cartesian position data of the status response.
func newCartesianPosData() []*armData {
	return []*armData{
		{
			name:     "XPos",
			dataType: typeFloat32,
		},
		{
			name:     "YPos",
			dataType: typeFloat32,
		},
		{
			name:     "ZPos",
			dataType: typeFloat32,
		},
		{
			name:     "RollPos",
			dataType: typeFloat32,
		},
		{
			name:     "PitchPos",
			dataType: typeFloat32,
		},
		{
			name:     "YawPos",
			dataType: typeFloat32,
		},
	}
}

// newCartesianSpeedData returns the cartesian speed data of the status response.
func newCartesianSpeedData() []*armData {
	return []*armData{
		{
			name:     "XSpeed",
			dataType: typeFloat32,
		},
		{
			name:     "YSpeed",
			dataType: typeFloat32,
		},
		{
			name:     "ZSpeed",
			dataType: typeFloat32,
		},
		{
			name:     "RollSpeed",
			dataType: typeFloat32,
		},
		{
			name:     "PitchSpeed",
			dataType: typeFloat32,
		},
		{
			name:     "YawSpeed",
			dataType: typeFloat32,
		},
	}
}

// newArmAngleData returns the arm angle data of the cartesian control command and the status response.
func newArmAngleData() []*armData {
	return []*armData{
		{
			name:     "ArmAngle",
			dataType: typeFloat32,
		},
	}
}

// newTimeData returns the time data of the movement commands.
func newTimeData() []*armData {
	return []*armData{
		{
			name:     "Time",
			dataType: typeFloat32,
		},
	}
}

// newRespondData returns the respond data shared by multiple response.
func newRespondData() []*armData {
	return []*armData{
		{
			name:     "Respond",
			dataType: typeUInt8,
		},
	}
}
