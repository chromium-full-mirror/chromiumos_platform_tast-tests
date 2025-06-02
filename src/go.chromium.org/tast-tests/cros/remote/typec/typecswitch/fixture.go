// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecswitch contains the usb switch interface for the tests in the typec directory.
package typecswitch

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/usbutils/unigraf"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "typecSwitch",
		Desc:     "Initializes and provides a Type-C switch (MCCI or Unigraf) interface.",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com", "jthies@google.com"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent:    "b:958036",
		Impl:            &SwitchFixture{},
		SetUpTimeout:    20 * time.Second, // For switch initialization and initial DisablePorts
		ResetTimeout:    15 * time.Second, // For DisablePorts and potential mode reset
		TearDownTimeout: 15 * time.Second, // For closing the switch
		Vars: []string{ // These Vars are needed by typecswitch.GetSwitch
			"typec.McciSerial",
			"typec.McciPort",
			"typec.McciPath",
			"typec.UnigrafUri",
		},
	})
}

// SwitchFixture holds the state for the Type-C switch fixture.
type SwitchFixture struct {
	TestSwitch Switch
}

// FixtureData holds the data passed from the fixture to the test.
// Tests will cast s.FixtValue() to this type.
type FixtureData struct {
	TestSwitch Switch
}

// SetUp initializes the Type-C switch.
func (f *SwitchFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	ts, err := newSwitch(ctx, s)
	if err != nil {
		s.Fatal("Failed to get switch handle: ", err)
	}
	f.TestSwitch = ts

	// Ensure ports are disabled initially as a baseline.
	if err := f.TestSwitch.DisablePorts(ctx); err != nil {
		// Attempt to close the switch if disabling ports fails during setup.
		if closeErr := f.TestSwitch.Close(ctx); closeErr != nil {
			s.Errorf("Failed to close switch during SetUp after DisablePorts failure: %v", closeErr)
		}
		s.Fatalf("Failed to disable ports during SetUp: %v", err)
	}

	return &FixtureData{TestSwitch: f.TestSwitch}
}

// TearDown closes the connection to the Type-C switch.
func (f *SwitchFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.TestSwitch != nil {
		if err := f.TestSwitch.Close(ctx); err != nil {
			s.Error("Failed to close switch during TearDown: ", err)
		}
	}
}

// Reset is called after each test. It ensures the switch is in a clean state for the next test.
func (f *SwitchFixture) Reset(ctx context.Context) error {
	if f.TestSwitch == nil {
		// This should ideally not happen if SetUp was successful.
		return errors.New("switch is not initialized in Reset")
	}

	// Ensure ports are disabled.
	if err := f.TestSwitch.DisablePorts(ctx); err != nil {
		return errors.Wrap(err, "failed to disable ports during Reset")
	}

	// Attempt to reset to USB3 mode as a common default.
	if err := f.TestSwitch.EnterUsb3Mode(ctx); err != nil {
		return errors.Wrap(err, "failed to reset Unigraf to USB3 mode during fixture reset")
	}
	return nil
}

// PreTest is called before each test.
func (i *SwitchFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest is called after each test.
func (i *SwitchFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

// newSwitch returns an interface for the usb switch.
func newSwitch(ctx context.Context, s *testing.FixtState) (Switch, error) {
	if unigrafURI, unigrafPresent := s.Var("typec.UnigrafUri"); unigrafPresent {
		unigrafObj, err := unigraf.New(ctx, unigrafURI)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create unigraf object")
		}
		return unigrafObj, nil

	} else if mcciSerial, mcciPresent := s.Var("typec.McciSerial"); mcciPresent {
		path, _ := s.Var("typec.McciPath")
		portStr, _ := s.Var("typec.McciPort")
		portUsed, err := strconv.Atoi(portStr)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse MCCI port cmdline argument")
		}

		mcciObj, err := mcci.GetSwitch(mcciSerial, path, portUsed)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get MCCI switch handle")
		}

		return mcciObj, nil
	}

	return nil, errors.New("failed to parse usb switch device arguments")
}
