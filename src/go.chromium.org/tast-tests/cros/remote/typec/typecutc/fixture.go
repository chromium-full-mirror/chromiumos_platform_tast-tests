// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecutc contains fixtures for utc device testing.
package typecutc

import (
	"context"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast-tests/cros/common/usbutils/utc"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "typecUtc",
		Desc:     "Initializes and provides a utc USB PD tester interface",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent:    "b:958036",
		Impl:            &UtcFixture{},
		SetUpTimeout:    3 * time.Minute, // For utc initialization.
		ResetTimeout:    3 * time.Minute, // For utc state reset.
		TearDownTimeout: 3 * time.Minute, // For closing the utc connection.
		Vars: []string{
			"typec.UtcUri",    // Required: URI for the utc device.
			"typec.UtcSerial", // Optional: The utc device serial.
		},
	})
}

const (
	defaultInitialPdoCount = 4
	defaultInitialPdState  = utc.InitPdStateDrp
	defaultUsbChannel      = utc.UsbChannelUSB3And2
)

// UtcFixture holds the state for the utc fixture.
type UtcFixture struct {
	utcController *utc.UsbTester
}

// FixtureData holds the data passed from the typecUtc fixture to the test.
// Tests will cast s.FixtValue() to this type.
type FixtureData struct {
	Utc *utc.UsbTester
}

// SetUp initializes the utc device.
func (f *UtcFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	utcURI := s.RequiredVar("typec.UtcUri")
	utcSerial, _ := s.Var("typec.UtcSerial")

	var pasitTopology *labapi.PasitHost
	if dutConfig, err := s.ChromeOSDUTLabConfig(""); err == nil {
		if dutConfig.GetChromeos().GetPasitHost() != nil {
			pasitTopology = dutConfig.GetChromeos().GetPasitHost()
			s.Log("Loaded DUT info from lab config")
		}
	}

	ug, err := utc.New(ctx, utcURI, utcSerial, pasitTopology)
	if err != nil {
		s.Fatalf("Failed to connect to utc device at %s: %v", utcURI, err)
	}
	f.utcController = ug

	// TODO(bszpila): Disable ports once SetTestPort is implemented properly.
	if err := f.utcController.SetTestPort(ctx, 1); err != nil {
		// Attempt to close if SetTestPort fails.
		if closeErr := f.utcController.Close(ctx); closeErr != nil {
			s.Error("Failed to close utc during SetUp after SetTestPort failure: ", closeErr)
		}
		s.Fatalf("Failed to set utc active port to %d: %v", 1, err)
	}

	// Set a default state (DRP) to ensure it's not sourcing/sinking unexpectedly.
	if err := f.utcController.SetInitPdState(ctx, utc.InitPdStateDrp); err != nil {
		if closeErr := f.utcController.Close(ctx); closeErr != nil {
			s.Error("Failed to close utc during SetUp after SetInitPdState failure: ", closeErr)
		}
		s.Fatal("Failed to set utc initial PD state to DRP: ", err)
	}
	s.Log("utc initial PD state set to DRP")

	// Reset to USB3 mode.
	if err := f.utcController.SetUsbChannel(ctx, utc.UsbChannelUSB3And2); err != nil {
		if closeErr := f.utcController.Close(ctx); closeErr != nil {
			s.Error("Failed to close utc during SetUp after SetUsbChannel failure: ", closeErr)
		}
		s.Fatal("Failed to set utc USB channel to USB3: ", err)
	}
	s.Log("utc USB channel set to USB3")

	return &FixtureData{Utc: f.utcController}
}

// TearDown closes the connection to the utc device.
func (f *UtcFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.utcController != nil {
		if err := f.utcController.Close(ctx); err != nil {
			s.Error("Failed to close utc device: ", err)
		}
	}
}

// Reset is called after each test. It ensures the utc device is in a clean state.
func (f *UtcFixture) Reset(ctx context.Context) error {
	if f.utcController == nil {
		return errors.New("utc controller not initialized in Reset")
	}

	// TODO(b/416456393): Disable ports once SetTestPort is implemented properly.
	if err := f.utcController.SetTestPort(ctx, 1); err != nil {
		return errors.Wrapf(err, "failed to set utc active port to %d during Reset", 1)
	}

	// Reset to default Init PD state.
	if err := f.utcController.SetInitPdState(ctx, defaultInitialPdState); err != nil {
		return errors.Wrapf(err, "failed to set utc to %s state during Reset", defaultInitialPdState)
	}

	// Reset to default USB mode.
	if err := f.utcController.SetUsbChannel(ctx, defaultUsbChannel); err != nil {
		return errors.Wrapf(err, "failed to set utc to %s mode during Reset", defaultUsbChannel)
	}

	// Set initial PDO count.
	if err := f.utcController.SetSrcPdoCount(ctx, defaultInitialPdoCount); err != nil {
		return errors.Wrapf(err, "failed to set utc initial PDO count to %d during Reset", defaultInitialPdoCount)
	}

	return nil
}

// PreTest is called before each test.
func (f *UtcFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest is called after each test.
func (f *UtcFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
