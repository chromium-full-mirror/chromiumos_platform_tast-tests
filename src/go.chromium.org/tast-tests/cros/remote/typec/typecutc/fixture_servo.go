// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typecutc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "typecUtcAndServo",
		Desc:     "Initializes Utc and deactivates Servo if present by setting it to SNK role",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent:    "b:958036",
		Impl:            &UtcAndServoFixture{},
		PreTestTimeout:  15 * time.Minute,
		SetUpTimeout:    15 * time.Minute, // Includes parent SetUp and Servo init.
		ResetTimeout:    15 * time.Minute, // Parent Reset handles Utc.
		TearDownTimeout: 15 * time.Minute, // Includes Servo teardown and parent TearDown.
		Vars: []string{
			"servo", // Optional: Servo spec if a servo is in the testbed.
		},
		Parent: "typecUtc", // Depends on the typecutc fixture.
	})
}

// UtcAndServoFixture holds the state for this composite fixture.
type UtcAndServoFixture struct {
	// Data from the parent typecutc fixture.
	parentData *FixtureData

	// Servo related state.
	servoProxy *servo.Proxy

	// Servo test configured by the fixture.
	servoTestConfigured bool
}

// SetUp initializes the utc (via parent) and then configures Servo.
func (f *UtcAndServoFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Get the utc controller from the parent fixture.
	parentData, ok := s.ParentValue().(*FixtureData)
	if !ok {
		s.Fatal("Failed to get utc data from parent fixture")
	}
	f.parentData = parentData

	dut := s.DUT()

	// Ensure DUT is connected.
	if !dut.Connected(ctx) {
		s.Log("Attempting to connect to DUT in SetUp")
		waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := dut.WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to the DUT: ", err)
		}
		s.Log("Connected to DUT")
	}

	f.servoTestConfigured = false

	// If servo is present prepare a proxy for it.
	if servoSpec, present := s.Var("servo"); present {
		s.Log("Servo specified, configuring it as SNK")
		pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
		if err != nil {
			s.Fatal("Failed to connect to servo: ", err)
		}
		f.servoProxy = pxy
	} else {
		s.Log("No servo specified, proceeding with utc only")
	}

	// Return the FixtureData from the parent (which contains the utc controller).
	return f.parentData
}

// TearDown restores Servo state if it was active and closes the proxy.
func (f *UtcAndServoFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.servoProxy == nil {
		return
	}

	s.Log("Restoring Servo state in TearDown")
	svo := f.servoProxy.Servo()
	// Attempt to set Servo back to SRC role.
	if err := svo.ServoCcSrc(ctx, true); err != nil {
		s.Error("Failed to set servo back to SRC role: ", err)
	} else {
		s.Log("Servo set back to SRC role")
	}

	f.servoProxy.Close(ctx)
	s.Log("Servo proxy closed")

	// GoBigSleepLint: Wait for servo control to take effect. Closing the proxy
	// will reenable the CCD watchdog so we need to wait for it to take effect.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		f.servoProxy = nil
		f.servoTestConfigured = false
		s.Fatal("Failed to sleep after CCD watchdog on: ", err)
	}
}

// Reset is called after each test. Parent's Reset handles Utc.
func (f *UtcAndServoFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest is called before each test.
func (f *UtcAndServoFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Skip the config if servo is not present or config was already done.
	if f.servoProxy == nil || f.servoTestConfigured {
		return
	}

	dut := s.DUT()
	svo := f.servoProxy.Servo()

	// Configure Servo to be OK with CCD losing connection.
	if err := svo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to switch CCD watchdog off: ", err)
	}

	// GoBigSleepLint: Wait for servo control to take effect.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		s.Fatal("Failed to sleep after CCD watchdog off: ", err)
	}

	// Set Servo to SNK role so it doesn't interfere as a source.
	if err := svo.ServoCcSnk(ctx); err != nil {
		s.Fatal("Failed to set servo to SNK role: ", err)
	}
	s.Log("Servo set to SNK role")

	// If utc setup involves Ethernet via Servo, DUT might disconnect and reconnect.
	s.Log("Waiting for DUT to be connectable after Servo/utc setup")
	connectCtx, connectCancel := context.WithTimeout(ctx, 2*time.Minute)
	defer connectCancel()
	if err := dut.WaitConnect(connectCtx); err != nil {
		s.Log("DUT not reachable after Servo/utc setup (this might be expected for some setups): ", err)
	} else {
		s.Log("DUT is connectable")
	}

	f.servoTestConfigured = true
}

// PostTest is called after each test.
func (f *UtcAndServoFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
