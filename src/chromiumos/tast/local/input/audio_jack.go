// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"context"
	"math/big"
	"os"

	"chromiumos/tast/errors"
	"chromiumos/tast/testing"
)

// AudioJackEventWriter supports injecting events into an audio jack device.
type AudioJackEventWriter struct {
	eventWriter *RawEventWriter
	path        string
	virtualFile *os.File
}

// AudioJack returns an EventWriter to inject events into an arbitrary audio
// jack device. To release the resources of this event writer, the `Close`
// function can be called.
func AudioJack(ctx context.Context) (*AudioJackEventWriter, error) {
	infos, err := readDevices("")
	if err != nil {
		return nil, errors.Wrap(err, "failed to read devices")
	}
	for _, info := range infos {
		if !info.isAudioJack() {
			continue
		}
		testing.ContextLogf(ctx, "Opening audio jack device %+v", info)

		device, err := Device(ctx, info.path)
		if err != nil {
			return nil, err
		}
		return &AudioJackEventWriter{
			eventWriter: device,
			path:        info.path,
		}, nil
	}

	// If there is no real audio jack device to inject into, create a virtual device.
	testing.ContextLog(ctx, "Creating virtual audio jack device")
	return virtualAudioJackDevice(ctx)
}

// Close closes the audio jack device.
func (aw *AudioJackEventWriter) Close() error {
	var firstErr error
	if aw.eventWriter != nil {
		firstErr = aw.eventWriter.Close()
	}
	if aw.virtualFile != nil {
		if err := aw.virtualFile.Close(); firstErr == nil {
			firstErr = err
		}
	}
	return firstErr
}

// PlugInHeadphone emits an headphone "add" event.
func (aw *AudioJackEventWriter) PlugInHeadphone() error {
	if err := aw.eventWriter.Event(EV_SW, SW_HEADPHONE_INSERT, 1); err != nil {
		return err
	}

	return aw.eventWriter.Sync()
}

// PlugOutHeadphone emits an headphone "remove" event.
func (aw *AudioJackEventWriter) PlugOutHeadphone() error {
	if err := aw.eventWriter.Event(EV_SW, SW_HEADPHONE_INSERT, 0); err != nil {
		return err
	}

	return aw.eventWriter.Sync()
}

// PlugInMicrophone emits an microphone "add" event.
func (aw *AudioJackEventWriter) PlugInMicrophone() error {
	if err := aw.eventWriter.Event(EV_SW, SW_MICROPHONE_INSERT, 1); err != nil {
		return err
	}

	return aw.eventWriter.Sync()
}

// PlugOutMicrophone emits an microphone "remove" event.
func (aw *AudioJackEventWriter) PlugOutMicrophone() error {
	if err := aw.eventWriter.Event(EV_SW, SW_MICROPHONE_INSERT, 0); err != nil {
		return err
	}

	return aw.eventWriter.Sync()
}

// virtualAudioJackDevice creates a virtual audio jack device and returns an
// event writer that injects into it.
func virtualAudioJackDevice(ctx context.Context) (*AudioJackEventWriter, error) {
	const (
		deviceName = "Headset Jack"

		// Device constants all set to zero.
		busType = 0
		vendor  = 0
		product = 0
		version = 0

		// Input characteristics.
		props   = 0
		evTypes = 1 << EV_SW
	)

	dev, virt, err := createVirtual(
		deviceName, devID{busType, vendor, product, version}, props, evTypes,
		map[EventType]*big.Int{
			// The device supports plugging and unplugging of a microphone and headphone.
			EV_SW: makeBigIntFromEventCodes([]EventCode{SW_HEADPHONE_INSERT, SW_MICROPHONE_INSERT}),
		}, map[EventCode]Axis{})

	if err != nil {
		return nil, err
	}
	device, err := Device(ctx, dev)
	if err != nil {
		return nil, err
	}

	return &AudioJackEventWriter{
		eventWriter: device,
		path:        dev,
		virtualFile: virt,
	}, nil
}
