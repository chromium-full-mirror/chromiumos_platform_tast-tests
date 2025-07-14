// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecunigraf contains fixtures for Unigraf device testing.
package typecunigraf

import (
	"context"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast-tests/cros/common/usbutils/unigraf"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "typecUnigraf",
		Desc:     "Initializes and provides a Unigraf USB PD tester interface",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent:    "b:958036",
		Impl:            &UnigrafFixture{},
		SetUpTimeout:    20 * time.Second, // For Unigraf initialization.
		ResetTimeout:    15 * time.Second, // For Unigraf state reset.
		TearDownTimeout: 15 * time.Second, // For closing the Unigraf connection.
		Vars: []string{
			"typec.UnigrafUri",    // Required: URI for the Unigraf device.
			"typec.UnigrafSerial", // Optional: The unigraf device serial.
		},
	})
}

const (
	defaultInitialPdoCount = 4
	defaultInitialPdState  = unigraf.InitPdStateDrp
	defaultUsbChannel      = unigraf.UsbChannelUSB3And2
)

// UnigrafFixture holds the state for the Unigraf fixture.
type UnigrafFixture struct {
	unigrafController *unigraf.UsbTester
}

// FixtureData holds the data passed from the typecUnigraf fixture to the test.
// Tests will cast s.FixtValue() to this type.
type FixtureData struct {
	Unigraf *unigraf.UsbTester
}

// SetUp initializes the Unigraf device.
func (f *UnigrafFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	unigrafURI := s.RequiredVar("typec.UnigrafUri")
	unigrafSerial, _ := s.Var("typec.UnigrafSerial")

	var pasitTopology *labapi.PasitHost
	if dutConfig, err := s.ChromeOSDUTLabConfig(""); err == nil {
		if dutConfig.GetChromeos().GetPasitHost() != nil {
			pasitTopology = dutConfig.GetChromeos().GetPasitHost()
			s.Log("Loaded DUT info from lab config")
		}
	}

	ug, err := unigraf.New(ctx, unigrafURI, unigrafSerial, pasitTopology)
	if err != nil {
		s.Fatalf("Failed to connect to Unigraf device at %s: %v", unigrafURI, err)
	}
	f.unigrafController = ug

	// TODO(bszpila): Disable ports once SetTestPort is implemented properly.
	if err := f.unigrafController.SetTestPort(ctx, 1); err != nil {
		// Attempt to close if SetTestPort fails.
		if closeErr := f.unigrafController.Close(ctx); closeErr != nil {
			s.Error("Failed to close Unigraf during SetUp after SetTestPort failure: ", closeErr)
		}
		s.Fatalf("Failed to set Unigraf active port to %d: %v", 1, err)
	}

	// Set a default state (DRP) to ensure it's not sourcing/sinking unexpectedly.
	if err := f.unigrafController.SetInitPdState(ctx, unigraf.InitPdStateDrp); err != nil {
		if closeErr := f.unigrafController.Close(ctx); closeErr != nil {
			s.Error("Failed to close Unigraf during SetUp after SetInitPdState failure: ", closeErr)
		}
		s.Fatal("Failed to set Unigraf initial PD state to DRP: ", err)
	}
	s.Log("Unigraf initial PD state set to DRP")

	// Reset to USB3 mode.
	if err := f.unigrafController.SetUsbChannel(ctx, unigraf.UsbChannelUSB3And2); err != nil {
		if closeErr := f.unigrafController.Close(ctx); closeErr != nil {
			s.Error("Failed to close Unigraf during SetUp after SetUsbChannel failure: ", closeErr)
		}
		s.Fatal("Failed to set Unigraf USB channel to USB3: ", err)
	}
	s.Log("Unigraf USB channel set to USB3")

	return &FixtureData{Unigraf: f.unigrafController}
}

// TearDown closes the connection to the Unigraf device.
func (f *UnigrafFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.unigrafController != nil {
		if err := f.unigrafController.Close(ctx); err != nil {
			s.Error("Failed to close Unigraf device: ", err)
		}
	}
}

// Reset is called after each test. It ensures the Unigraf device is in a clean state.
func (f *UnigrafFixture) Reset(ctx context.Context) error {
	if f.unigrafController == nil {
		return errors.New("Unigraf controller not initialized in Reset")
	}

	// TODO(b/416456393): Disable ports once SetTestPort is implemented properly.
	if err := f.unigrafController.SetTestPort(ctx, 1); err != nil {
		return errors.Wrapf(err, "failed to set Unigraf active port to %d during Reset", 1)
	}

	// Reset to default Init PD state.
	if err := f.unigrafController.SetInitPdState(ctx, defaultInitialPdState); err != nil {
		return errors.Wrapf(err, "failed to set Unigraf to %s state during Reset", defaultInitialPdState)
	}

	// Reset to default USB mode.
	if err := f.unigrafController.SetUsbChannel(ctx, defaultUsbChannel); err != nil {
		return errors.Wrapf(err, "failed to set Unigraf to %s mode during Reset", defaultUsbChannel)
	}

	// Set initial PDO count.
	if err := f.unigrafController.SetSrcPdoCount(ctx, defaultInitialPdoCount); err != nil {
		return errors.Wrapf(err, "failed to set Unigraf initial PDO count to %d during Reset", defaultInitialPdoCount)
	}

	return nil
}

// PreTest is called before each test.
func (f *UnigrafFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest is called after each test.
func (f *UnigrafFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
