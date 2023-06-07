// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbdevice

import (
	"bytes"
	"encoding/binary"
	"unsafe"
)

// See https://www.usb.org/document-library/usb-20-specification for more details
// about the constants defined in this file.

// RequestCode describes a usb request code.
type RequestCode uint8

const (
	// RequestGetStatus is GET_STATUS USB standard request code.
	RequestGetStatus RequestCode = 0x00
	// RequestClearFeature is CLEAR_FEATURE USB standard request code.
	RequestClearFeature RequestCode = 0x01
	// RequestSetFeature is SET_FEATURE USB standard request code.
	RequestSetFeature RequestCode = 0x03
	// RequestSetAddress is SET_ADDRESS USB standard request code.
	RequestSetAddress RequestCode = 0x05
	// RequestGetDescriptor is GET_DESCRIPTOR USB standard request code.
	RequestGetDescriptor RequestCode = 0x06
	// RequestSetDescriptor is SET_DESCRIPTOR USB standard request code.
	RequestSetDescriptor RequestCode = 0x07
	// RequestGetConfiguration is GET_CONFIGURATION USB standard request code.
	RequestGetConfiguration RequestCode = 0x08
	// RequestSetConfiguration is SET_CONFIGURATION USB standard request code.
	RequestSetConfiguration RequestCode = 0x09
	// RequestGetInterface is GET_INTERFACE USB standard request code.
	RequestGetInterface RequestCode = 0x0A
	// RequestSetInterface is SET_INTERFACE USB standard request code.
	RequestSetInterface RequestCode = 0x0B
	// RequestSynchFrame is SYNCH_FRAME USB standard request code.
	RequestSynchFrame RequestCode = 0x0C
	// RequestSetSel is SET_SEL USB standard request code.
	RequestSetSel RequestCode = 0x30
	// RequestSetIsochDelay is SET_ISOCH_DELAY USB standard request code.
	RequestSetIsochDelay RequestCode = 0x31
)

// SetupPacket describes USB setup packet.
type SetupPacket struct {
	// Bitmap containing characteristics of request.
	BmRequestType uint8
	// Specific request type.
	BRequest RequestCode
	// Word-sized field that varies according to request.
	WValue uint16
	// Word-sized field that varies according to request; typically used to pass an index or offset.
	WIndex uint16
	// Number of bytes to transfer if there is a Data stage.
	WLength uint16
}

// Direction describes direction of SetupPacket.BmRequestType.
type Direction uint8

const (
	// HostToDevice represents host-to-device direction bit in SetupPacket.BmRequestType bitmap.
	HostToDevice Direction = 0x00
	// DeviceToHost represents device-to-host direction bit in SetupPacket.BmRequestType bitmap.
	DeviceToHost Direction = 0x01
)

// RequestClass describes request class part of SetupPacket.BmRequestType.
type RequestClass uint8

const (
	// RequestClassStandard represents Standard type bits in SetupPacket.BmRequestType bitmap.
	RequestClassStandard RequestClass = 0x00
	// RequestClassClass represents Class type bits in SetupPacket.BmRequestType bitmap.
	RequestClassClass RequestClass = 0x01
	// RequestClassVendor represents Vendor type bits in SetupPacket.BmRequestType bitmap.
	RequestClassVendor RequestClass = 0x02
	// RequestClassReserved represents Reserved type bits in SetupPacket.BmRequestType bitmap.
	RequestClassReserved RequestClass = 0x03
)

// RequestRecipient describes recipient part of SetupPacket.BmRequestType.
type RequestRecipient uint8

const (
	// RequestRecipientDevice represents Device recipient bits in SetupPacket.BmRequestType bitmap.
	RequestRecipientDevice RequestRecipient = 0x00
	// RequestRecipientInterface represents Interface recipient bits in SetupPacket.BmRequestType bitmap.
	RequestRecipientInterface RequestRecipient = 0x01
	// RequestRecipientEndpoint represents Endpoint recipient bits in SetupPacket.BmRequestType bitmap.
	RequestRecipientEndpoint RequestRecipient = 0x02
	// RequestRecipientOther represents Other recipient bits in SetupPacket.BmRequestType bitmap.
	RequestRecipientOther RequestRecipient = 0x03
)

// direction returns direction of SetupPacket.
func (setup *SetupPacket) direction() Direction {
	return Direction((setup.BmRequestType >> 7) & 1)
}

// requestClass returns request class of SetupPacket.
func (setup *SetupPacket) requestClass() RequestClass {
	return RequestClass((setup.BmRequestType >> 5) & 0b11)
}

// recipient returns recipient of SetupPacket.
func (setup *SetupPacket) recipient() RequestRecipient {
	return RequestRecipient(setup.BmRequestType & 0b1111)
}

// HidRequestType describes the type of a HID request, see https://www.usb.org/sites/default/files/hid1_11.pdf .
type HidRequestType uint8

const (
	// HidRequestGetReport represents GET_REPORT type of HID request.
	HidRequestGetReport HidRequestType = 0x01
	// HidRequestGetIdle represents GET_IDLE type of HID request.
	HidRequestGetIdle HidRequestType = 0x02
	// HidRequestGetProtocol represents GET_PROTOCOL type of HID request.
	HidRequestGetProtocol HidRequestType = 0x03
	// HidRequestGetDescriptor represents GET_DESCRIPTOR type of HID request.
	HidRequestGetDescriptor HidRequestType = 0x06
	// HidRequestSetDescriptor represents SET_DESCRIPTOR type of HID request.
	HidRequestSetDescriptor HidRequestType = 0x07
	// HidRequestSetReport represents SET_REPORT type of HID request.
	HidRequestSetReport HidRequestType = 0x09
	// HidRequestSetIdle represents SET_IDLE type of HID request.
	HidRequestSetIdle HidRequestType = 0x0A
	// HidRequestSetProtocol represents SET_PROTOCOL type of HID request.
	HidRequestSetProtocol HidRequestType = 0x0B
)

// DescriptorType describes the type of a USB descriptor.
type DescriptorType uint8

const (
	// DescriptorDevice represents DEVICE USB descriptor type.
	DescriptorDevice DescriptorType = 0x01
	// DescriptorConfiguration represents CONFIGURATION USB descriptor type.
	DescriptorConfiguration DescriptorType = 0x02
	// DescriptorString represents STRING USB descriptor type.
	DescriptorString DescriptorType = 0x03
	// DescriptorInterface represents INTERFACE USB descriptor type.
	DescriptorInterface DescriptorType = 0x04
	// DescriptorEndpoint represents ENDPOINT USB descriptor type.
	DescriptorEndpoint DescriptorType = 0x05
	// DescriptorDeviceQualifier represents DEVICE_QUALIFIER USB descriptor type.
	DescriptorDeviceQualifier DescriptorType = 0x06
	// DescriptorOtherSpeedConfiguration represents OTHER_SPEED_CONFIGURATION USB descriptor type.
	DescriptorOtherSpeedConfiguration DescriptorType = 0x07
	// DescriptorInterfacePower represents INTERFACE_POWER USB descriptor type.
	DescriptorInterfacePower DescriptorType = 0x08
	// DescriptorDebug represents DEBUG USB descriptor type.
	DescriptorDebug DescriptorType = 0x0a
	// DescriptorHid represents HID USB descriptor type.
	DescriptorHid DescriptorType = 0x21
	// DescriptorHidReport represents HID_REPORT USB descriptor type.
	DescriptorHidReport DescriptorType = 0x22
)

// StringDescriptor describes a string descriptor.
type StringDescriptor struct {
	// Size of the descriptor in bytes.
	BLength uint8
	// Type of descriptor.
	BDescriptorType DescriptorType
	// UNICODE encoded strings.
	BString []byte
}

// MarshalBinary marshalls StringDescriptorHeader into binary form.
// Need an explicit marshalling because BString length is not fixed.
func (h StringDescriptor) MarshalBinary() (data []byte, err error) {
	var bs bytes.Buffer
	length := uint8(unsafe.Sizeof(h.BLength)) + uint8(unsafe.Sizeof(h.BDescriptorType)) + uint8(len(h.BString))
	h.BLength = length
	binary.Write(&bs, binary.LittleEndian, h.BLength)
	binary.Write(&bs, binary.LittleEndian, h.BDescriptorType)
	binary.Write(&bs, binary.LittleEndian, h.BString)
	return bs.Bytes(), nil
}

const (
	// Version11 represents USB version 1.1.0 in Binary-Coded Decimal.
	Version11 = 0x0110
	// HidVersion111 represents hID version 1.1.1 in Binary-Coded Decimal.
	HidVersion111 = 0x0111

	// VendorIDGoogle represents google Vendor ID.
	VendorIDGoogle = 0x18D1
	// ProductIDGoogleKeyboard represents google Keyboard Product ID. This is a non-existing ID.
	ProductIDGoogleKeyboard = 0x0123 // TODO(b/283696258): introduce a separate ID and replace here
	// VendorIDLogitech represents logitech Vendor ID.
	VendorIDLogitech = 0x046D // TODO(b/283696258): remove, use Google ID instead
	// ProductIDLogitechMouse represents logitech Mouse Product ID.
	ProductIDLogitechMouse = 0xc077 // TODO(b/283696258): introduce a separate ID and replace here
	// BcdDeviceVersion1 represents device release number 1.0.0.
	BcdDeviceVersion1 = 0x0100

	// LangidEngUsa represents engUsa LangID.
	LangidEngUsa = 0x0409
	// ConfigAttrBase represents base bit in bmAttributes in Configuration descriptor.
	ConfigAttrBase = 0b10000000
	// ConfigAttrSelfPowered represents self-powered bit in bmAttributes in Configuration descriptor.
	ConfigAttrSelfPowered = 0b01000000
	// ConfigAttrRemoteWakeup represents remote wakeup bit in bmAttributes in Configuration descriptor.
	ConfigAttrRemoteWakeup = 0b00100000

	// EpTransferTypeControl represents control transfer type bit in bmAttributes in Endpoint descriptor.
	EpTransferTypeControl = 0b00000000
	// EpTransferTypeIsochronous represents isochronous transfer type bit in bmAttributes in Endpoint descriptor.
	EpTransferTypeIsochronous = 0b00000001
	// EpTransferTypeBulk represents bulk transfer type bit in bmAttributes in Endpoint descriptor.
	EpTransferTypeBulk = 0b00000010
	// EpTransferTypeInterrupt represents interrupt transfer type bit in bmAttributes in Endpoint descriptor.
	EpTransferTypeInterrupt = 0b00000011

	// EpDirIn represents in direction bit in BEndpointAddress in Endpoint descriptor.
	EpDirIn = 0b10000000
	// EpDirOut represents out direction bit in BEndpointAddress in Endpoint descriptor.
	EpDirOut = 0b0

	// InterfaceClassHid represents hid class code in Interface descriptor.
	InterfaceClassHid = 0x03
	// InterfaceSubclassBoot represents boot subclass code in Interface descriptor.
	InterfaceSubclassBoot = 0x01
	// InterfaceProtocolKeyboard represents keyboard interface protocol in Interface descriptor.
	InterfaceProtocolKeyboard = 0x01
	// InterfaceProtocolMouse represents mouse interface protocol in Interface descriptor.
	InterfaceProtocolMouse = 0x02
)

// DeviceDescriptor describes a device descriptor. It includes information that applies globally to the device and all of the device’s configurations.
type DeviceDescriptor struct {
	// Size of the descriptor.
	BLength uint8
	// Type of descriptor.
	BDescriptorType DescriptorType
	// USB specification number which device complies too.
	BcdUSB uint16
	// Class code (assigned by USB org).
	// If equal to zero, each interface specifies it’s own class code.
	// If equal to 0xFF, the class code is vendor specified.
	// Otherwise field is valid class code.
	BDeviceClass uint8
	// Subclass code (assigned by USB org).
	BDeviceSubclass uint8
	// Protocol code (assigned by USB org).
	BDeviceProtocol uint8
	// Maximum packet size for zero endpoint. Valid sizes are 8, 16, 32, 64.
	BMaxPacketSize uint8
	// Vendor ID (assigned by USB org).
	IDVendor uint16
	// Product ID (assigned by manufacturer).
	IDProduct uint16
	// Device release number in binary-coded decimal.
	BcdDevice uint16
	// Index of manufacturer string descriptor.
	IManufacturer uint8
	// Index of product string descriptor.
	IProduct uint8
	// Index of serial number string descriptor.
	ISerialNumber uint8
	// Number of possible configurations.
	BNumConfigurations uint8
}

// HidDescriptor describes a device HID descriptor.
type HidDescriptor struct {
	// Size of the descriptor.
	BLength uint8
	// Type of descriptor.
	BDescriptorType DescriptorType
	// HID version number.
	BcdHID uint16
	// Country code.
	BCountryCode uint8
	// Number of additional class specific descriptors.
	BNumDescriptors uint8
	// Descriptor type.
	BClassDescriptorType DescriptorType
	// Length of the descriptor.
	WReportDescriptorLength uint16
}

// EndpointDescriptor describes a device endpoint descriptor.
type EndpointDescriptor struct {
	// Size of Descriptor in Bytes.
	BLength uint8
	// Type of descriptor.
	BDescriptorType DescriptorType
	// Endpoint address.
	BEndpointAddress uint8
	// Bitmap of attributes.
	BmAttributes uint8
	// Maximum packet size this endpoint is capable of sending or receiving.
	WMaxPacketSize uint16
	// Interval for polling endpoint data transfers. Value in frame counts. Ignored for bulk & control endpoints. Isochronous must equal 1 and field may range from 1 to 255 for interrupt endpoints.
	BInterval uint8
}

// ConfigurationDescriptor describes a device configuration descriptor.
type ConfigurationDescriptor struct {
	// Size of Descriptor in bytes.
	BLength uint8
	// Type of descriptor.
	BDescriptorType DescriptorType
	// Total length in bytes of data returned.
	WTotalLength uint16
	// Number of interfaces.
	BNumInterfaces uint8
	// Value to use as an argument to select this configuration.
	BConfigurationValue uint8
	// Index of string descriptor describing this configuration.
	IConfiguration uint8
	// Bitmap of attributes.
	BmAttributes uint8
	// Maximum power consumption in 2mA units.
	BMaxPower uint8
}

// InterfaceDescriptor describes a device interface descriptor.
type InterfaceDescriptor struct {
	// Size of Descriptor in Bytes.
	BLength uint8
	// Type of descriptor.
	BDescriptorType DescriptorType
	// Number of interface.
	BInterfaceNumber uint8
	// Value used to select alternative setting.
	BAlternateSetting uint8
	// Number of endpoints used for this interface.
	BNumEndpoints uint8
	// Class code (assigned by USB org).
	BInterfaceClass uint8
	// Subclass code (assigned by USB org).
	BInterfaceSubclass uint8
	// Protocol code (assigned by USB org)
	BInterfaceProtocol uint8
	// Index of string descriptor describing this interface.
	IInterface uint8
}

const (
	// SpeedUnknown means The OS doesn't report or know the device speed.
	SpeedUnknown = 0
	// SpeedLow means the device is operating at low speed (1.5MBit/s).
	SpeedLow = 1
	// SpeedFull means the device is operating at full speed (12MBit/s).
	SpeedFull = 2
	// SpeedHigh means the device is operating at high speed (480MBit/s).
	SpeedHigh = 3
	// SpeedSuper means the device is operating at super speed (5000MBit/s).
	SpeedSuper = 4
	// SpeedSuperPlus means the device is operating at super speed plus (10000MBit/s).
	SpeedSuperPlus = 5
)

// DeviceHandler represents a device handler for USB messages.
type DeviceHandler interface {
	HandleUSBMessage(endpoint uint32, setupBytes [8]byte, transferBuffer []byte) error
}

// Device represents a USB device.
// Implementations are expected to provide descriptors and other metadata for the usb device,
// a handler to use for handling USB messages and a Data function to generate data for USB messages sent.
type Device interface {
	// LangID returns a LANGID (defined by the USB-IF) used by the device.
	LangID() uint16
	// DeviceDescriptor returns a device descriptor.
	DeviceDescriptor() DeviceDescriptor
	// HidDescriptor returns a HID descriptor.
	HidDescriptor() HidDescriptor
	// InterfaceDescriptor returns an interface descriptor.
	InterfaceDescriptor() InterfaceDescriptor
	// ConfigurationDescriptor returns a configuration descriptor.
	ConfigurationDescriptor() ConfigurationDescriptor
	// EndpointDescriptors returns an endpoint descriptors.
	EndpointDescriptors() []EndpointDescriptor
	// StringDescriptor returns a string descriptor.
	StringDescriptor(int) (string, error)
	// HidReport returns a HID report in case of a HID device.
	HidReportDescriptor() []byte
	// Data returns the value to return for a data USB message.
	Data() ([]byte, error)
	// GetHandler returns a handler that will handle USB communication for the device.
	GetHandler() DeviceHandler
}
