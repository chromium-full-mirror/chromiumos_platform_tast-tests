// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"bytes"
	"encoding/binary"
)

// See https://www.kernel.org/doc/html/latest/usb/usbip_protocol.html for more details
// about the constants defined in this file.

// usbipVersion describes USBIP usbipVersion.
type usbipVersion uint16

// opCommand describes USBIP op command code.
type opCommand uint16

// command describes USBIP command code.
type command uint32

// status describes USBIP status code.
type status uint32

// direction describes USBIP message direction.
type direction uint32

const (
	serverPort int          = 3240
	version    usbipVersion = 0x0111

	commandOpReqDevlist opCommand = 0x8005
	commandOpRepDevlist opCommand = 0x0005
	commandOpReqImport  opCommand = 0x8003
	commandOpRepImport  opCommand = 0x0003

	commandSubmit    command = 0x1
	commandUnlink    command = 0x2
	commandRetSubmit command = 0x3
	commandRetUnlink command = 0x4

	statusOk status = 0x0

	dirOut direction = 0x0
	dirIn  direction = 0x1
)

// opCommandHeader describes op command header.
type opCommandHeader struct {
	// USBIP version.
	Version usbipVersion
	// Command code.
	CommandCode opCommand
	// Status is unused, shall be set to 0.
	Status status
}

// opRepDevlist describes OP_REP_DEVLIST message.
type opRepDevlist struct {
	// Op command header.
	Header opCommandHeader
	// Number of devices exported.
	NumDevices uint32
	// List of exported device summaries.
	Devices []DeviceSummary
}

// MarshalBinary marshalls UsbIPOpRepDevlist into binary form.
// Need an explicit marshalling because the number of devices is not fixed.
func (dl opRepDevlist) MarshalBinary() (data []byte, err error) {
	var bs bytes.Buffer
	binary.Write(&bs, binary.BigEndian, dl.Header)
	binary.Write(&bs, binary.BigEndian, dl.NumDevices)
	binary.Write(&bs, binary.BigEndian, dl.Devices)
	return bs.Bytes(), nil
}

// DeviceSummaryHeader describes device summary header used in OP_REP_DEVLIST message.
type DeviceSummaryHeader struct {
	// Path of the device on the host.
	Path [256]byte
	// Bus ID of the device on the host.
	BusID [32]byte
	// Bus number of the device on the host.
	Busnum uint32
	// Device number of the device on the host.
	Devnum uint32
	// Speed of the exported device.
	Speed uint32
	// IDVendor of the exported device.
	IDVendor uint16
	// IDProduct of the exported device.
	IDProduct uint16
	// BcdDevice of the exported device.
	BcdDevice uint16
	// BDeviceClass of the exported device.
	BDeviceClass uint8
	// BDeviceSubclass of the exported device.
	BDeviceSubclass uint8
	// BDeviceProtocol of the exported device.
	BDeviceProtocol uint8
	// BConfigurationValue of the exported device.
	BConfigurationValue uint8
	// BNumConfigurations of the exported device.
	BNumConfigurations uint8
	// BNumInterfaces of the exported device.
	BNumInterfaces uint8
}

// DeviceInterface describes device interface summary used in OP_REP_DEVLIST message.
type DeviceInterface struct {
	// BInterfaceClass of the exported device.
	BInterfaceClass uint8
	// BInterfaceSubclass of the exported device.
	BInterfaceSubclass uint8
	// BInterfaceProtocol of the exported device.
	BInterfaceProtocol uint8
	// Padding byte for alignment, shall be set to zero.
	Padding uint8
}

// DeviceSummary describes device summary used in OP_REP_DEVLIST message.
type DeviceSummary struct {
	// Device summary header.
	Header DeviceSummaryHeader
	// Device interface summary.
	// Currently only support a single interface.
	DeviceInterface DeviceInterface
}

// opRepImport describes OP_REP_IMPORT message.
type opRepImport struct {
	// Op command header.
	header opCommandHeader
	// Device summary header.
	device DeviceSummaryHeader
}

// messageHeader describes usbip_header_basic.
type messageHeader struct {
	Command command
	// Sequential number that identifies requests and corresponding responses; incremented per connection.
	SequenceNumber uint32
	// Specifies a remote USB device uniquely instead of busnum and devnum.
	DeviceID uint32
	// Direction, only used by client, for server this shall be 0.
	Direction direction
	// Endpoint number, only used by client, for server this shall be 0.
	Endpoint uint32
}

// commandSubmitBody describes USBIP_CMD_SUBMIT message.
type commandSubmitBody struct {
	// Transfer flags based on USBIP_URB transfer_flags.
	TransferFlags uint32
	// Transfer buffer length based on URB transfer_buffer_length.
	TransferBufferLength uint32
	// Initial frame for ISO transfer; shall be set to 0 if not ISO transfer.
	StartFrame uint32
	// Number of ISO packets; shall be set to 0xffffffff if not ISO transfer.
	NumberOfPackets uint32
	// Maximum time for the request on the server-side host controller.
	Interval uint32
	// Data bytes for USB setup, filled with zeros if not used.
	SetupBytes [8]byte
}

// returnSubmitBody describes USBIP_RET_SUBMIT message.
type returnSubmitBody struct {
	// Zero for successful URB transaction, otherwise some kind of error happened.
	Status uint32
	// Number of URB data bytes.
	ActualLength uint32
	// Initial frame for ISO transfer; shall be set to 0 if not ISO transfer.
	StartFrame uint32
	// Number of ISO packets; shall be set to 0xffffffff if not ISO transfer.
	NumberOfPackets uint32
	// Error count.
	ErrorCount uint32
	// Padding, shall be set to 0.
	Padding uint64
}

// commandUnlinkBody describes USBIP_CMD_UNLINK message.
type commandUnlinkBody struct {
	// Seqnum of the SUBMIT request to unlink.
	UnlinkSequenceNumber uint32
	// Padding, shall be set to 0.
	Padding [24]byte
}

// returnUnlinkBody describes USBIP_RET_UNLINK message.
type returnUnlinkBody struct {
	// Status is similar to the status of USBIP_RET_SUBMIT (share the same memory offset). When UNLINK is successful, status is -ECONNRESET; when USBIP_CMD_UNLINK is after USBIP_RET_SUBMIT status is 0.
	Status int32
	// Padding, shall be set to 0.
	Padding [24]byte
}
