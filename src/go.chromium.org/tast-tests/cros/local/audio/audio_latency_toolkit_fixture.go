// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/usbutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name: fixture.AudioLatencyToolkit,
		Desc: "Example test for using Audio Latency Toolkit to measure latency",
		Contacts: []string{
			"crosep-intertech@google.com",
			"chromeos-audio-bugs@google.com",
		},
		Impl:            LatencyToolkitFixture{},
		SetUpTimeout:    20 * time.Second,
		TearDownTimeout: 20 * time.Second,
	})
}

// LatencyToolkitFixture is a fixture to find and select connected audio latency
// toolkit
type LatencyToolkitFixture struct {
	Device usbutil.Device
}

// SetUp the AudioLatencyToolkitFixture
func (f LatencyToolkitFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {

	attachedDevices, err := usbutil.AttachedDevices(ctx)
	if err != nil {
		s.Fatal("Failed to get attached devices: ", err)
	}

	var teensyDevice *usbutil.Device

	for _, attachedDevice := range attachedDevices {
		if strings.Contains(attachedDevice.ProductName, "Teensyduino") {
			teensyDevice = &attachedDevice
		}
		s.Logf("Attached Device, Vendor: %s Product Name: %s", attachedDevice.VendorName, attachedDevice.ProductName)
	}

	if teensyDevice == nil {
		s.Fatal("Teensy not connected to DUT")
	}

	f.Device = *teensyDevice

	return f
}

// TearDown the LatencyToolkitFixture
func (f LatencyToolkitFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}

// Reset the LatencyToolkitFixture
func (f LatencyToolkitFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest the LatencyToolkitFixture
func (f LatencyToolkitFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest the LatencyToolkitFixture
func (f LatencyToolkitFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
