// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbdevice

import (
	"context"

	"go.chromium.org/tast/core/errors"
)

// UsbMouse represents a virtual mouse.
// See https://wiki.osdev.org/USB_Human_Interface_Devices#USB_mouse for more details.
type UsbMouse interface {
	// origin is in the top left, x - to the right, y - to the bottom
	UpdateState(deltaX, deltaY, buttonState byte) UsbMouse
}
type usbMouseImpl struct {
	moveQueue [][]byte
	handler   DeviceHandler
}

const (
	// MouseNoButton is a mouse state when no buttons are pressed.
	MouseNoButton = 0b00000000
	// MouseLeftButton is a mouse state when left button is pressed.
	MouseLeftButton = 0b00000001
	// MouseRightButton is a mouse state when right button is pressed.
	MouseRightButton = 0b00000010
	// MouseMiddleButton is a mouse state when middle button is pressed.
	MouseMiddleButton = 0b00000010
)
const (
	mouseStringManufacturer  = 1
	mouseStringProduct       = 2
	mouseStringSerialNumber  = 3
	mouseStringConfiguration = 4
	mouseStringInterface     = 5
)

var mouseStrings = map[int]string{
	mouseStringManufacturer:  "Google",
	mouseStringProduct:       "Virtual USB Mouse",
	mouseStringSerialNumber:  "No Serial Number",
	mouseStringConfiguration: "Default Configuration",
	mouseStringInterface:     "Default Interface",
}

// NewUsbMouse returns a new instance of UsbMouse.
// Mouse handler has to store context in order to perform a sleep.
// As a result, mouse instances must only be created inside of the test (not in a fixture).
func NewUsbMouse(ctx context.Context) *usbMouseImpl {
	handler := newHidDeviceHandler()
	m := usbMouseImpl{handler: handler}

	handler.withDevice(&m).withContext(ctx)
	return &m
}

func getNoop() []byte {
	return []byte{0x0, 0x0, 0x0, 0x0}
}

// UpdateState updates mouse coordinates relative to the current position and pressed buttons.
// These commands are added to the queue. This allows to queue multiple entries in a non-blocking way.
func (m *usbMouseImpl) UpdateState(deltaX, deltaY, buttonState byte) UsbMouse {
	data := []byte{buttonState, deltaX, deltaY, 0x00}
	m.moveQueue = append(
		m.moveQueue,
		data,
	)

	return m
}

// GetHandler returns a handler that will handle USB communication for the device.
func (m *usbMouseImpl) GetHandler() DeviceHandler {
	return m.handler
}

// Data returns the value to return for a data USB message.
func (m *usbMouseImpl) Data() ([]byte, error) {
	var data []byte
	if len(m.moveQueue) > 0 {
		data, m.moveQueue = m.moveQueue[0], m.moveQueue[1:]
		return data, nil
	}

	return getNoop(), nil
}

// HidReport returns a HID report.
// The data has been collected using usbmon from a real mouse.
func (m *usbMouseImpl) HidReportDescriptor() []byte {
	return []byte{
		// comments by http://eleccelerator.com/usbdescreqparser/
		0x05, 0x01, // Usage Page (Generic Desktop Ctrls)
		0x09, 0x02, // Usage (Mouse)
		0xA1, 0x01, // Collection (Application)
		0x09, 0x01, //   Usage (Pointer)
		0xA1, 0x00, //   Collection (Physical)
		0x05, 0x09, //     Usage Page (Button)
		0x19, 0x01, //     Usage Minimum (0x01)
		0x29, 0x03, //     Usage Maximum (0x03)
		0x15, 0x00, //     Logical Minimum (0)
		0x25, 0x01, //     Logical Maximum (1)
		0x95, 0x08, //     Report Count (8)
		0x75, 0x01, //     Report Size (1)
		0x81, 0x02, //     Input (Data,Var,Abs,No Wrap,Linear,Preferred State,No Null Position)
		0x05, 0x01, //     Usage Page (Generic Desktop Ctrls)
		0x09, 0x30, //     Usage (X)
		0x09, 0x31, //     Usage (Y)
		0x09, 0x38, //     Usage (Wheel)
		0x15, 0x81, //     Logical Minimum (-127)
		0x25, 0x7F, //     Logical Maximum (127)
		0x75, 0x08, //     Report Size (8)
		0x95, 0x03, //     Report Count (3)
		0x81, 0x06, //     Input (Data,Var,Rel,No Wrap,Linear,Preferred State,No Null Position)
		0xC0, //   End Collection
		0xC0, // End Collection
	}
}

// HidDescriptor returns a HID descriptor.
func (m *usbMouseImpl) HidDescriptor() HidDescriptor {
	return HidDescriptor{
		BLength:                 sizeOf[HidDescriptor](),
		BDescriptorType:         DescriptorHid,
		BcdHID:                  HidVersion111, // Using HID 1.1.1.
		BCountryCode:            0,             // 0 means hardware is not localized.
		BNumDescriptors:         1,             // There is 1 additional descriptor(HID report).
		BClassDescriptorType:    DescriptorHidReport,
		WReportDescriptorLength: uint16(len(m.HidReportDescriptor())),
	}
}

// LangID returns a LANGID (defined by the USB-IF) used by the device.
func (m *usbMouseImpl) LangID() uint16 {
	return LangidEngUsa // Provide strings in English only.
}

// DeviceDescriptor returns a device descriptor.
func (m *usbMouseImpl) DeviceDescriptor() DeviceDescriptor {
	return DeviceDescriptor{
		BLength:            sizeOf[DeviceDescriptor](),
		BDescriptorType:    DescriptorDevice,
		BcdUSB:             Version11,            // Using USB 1.1.0 to avoid handling additional descriptors in 2.0+.
		BDeviceClass:       0,                    // Must be 0 for a HID device.
		BDeviceSubclass:    0,                    // Must be 0 for a HID device.
		BDeviceProtocol:    0,                    // Must be 0 for a HID device.
		BMaxPacketSize:     8,                    // Copied from a real device using usbmon.
		IDVendor:           VendorIDGoogle,       // Must be an existing vendor ID, otherwise ChromeOS will disconnect the mouse.
		IDProduct:          ProductIDGoogleMouse, // Must be an existing product ID, otherwise ChromeOS will disconnect the mouse.
		BcdDevice:          BcdDeviceVersion1,
		IManufacturer:      mouseStringManufacturer, // Corresponding manufacturer string index.
		IProduct:           mouseStringProduct,      // Corresponding product string index.
		ISerialNumber:      mouseStringSerialNumber, // Corresponding serial number string index.
		BNumConfigurations: 1,                       // Provide a single configuration.
	}
}

// InterfaceDescriptor returns an interface descriptor.
func (m *usbMouseImpl) InterfaceDescriptor() InterfaceDescriptor {
	return InterfaceDescriptor{
		BLength:           sizeOf[InterfaceDescriptor](),
		BDescriptorType:   DescriptorInterface,
		BInterfaceNumber:  0, // This is the first and only interface exposed.
		BAlternateSetting: 0, // No alternate settings.
		BNumEndpoints:     uint8(len(m.EndpointDescriptors())),
		// BInterfaceClass + BInterfaceSubclass + BInterfaceProtocol together define a HID mouse.
		BInterfaceClass:    InterfaceClassHid,
		BInterfaceSubclass: InterfaceSubclassBoot,
		BInterfaceProtocol: InterfaceProtocolMouse,
		IInterface:         mouseStringInterface, // Corresponding interface string index.
	}
}

// ConfigurationDescriptor returns a configuration descriptor.
func (m *usbMouseImpl) ConfigurationDescriptor() ConfigurationDescriptor {
	return ConfigurationDescriptor{
		BLength:         sizeOf[ConfigurationDescriptor](),
		BDescriptorType: DescriptorConfiguration,
		WTotalLength: uint16(sizeOf[ConfigurationDescriptor]()) +
			uint16(sizeOf[InterfaceDescriptor]()) +
			uint16(sizeOf[HidDescriptor]()) +
			uint16(uint8(len(m.EndpointDescriptors()))*sizeOf[EndpointDescriptor]()),
		BNumInterfaces:      1,                        // Provide a single interface.
		BConfigurationValue: 0,                        // This is the first and only configuration exposed.
		IConfiguration:      mouseStringConfiguration, // Corresponding configuration string index.
		BmAttributes:        ConfigAttrBase,           // Copied from a real device using usbmon.
		BMaxPower:           0x32,                     // 100 mah current (copied from a real device using usbmon).
	}
}

// EndpointDescriptors returns an endpoint descriptors.
func (m *usbMouseImpl) EndpointDescriptors() []EndpointDescriptor {
	return []EndpointDescriptor{
		{
			BLength:          sizeOf[EndpointDescriptor](),
			BDescriptorType:  DescriptorEndpoint,
			BEndpointAddress: EpDirIn | 0b00000001,    // IN endpoint number #1 .
			BmAttributes:     EpTransferTypeInterrupt, // Mice always use interrupt transfer type.
			WMaxPacketSize:   4,                       // Mouse state contains 4 bytes.
			BInterval:        0x0a,                    // Copied from a real device using usbmon.
		},
	}
}

// StringDescriptor returns a string descriptor.
func (m *usbMouseImpl) StringDescriptor(index int) (string, error) {
	val, isAvailable := mouseStrings[index]
	if !isAvailable {
		return "", errors.Errorf("invalid string descriptor index: %d", index)
	}

	return val, nil
}
