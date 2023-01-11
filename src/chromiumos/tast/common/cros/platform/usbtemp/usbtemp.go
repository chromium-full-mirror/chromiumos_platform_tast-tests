// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.
// To measure temperature using usbtemp.com PL2303GL sensor:
// 1. Connect a sensor to a DUT
// 2. Write code to interact with the sensor:
//     usbtemp, err := usbtemp.Open("/dev/ttyUSB0")
//     if err != nil {
//         s.Fatal("Failed to open usbtemp: ", err)
//     }
//
//     // If you want to have several usbtemp sensors plugged in
//     // use Rom() to distinguish between them (optional):
//     rom, err := usbtemp.Rom()
//     if err != nil {
//         s.Fatal("Failed to get serial from ROM: ", err)
//     }
//     testing.ContextLog(ctx, rom)
//
//     // This will return the temperature reading:
//     temperature, err := usbtemp.Temperature()
//     if err != nil {
//         s.Fatal("Failed to get temperature: ", err)
//     }
//     testing.ContextLog(ctx, "Temperature: ", temperature)

// Package usbtemp interacts with usbtemp.com thermometer to get temperature outside of the DUT enclosure.
package usbtemp

import (
	"context"
	"time"

	"github.com/4lon/crc8"
	"go.bug.st/serial"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// This implementation is an adaption of another script which can be found at https://github.com/usbtemp/usbtemp-python3/blob/754fbaa47fa454cac387f12182622f36f57b5757/usbtemp.py.

// UsbTemp exposes the interface to read the temperature and get the serial number.
type UsbTemp struct {
	port serial.Port
}

func checkSum(data []byte) byte {
	table := crc8.MakeTable(crc8.CRC8_MAXIM)
	crc := crc8.Checksum([]byte(data), table)
	return crc
}

// Open tries to open a usbtemp connected to a specified |portName| serial device.
func Open(portName string) (*UsbTemp, error) {
	mode := &serial.Mode{
		BaudRate: 9600,
	}
	usbPort, err := serial.Open(portName, mode)
	if err != nil {
		return nil, errors.Wrapf(err, "serial open error: %q", portName)
	}
	if err := usbPort.SetReadTimeout(time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to set read timeout")
	}

	return &UsbTemp{
		port: usbPort,
	}, nil
}

func (thermo *UsbTemp) clear() error {
	if err := thermo.port.ResetInputBuffer(); err != nil {
		return err
	}

	return thermo.port.ResetOutputBuffer()
}

func (thermo *UsbTemp) reset(ctx context.Context) error {
	if err := thermo.clear(); err != nil {
		return errors.Wrap(err, "serial clear error")
	}

	mode := &serial.Mode{
		BaudRate: 9600,
	}
	if err := thermo.port.SetMode(mode); err != nil {
		return errors.Wrap(err, "set mode")
	}

	_, err := thermo.port.Write([]byte("\xf0"))
	if err != nil {
		return errors.Wrap(err, "serial write error")
	}

	buff := make([]byte, 1)
	n, err := thermo.port.Read(buff)
	if err != nil {
		return errors.Wrap(err, "serial read error")
	}

	mode = &serial.Mode{
		BaudRate: 115200,
	}
	if err := thermo.port.SetMode(mode); err != nil {
		return errors.Wrap(err, "set mode")
	}

	if n != 1 {
		return errors.Errorf("unexpected records: got %d; want 1", n)
	} else if buff[0] == 0xf0 {
		return errors.New("no device present")
	} else if buff[0] == 0x00 {
		return errors.New("short circuit")
	} else if buff[0] < 0x10 || buff[0] > 0xe0 {
		return errors.Errorf("presence error: 0x%x", buff[0])
	}

	return nil
}

// readBytesRaw reads bytes one-by-one to ensure required number of bytes.
func (thermo *UsbTemp) readBytesRaw(n int) ([]byte, error) {
	result := make([]byte, n)
	for i := range result {
		buff := make([]byte, 1)
		n, err := thermo.port.Read(buff)
		if err != nil {
			return []byte{}, errors.Wrapf(err, "readBytesRaw %d", i)
		}
		if n != 1 {
			return []byte{}, errors.Wrapf(err, "unexpected EOF: %d", i)
		}
		result[i] = buff[0]
	}
	return result, nil
}

func (thermo *UsbTemp) writeByte(b byte) (byte, error) {
	buff := make([]byte, 8)
	for i := range buff {
		if (b & 0x01) != 0 {
			buff[i] = 0xff
		} else {
			buff[i] = 0x00
		}
		b >>= 1
	}

	if err := thermo.clear(); err != nil {
		return 0, errors.Wrap(err, "clear")
	}

	_, err := thermo.port.Write(buff)
	if err != nil {
		return 0, errors.Wrap(err, "serial write error")
	}

	buff, err = thermo.readBytesRaw(8)
	if err != nil {
		return 0, errors.Wrap(err, "writeByte: readBytesRaw")
	}

	result := byte(0)
	for i := range buff {
		result >>= 1
		if buff[i] == 0xff {
			result |= 0x80
		}
	}

	return result, nil
}

func (thermo *UsbTemp) write(b byte) error {
	result, err := thermo.writeByte(b)
	if err != nil {
		return errors.Wrap(err, "write")
	}
	if result != b {
		return errors.New("invalid response")
	}
	return nil
}

func (thermo *UsbTemp) read(ctx context.Context) (byte, error) {
	result, err := thermo.writeByte(0xff)
	if err != nil {
		return 0, errors.Wrap(err, "read byte")
	}
	return result, nil
}

func (thermo *UsbTemp) readBytes(ctx context.Context, n int) ([]byte, error) {
	buff := make([]byte, n)
	for i := range buff {
		val, err := thermo.read(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "readBytes")
		}
		buff[i] = val
	}
	return buff, nil
}

// Rom reads the serial number from ROM.
func (thermo *UsbTemp) Rom(ctx context.Context) ([]byte, error) {
	if err := thermo.reset(ctx); err != nil {
		return []byte{}, errors.Wrap(err, "rom")
	}
	if err := thermo.write(0x33); err != nil {
		return []byte{}, errors.Wrap(err, "rom")
	}
	data, err := thermo.readBytes(ctx, 8)
	if err != nil {
		return []byte{}, errors.Wrap(err, "rom")
	}
	if checkSum(data[0:7]) != data[7] {
		return []byte{}, errors.New("rom: CRC error")
	}
	return data, nil
}

// Temperature reads the current temperature.
func (thermo *UsbTemp) Temperature(ctx context.Context) (float64, error) {
	if err := thermo.reset(ctx); err != nil {
		return 0, errors.Wrap(err, "reset")
	}
	if err := thermo.write(0xcc); err != nil {
		return 0, errors.Wrap(err, "write")
	}
	if err := thermo.write(0x44); err != nil {
		return 0, errors.Wrap(err, "write")
	}
	if err := testing.Sleep(ctx, 1*time.Second); err != nil {
		return 0, errors.Wrap(err, "sleep")
	}
	if err := thermo.reset(ctx); err != nil {
		return 0, errors.Wrap(err, "reset")
	}
	if err := thermo.write(0xcc); err != nil {
		return 0, errors.Wrap(err, "write")
	}
	if err := thermo.write(0xbe); err != nil {
		return 0, errors.Wrap(err, "write")
	}

	data, err := thermo.readBytes(ctx, 9)
	if err != nil {
		return 0, errors.Wrap(err, "readBytes")
	}
	if checkSum(data[0:8]) != data[8] {
		return 0, errors.New("temperature: CRC error")
	}

	temperature := float64(int(data[1])<<8|int(data[0])) / 16.0
	return temperature, nil
}
