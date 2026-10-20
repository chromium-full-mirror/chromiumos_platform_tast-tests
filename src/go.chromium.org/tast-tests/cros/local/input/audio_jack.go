// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package input

import (
	"context"
	"fmt"
	"math/big"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// udevSettleTimeout is the maximum time to wait for udev to process the
// addition of a virtual audio jack device.
const udevSettleTimeout = 10 * time.Second

// AudioJackEventWriter supports injecting events into an audio jack device.
type AudioJackEventWriter struct {
	eventWriter *RawEventWriter
	path        string
	virtualFile *os.File
}

// Appended to virtual audio jack device name
var nextVirtAudioJackNum = 1

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
func (aw *AudioJackEventWriter) Close(ctx context.Context) error {
	var firstErr error
	if aw.eventWriter != nil {
		if firstErr = aw.eventWriter.Close(); firstErr != nil {
			testing.ContextLog(ctx, "Failed to close raw event writer while closing the audio jack event writer: ", firstErr)
		}
	}
	if aw.virtualFile != nil {
		if err := aw.virtualFile.Close(); err != nil {
			testing.ContextLog(ctx, "Failed to close virtual file while closing the audio jack event writer: ", err)
			if firstErr == nil {
				firstErr = err
			}
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
func virtualAudioJackDevice(ctx context.Context) (aw *AudioJackEventWriter, retErr error) {
	const (
		// Device constants all set to zero.
		busType = 0
		vendor  = 0
		product = 0
		version = 0

		// Input characteristics.
		props   = 0
		evTypes = 1 << EV_SW
	)

	// Include our PID in the device name to be extra careful in case an old bundle process hasn't exited.
	deviceName := fmt.Sprintf("Tast virtual audio jack %d.%d", os.Getpid(), nextVirtAudioJackNum)
	nextVirtAudioJackNum++

	testing.ContextLogf(ctx, "Creating virtual audio jack device %q", deviceName)
	dev, virt, err := createVirtual(
		deviceName, devID{busType, vendor, product, version}, props, evTypes,
		map[EventType]*big.Int{
			// The device supports plugging and unplugging of a microphone and headphone.
			EV_SW: makeBigIntFromEventCodes([]EventCode{SW_HEADPHONE_INSERT, SW_MICROPHONE_INSERT}),
		}, map[EventCode]Axis{})
	if err != nil {
		return nil, err
	}

	defer func(ctx context.Context) {
		if retErr != nil {
			if err := virt.Close(); err != nil {
				testing.ContextLog(ctx, "Failed to close the virtual device for the audio jack event writer: ", err)
			}
		}
	}(ctx)
	testing.ContextLog(ctx, "Using virtual audio jack device ", dev)

	// The device node is usable by root right away, but unprivileged consumers
	// such as cros_healthd can only open it once udev has applied the input
	// device rules (e.g. GROUP="input"). Wait for udev to finish processing the
	// new device so that callers can rely on it immediately.
	if err := waitForUdevSettled(ctx); err != nil {
		retErr = errors.Wrap(err, "failed to wait for udev to set up the virtual audio jack device")
		return nil, retErr
	}

	device, err := Device(ctx, dev)
	if err != nil {
		retErr = err
		return nil, retErr
	}

	return &AudioJackEventWriter{
		eventWriter: device,
		path:        dev,
		virtualFile: virt,
	}, nil
}

// waitForUdevSettled waits until udev has processed all queued events, such as
// the addition of a freshly created virtual device.
func waitForUdevSettled(ctx context.Context) error {
	ctx, cancel := context.WithTimeout(ctx, udevSettleTimeout)
	defer cancel()
	return testexec.CommandContext(ctx, "udevadm", "settle", fmt.Sprintf("--timeout=%d", int(udevSettleTimeout.Seconds()))).Run(testexec.DumpLogOnError)
}
