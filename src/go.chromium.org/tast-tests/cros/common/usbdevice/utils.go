// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbdevice

import (
	"bytes"
	"encoding"
	"encoding/binary"
	"io"
	"unicode/utf16"

	"go.chromium.org/tast/core/errors"
)

// sizeOf returns the size of the passed type.
func sizeOf[T any]() uint8 {
	var val T
	buffer := bytes.Buffer{}
	binary.Write(&buffer, binary.BigEndian, &val)
	return uint8(buffer.Len())
}

// ReadLE reads a struct using LittleEndian byte order.
func ReadLE[T any](reader io.Reader) (T, error) {
	var value T
	err := binary.Read(reader, binary.LittleEndian, &value)
	return value, err
}

// WriteLE writes a struct using LittleEndian byte order.
func WriteLE[T any](writer io.Writer, val T) error {
	bytesToWrite, err := ToLE(val)
	if err != nil {
		return err
	}
	return Write(writer, bytesToWrite)
}

// ToLE serializes the data using LittleEndian byte order.
func ToLE[T any](val T) ([]byte, error) {
	return toEndian(val, binary.LittleEndian)
}

// ReadBE reads a struct using BigEndian byte order.
func ReadBE[T any](reader io.Reader) (T, error) {
	var value T
	err := binary.Read(reader, binary.BigEndian, &value)
	return value, err
}

// WriteBE writes a struct using BigEndian byte order.
func WriteBE[T any](writer io.Writer, val T) error {
	bytesToWrite, err := ToBE(val)
	if err != nil {
		return err
	}
	return Write(writer, bytesToWrite)
}

// ToBE serializes the data using BigEndian byte order.
func ToBE[T any](val T) ([]byte, error) {
	return toEndian(val, binary.BigEndian)
}

// toEndian serializes the data using passed byte order, it respects BinaryMarshaler interface.
func toEndian[T any](val T, order binary.ByteOrder) ([]byte, error) {
	var bytesToWrite []byte
	var err error
	if binaryMarshallerInstance, ok := any(val).(encoding.BinaryMarshaler); ok {
		bytesToWrite, err = binaryMarshallerInstance.MarshalBinary()
	} else {
		buffer := bytes.Buffer{}
		err = binary.Write(&buffer, order, val)
		bytesToWrite = buffer.Bytes()
	}
	return bytesToWrite, err
}

// Write writes bytes into passed writer.
func Write(writer io.Writer, data []byte) error {
	_, err := writer.Write(data)
	return err
}

// utf16encode encodes a string in utf16.
func utf16encode(message string) ([]byte, error) {
	buffer := bytes.Buffer{}
	for _, val := range utf16.Encode([]rune(message)) {
		if err := binary.Write(&buffer, binary.LittleEndian, val); err != nil {
			return nil, errors.Wrapf(err, "unable to serialize %c", val)
		}
	}
	return buffer.Bytes(), nil
}
