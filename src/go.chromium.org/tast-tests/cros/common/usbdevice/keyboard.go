// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbdevice

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// UsbKeyboard represents a virtual keyboard.
// See https://wiki.osdev.org/USB_Human_Interface_Devices#USB_keyboard for more details.
type UsbKeyboard interface {
	Type(string) UsbKeyboard
}

const (
	keyboardStringManufacturer  = 1
	keyboardStringProduct       = 2
	keyboardStringSerialNumber  = 3
	keyboardStringConfiguration = 4
	keyboardStringInterface     = 5
)

var keyboardStrings = map[int]string{
	keyboardStringManufacturer:  "Google",
	keyboardStringProduct:       "Virtual USB Keyboard",
	keyboardStringSerialNumber:  "No Serial Number",
	keyboardStringConfiguration: "Default Configuration",
	keyboardStringInterface:     "Default Interface",
}

type usbKeyboardImpl struct {
	keysQueue [][]byte
	handler   DeviceHandler
	ctx       context.Context
}

// NewUsbKeyboard returns a new instance of UsbKeyboard.
// Keyboard handler has to store context in order to perform a sleep.
// As a result, keyboard instances must only be created inside of the test (not in a fixture).
func NewUsbKeyboard(ctx context.Context) *usbKeyboardImpl {
	handler := newHidDeviceHandler()
	k := usbKeyboardImpl{
		handler:   handler,
		ctx:       ctx,
		keysQueue: [][]byte{},
	}
	handler.withDevice(&k).withContext(ctx)
	return &k
}

// getKeySet given a character returns a key set with specific keys pressed.
// Key sets allow to press up to 6 keys + modifiers.
// Currently we are only producing key sets containing one key (with left shift modifier, depending on the character).
func getKeySet(key byte) []byte {
	type location struct {
		code  int
		shift bool
	}
	// Locations are based on https://www.win.tue.nl/~aeb/linux/kbd/scancodes-14.html
	locations := map[string]location{
		"	 -=[]\\":                   {0x2b, false},
		";'`,./":                     {0x33, false},
		"1234567890":                 {0x1e, false},
		"_+{}|":                      {0x2d, true},
		":\"~<>?":                    {0x33, true},
		"!@#$%^&*()":                 {0x1e, true},
		"abcdefghijklmnopqrstuvwxyz": {0x04, false},
		"ABCDEFGHIJKLMNOPQRSTUVWXYZ": {0x04, true},
	}
	var leftShift byte = 0b00000010
	for pattern, v := range locations {
		if i := strings.Index(pattern, string(key)); i != -1 {
			var modifier byte
			if v.shift {
				modifier = modifier | leftShift
			}
			return []byte{modifier, 0x00, byte(v.code + i), 0x00, 0x00, 0x00, 0x00, 0x00}
		}
	}

	panic(fmt.Sprintf("Unsupported key provided: %s", string(key)))
}

// getKeyUpSet returns a key set with all the keys depressed.
// It does not depend on the keys pressed previously - we depress all the keys unconditionally.
// Together with getKeySet it is used to emulate a key press:
// getKeySet() emulates a key down,
// getKeyUpSet() emulates a key up.
func getKeyUpSet() []byte {
	return []byte{0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00, 0x00}
}

// Type allows to type using the virtual keyboard.
// The passed string is split into characters,
// characters are converted to the scancodes and interleaved with
// keys-up sequences to emulate key down and key up events.
// These commands are added to the keys queue. This allows to queue
// multiple entries in a non-blocking way.
func (k *usbKeyboardImpl) Type(str string) UsbKeyboard {
	bytes := []byte(str)
	for _, key := range bytes {
		k.keysQueue = append(
			k.keysQueue,
			getKeySet(key),
			getKeyUpSet(),
		)
	}
	return k
}

// GetHandler returns a handler that will handle USB communication for the device.
func (k *usbKeyboardImpl) GetHandler() DeviceHandler {
	return k.handler
}

// Data returns the value to return for a data USB message.
func (k *usbKeyboardImpl) Data() ([]byte, error) {
	var data []byte
	if len(k.keysQueue) > 0 {
		data, k.keysQueue = k.keysQueue[0], k.keysQueue[1:]
		// GoBigSleepLint: Simulating a specific speed of keypress.
		if err := testing.Sleep(k.ctx, 50*time.Millisecond); err != nil {
			return nil, errors.Wrap(err, "ran out of context time while returning key presses")
		}
		return data, nil
	}

	return getKeyUpSet(), nil
}

// HidReport returns a HID report.
// The data has been collected using usbmon from a real keyboard.
func (k *usbKeyboardImpl) HidReportDescriptor() []byte {
	return []byte{
		// comments by http://eleccelerator.com/usbdescreqparser/
		0x05, 0x01, // Usage Page (Generic Desktop Ctrls)
		0x09, 0x06, // Usage (Keyboard)
		0xA1, 0x01, // Collection (Application)
		0x05, 0x07, //   Usage Page (Kbrd/Keypad)
		0x19, 0xE0, //   Usage Minimum (0xE0)
		0x29, 0xE7, //   Usage Maximum (0xE7)
		0x15, 0x00, //   Logical Minimum (0)
		0x25, 0x01, //   Logical Maximum (1)
		0x75, 0x01, //   Report Size (1)
		0x95, 0x08, //   Report Count (8)
		0x81, 0x02, //   Input (Data,Var,Abs,No Wrap,Linear,Preferred State,No Null Position)
		0x95, 0x01, //   Report Count (1)
		0x75, 0x08, //   Report Size (8)
		0x81, 0x01, //   Input (Const,Array,Abs,No Wrap,Linear,Preferred State,No Null Position)
		0x95, 0x05, //   Report Count (5)
		0x75, 0x01, //   Report Size (1)
		0x05, 0x08, //   Usage Page (LEDs)
		0x19, 0x01, //   Usage Minimum (Num Lock)
		0x29, 0x05, //   Usage Maximum (Kana)
		0x91, 0x02, //   Output (Data,Var,Abs,No Wrap,Linear,Preferred State,No Null Position,Non-volatile)
		0x95, 0x01, //   Report Count (1)
		0x75, 0x03, //   Report Size (3)
		0x91, 0x01, //   Output (Const,Array,Abs,No Wrap,Linear,Preferred State,No Null Position,Non-volatile)
		0x95, 0x06, //   Report Count (6)
		0x75, 0x08, //   Report Size (8)
		0x15, 0x00, //   Logical Minimum (0)
		0x25, 0x65, //   Logical Maximum (101)
		0x05, 0x07, //   Usage Page (Kbrd/Keypad)
		0x19, 0x00, //   Usage Minimum (0x00)
		0x29, 0x65, //   Usage Maximum (0x65)
		0x81, 0x00, //   Input (Data,Array,Abs,No Wrap,Linear,Preferred State,No Null Position)
		0xC0, // End Collection
	}
}

// HidDescriptor returns a HID descriptor.
func (k *usbKeyboardImpl) HidDescriptor() HidDescriptor {
	return HidDescriptor{
		BLength:                 sizeOf[HidDescriptor](),
		BDescriptorType:         DescriptorHid,
		BcdHID:                  HidVersion111, // Using HID 1.1.1.
		BCountryCode:            0,             // 0 means hardware is not localized.
		BNumDescriptors:         1,             // There is 1 additional descriptor(HID report).
		BClassDescriptorType:    DescriptorHidReport,
		WReportDescriptorLength: uint16(len(k.HidReportDescriptor())),
	}
}

// LangID returns a LANGID (defined by the USB-IF) used by the device.
func (k *usbKeyboardImpl) LangID() uint16 {
	return LangidEngUsa // Provide strings in English only.
}

// DeviceDescriptor returns a device descriptor.
func (k *usbKeyboardImpl) DeviceDescriptor() DeviceDescriptor {
	return DeviceDescriptor{
		BLength:            sizeOf[DeviceDescriptor](),
		BDescriptorType:    DescriptorDevice,
		BcdUSB:             Version11, // Using USB 1.1.0 to avoid handling additional descriptors in 2.0+.
		BDeviceClass:       0,         // Must be 0 for a HID device.
		BDeviceSubclass:    0,         // Must be 0 for a HID device.
		BDeviceProtocol:    0,         // Must be 0 for a HID device.
		BMaxPacketSize:     8,         // Key set the keyboard returns contains 8 bytes.
		IDVendor:           VendorIDGoogle,
		IDProduct:          ProductIDGoogleKeyboard,
		BcdDevice:          BcdDeviceVersion1,
		IManufacturer:      keyboardStringManufacturer, // Corresponding manufacturer string index.
		IProduct:           keyboardStringProduct,      // Corresponding product string index.
		ISerialNumber:      keyboardStringSerialNumber, // Corresponding serial number string index.
		BNumConfigurations: 1,                          // Provide a single configuration.
	}
}

// InterfaceDescriptor returns an interface descriptor.
func (k *usbKeyboardImpl) InterfaceDescriptor() InterfaceDescriptor {
	return InterfaceDescriptor{
		BLength:           sizeOf[InterfaceDescriptor](),
		BDescriptorType:   DescriptorInterface,
		BInterfaceNumber:  0, // This is the first and only interface exposed.
		BAlternateSetting: 0, // No alternate settings.
		BNumEndpoints:     uint8(len(k.EndpointDescriptors())),
		// BInterfaceClass + BInterfaceSubclass + BInterfaceProtocol together define a HID keyboard.
		BInterfaceClass:    InterfaceClassHid,
		BInterfaceSubclass: InterfaceSubclassBoot,
		BInterfaceProtocol: InterfaceProtocolKeyboard,
		IInterface:         keyboardStringInterface, // Corresponding interface string index.
	}
}

// ConfigurationDescriptor returns a configuration descriptor.
func (k *usbKeyboardImpl) ConfigurationDescriptor() ConfigurationDescriptor {
	return ConfigurationDescriptor{
		BLength:         sizeOf[ConfigurationDescriptor](),
		BDescriptorType: DescriptorConfiguration,
		WTotalLength: uint16(sizeOf[ConfigurationDescriptor]()) +
			uint16(sizeOf[InterfaceDescriptor]()) +
			uint16(sizeOf[HidDescriptor]()) +
			uint16(uint8(len(k.EndpointDescriptors()))*sizeOf[EndpointDescriptor]()),
		BNumInterfaces:      1,                           // Provide a single interface.
		BConfigurationValue: 0,                           // This is the first and only configuration exposed.
		IConfiguration:      keyboardStringConfiguration, // Corresponding configuration string index.
		BmAttributes:        ConfigAttrBase,              // Copied from a real device using usbmon.
		BMaxPower:           0x32,                        // 100 mah current (copied from a real device using usbmon).
	}
}

// EndpointDescriptors returns an endpoint descriptors.
func (k *usbKeyboardImpl) EndpointDescriptors() []EndpointDescriptor {
	return []EndpointDescriptor{
		{
			BLength:          sizeOf[EndpointDescriptor](),
			BDescriptorType:  DescriptorEndpoint,
			BEndpointAddress: EpDirIn | 0b00000001,    // IN endpoint number #1 .
			BmAttributes:     EpTransferTypeInterrupt, // Keyboards always use interrupt transfer type.
			WMaxPacketSize:   8,                       // Key set the keyboard returns contains 8 bytes.
			BInterval:        0xFF,                    // Copied from a real device using usbmon.
		},
	}
}

// StringDescriptor returns a string descriptor.
func (k *usbKeyboardImpl) StringDescriptor(index int) (string, error) {
	val, isAvailable := keyboardStrings[index]
	if !isAvailable {
		return "", errors.Errorf("invalid string descriptor index: %d", index)
	}

	return val, nil
}
