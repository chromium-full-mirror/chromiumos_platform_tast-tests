// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbdevice

import (
	"bytes"
	"context"
	"time"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type usbHidDeviceHandler interface {
	withDevice(device Device) usbHidDeviceHandler
}

type hidDeviceHandlerImpl struct {
	device        Device
	ctx           context.Context
	isSendingData bool
}

func newHidDeviceHandler() *hidDeviceHandlerImpl {
	return &hidDeviceHandlerImpl{
		isSendingData: false,
	}
}

func (h *hidDeviceHandlerImpl) withDevice(device Device) *hidDeviceHandlerImpl {
	h.device = device
	return h
}

func (h *hidDeviceHandlerImpl) withContext(ctx context.Context) *hidDeviceHandlerImpl {
	h.ctx = ctx
	return h
}

func (h *hidDeviceHandlerImpl) getStringDescriptor(index uint8) (StringDescriptor, error) {
	var message []byte
	if index == 0 {
		// Index zero must return langID.
		// See https://www.keil.com/pack/doc/mw/USB/html/_u_s_b__string__descriptor.html .
		language, err := ToLE(h.device.LangID())
		if err != nil {
			return StringDescriptor{}, errors.Wrap(err, "unable to serialize langID")
		}
		message = language
	} else {
		val, err := h.device.StringDescriptor(int(index))
		if err != nil {
			return StringDescriptor{}, errors.Wrapf(err, "unable to get string descriptor %d", index)
		}
		message, err = utf16encode(val)
		if err != nil {
			return StringDescriptor{}, errors.Wrapf(err, "unable to serialize string %q", val)
		}
	}

	descriptor := StringDescriptor{
		BDescriptorType: DescriptorString,
		BString:         message,
	}

	return descriptor, nil
}

func (h *hidDeviceHandlerImpl) HandleUSBMessage(endpoint uint32, setupBytes [8]byte, transferBuffer []byte) error {
	setup, err := ReadLE[SetupPacket](bytes.NewBuffer(setupBytes[:]))
	if err != nil {
		return errors.Wrap(err, "unable to read UsbSetupPacket")
	}
	if endpoint == 0 {
		return h.handleControlMessage(setup, transferBuffer)
	} else if endpoint == 1 {
		return h.handleDataMessage(h.ctx, transferBuffer)
	}
	return errors.Errorf("unexpected USB endpoint: %d", endpoint)
}

func (h *hidDeviceHandlerImpl) handleControlMessage(setup SetupPacket, transferBuffer []byte) error {
	if setup.recipient() == RequestRecipientDevice {
		return h.handleDeviceRequest(setup, transferBuffer)
	}
	if setup.recipient() == RequestRecipientInterface {
		return h.handleInterfaceRequest(setup, transferBuffer)
	}
	return errors.Errorf("invalid CMD_SUBMIT recipient: %d", setup.recipient())
}

func (h *hidDeviceHandlerImpl) handleDeviceRequest(setup SetupPacket, transferBuffer []byte) error {
	switch RequestCode(setup.BRequest) {
	case RequestGetDescriptor:
		return handleGetDescriptor(setup, h, transferBuffer)
	case RequestGetStatus: // noop
	case RequestGetConfiguration: // noop
	case RequestSetConfiguration: // noop
	default:
		return errors.Errorf("invalid CMD_SUBMIT bRequest: %d", setup.BRequest)
	}
	return nil
}

func handleGetDescriptor(setup SetupPacket, h *hidDeviceHandlerImpl, transferBuffer []byte) error {
	descriptorType, descriptorIndex := getDescriptorTypeAndIndex(setup.WValue)
	descriptor, err := h.getDescriptor(descriptorType, descriptorIndex)
	if err != nil {
		return err
	}
	copy(transferBuffer, descriptor)
	return nil
}

func (h *hidDeviceHandlerImpl) handleInterfaceRequest(setup SetupPacket, transferBuffer []byte) error {
	switch HidRequestType(setup.BRequest) {
	case HidRequestSetIdle: // noop
	case HidRequestSetReport: // noop
	case HidRequestSetProtocol: // noop
	case HidRequestGetDescriptor:
		return handleHidGetDescriptor(setup, transferBuffer, h)
	default:
		return errors.Errorf("invalid USB Interface bRequest: %d", setup.BRequest)
	}
	return nil
}

func handleHidGetDescriptor(setup SetupPacket, transferBuffer []byte, h *hidDeviceHandlerImpl) error {
	descriptorType, descriptorIndex := getDescriptorTypeAndIndex(setup.WValue)
	if descriptorType != DescriptorHidReport {
		return errors.Errorf("invalid USB Interface descriptor: %d - %d", descriptorType, descriptorIndex)
	}
	copy(transferBuffer, h.device.HidReportDescriptor())
	return nil
}

func (h *hidDeviceHandlerImpl) handleDataMessage(ctx context.Context, transferBuffer []byte) error {
	if !h.isSendingData {
		// GoBigSleepLint: First ~200 milliseconds the data messages aren't processed by ChromeOS. Need to wait to avoid sending data into the void.
		if err := testing.Sleep(ctx, 200*time.Millisecond); err != nil {
			return errors.Wrap(err, "ran out of context before sending data")
		}
		h.isSendingData = true
	}

	data, err := h.device.Data()
	if err != nil {
		return errors.Wrap(err, "unable to produce data message")
	}
	copy(transferBuffer, data)
	return nil
}

func (h *hidDeviceHandlerImpl) getDescriptor(descriptorType DescriptorType, index uint8) ([]byte, error) {
	switch descriptorType {
	case DescriptorDevice:
		// DescriptorDevice is requested on device attach. At that moment there is no data transmission yet.
		// Set isSendingData = false here allows to handle multiple device attach-detach.
		h.isSendingData = false
		return ToLE(h.device.DeviceDescriptor())
	case DescriptorConfiguration:
		// Only support a single interface at the moment.
		return combineConfigs(
			h.device.ConfigurationDescriptor(),
			h.device.InterfaceDescriptor(),
			h.device.HidDescriptor(),
			h.device.EndpointDescriptors(),
		)
	case DescriptorString:
		descriptor, err := h.getStringDescriptor(index)
		if err != nil {
			return nil, errors.Wrap(err, "unable to get string descriptor")
		}
		return ToLE(descriptor)
	case DescriptorDebug:
		return []byte{}, nil
	case DescriptorDeviceQualifier:
		return nil, errors.Errorf("the device is specified as BcdUSB 0x0200 or higher. This is not supported. Need to implement descriptor type: %d", descriptorType)
	default:
		return nil, errors.Errorf("invalid Descriptor type: %d", descriptorType)
	}
}

func combineConfigs(config ConfigurationDescriptor, interfaceDescriptor InterfaceDescriptor, hid HidDescriptor, endpoints []EndpointDescriptor) ([]byte, error) {
	buffer := bytes.Buffer{}
	if err := WriteLE(&buffer, config); err != nil {
		return nil, errors.Wrap(err, "unable to serialise configuration descriptor")
	}
	if err := WriteLE(&buffer, interfaceDescriptor); err != nil {
		return nil, errors.Wrap(err, "unable to serialise interface descriptor")
	}
	if err := WriteLE(&buffer, hid); err != nil {
		return nil, errors.Wrap(err, "unable to serialise hid descriptor")
	}
	for _, endpoint := range endpoints {
		if err := WriteLE(&buffer, endpoint); err != nil {
			return nil, errors.Wrap(err, "unable to serialise endpoint")
		}
	}
	return buffer.Bytes(), nil
}

func getDescriptorTypeAndIndex(wValue uint16) (DescriptorType, uint8) {
	descriptorType := DescriptorType(wValue >> 8)
	descriptorIndex := uint8(wValue & 0xFF)
	return descriptorType, descriptorIndex
}
