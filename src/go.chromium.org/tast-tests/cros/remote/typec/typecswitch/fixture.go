// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecswitch contains the usb switch fixture and helper functions for the tests in the typec directory.
package typecswitch

import (
	"context"
	"strconv"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast-tests/cros/common/usbutils/utc"
	"go.chromium.org/tast-tests/cros/common/usbutils/usbswitch"
	"go.chromium.org/tast-tests/cros/remote/typec/mcci"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "typecSwitch",
		Desc:     "Initializes and provides a Type-C switch (MCCI or utc) interface",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent:    "b:958036",
		Impl:            &SwitchFixture{},
		SetUpTimeout:    20 * time.Second, // For switch initialization and initial DisablePorts
		ResetTimeout:    15 * time.Second, // For DisablePorts and potential mode reset
		TearDownTimeout: 15 * time.Second, // For closing the switch
		Vars: []string{
			"typec.McciSerial",
			"typec.McciPath",
			"typec.UtcUri",
			"typec.SwitchPort",
			"typec.UtcSerial",
		},
	})
}

// SwitchFixture holds the state for the Type-C switch fixture.
type SwitchFixture struct {
	TestSwitch usbswitch.Switch
	PortNum    int
}

// FixtureData holds the data passed from the fixture to the test.
// Tests will cast s.FixtValue() to this type.
type FixtureData struct {
	TestSwitch usbswitch.Switch
}

// SetUp initializes the Type-C switch.
func (f *SwitchFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	ts, err := newSwitch(ctx, s)
	if err != nil {
		s.Fatal("Failed to get switch handle: ", err)
	}
	f.TestSwitch = ts

	if portStr, portPresent := s.Var("typec.SwitchPort"); portPresent {
		if portUsed, err := strconv.Atoi(portStr); err != nil {
			if err := f.TestSwitch.Close(ctx); err != nil {
				s.Error("Failed to close switch: ", err)
			}
			s.Fatal("Failed to convert port number to integer: ", err)
		} else if err := f.TestSwitch.SetActiveSwitchPort(ctx, portUsed); err != nil {
			// Attempt to close the switch if setting active port fails during setup.
			if closeErr := f.TestSwitch.Close(ctx); closeErr != nil {
				s.Error("Failed to close switch during SetUp after SetActiveSwitchPort failure: ", closeErr)
			}
			s.Fatal("Failed to set port during SetUp: ", err)
		} else {
			f.PortNum = portUsed
		}
	} else {
		if err := f.TestSwitch.Close(ctx); err != nil {
			s.Error("Failed to close switch: ", err)
		}
		s.Fatal("Port number is not set in the fixture with typec.SwitchPort var")
	}
	testing.ContextLogf(ctx, "Port number set to %d", f.PortNum)

	// Ensure ports are disabled initially as a baseline.
	if err := f.TestSwitch.DisablePorts(ctx); err != nil {
		// Attempt to close the switch if disabling ports fails during setup.
		if closeErr := f.TestSwitch.Close(ctx); closeErr != nil {
			s.Error("Failed to close switch during SetUp after DisablePorts failure: ", closeErr)
		}
		s.Fatal("Failed to disable ports during SetUp: ", err)
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
	testing.ContextLog(ctx, "Ports disabled during typecSwitch Reset")

	// Attempt to reset to USB3 mode as a common default.
	if err := f.TestSwitch.EnterMode(ctx, usbswitch.Usb3Mode); err != nil {
		return errors.Wrap(err, "failed to reset utc to USB3 mode during fixture reset")
	}
	return nil
}

// PreTest is called before each test.
func (f *SwitchFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest is called after each test.
func (f *SwitchFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

// newSwitch returns an interface for the usb switch.
func newSwitch(ctx context.Context, s *testing.FixtState) (usbswitch.Switch, error) {
	if utcURI, utcPresent := s.Var("typec.UtcUri"); utcPresent {
		var pasitTopology *labapi.PasitHost
		if dutConfig, err := s.ChromeOSDUTLabConfig(""); err == nil {
			if dutConfig.GetChromeos().GetPasitHost() != nil {
				pasitTopology = dutConfig.GetChromeos().GetPasitHost()
				s.Log("Loaded DUT info from lab config")
			}
		}
		utcSerial, _ := s.Var("typec.UtcSerial")
		utcObj, err := utc.New(ctx, utcURI, utcSerial, pasitTopology)
		if err != nil {
			return nil, errors.Wrap(err, "failed to create utc object")
		}
		return utcObj, nil

	} else if mcciSerial, mcciPresent := s.Var("typec.McciSerial"); mcciPresent {
		path, _ := s.Var("typec.McciPath")

		mcciObj, err := mcci.New(mcciSerial, path)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get MCCI switch handle")
		}

		return mcciObj, nil
	}

	return nil, errors.New("failed to parse usb switch device arguments")
}
