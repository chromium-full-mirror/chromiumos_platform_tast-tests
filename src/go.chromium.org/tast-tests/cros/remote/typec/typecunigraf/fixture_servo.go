// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package typecunigraf

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "typecUnigrafAndServo",
		Desc:     "Initializes Unigraf and deactivates Servo if present by setting it to SNK role.",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent:    "b:958036",
		Impl:            &UnigrafAndServoFixture{},
		SetUpTimeout:    30 * time.Second, // Includes parent SetUp and Servo init.
		ResetTimeout:    15 * time.Second, // Parent Reset handles Unigraf.
		TearDownTimeout: 30 * time.Second, // Includes Servo teardown and parent TearDown.
		Vars: []string{
			"servo", // Optional: Servo spec if a servo is in the testbed.
		},
		Parent: "typecUnigraf", // Depends on the typecUnigraf fixture.
	})
}

// UnigrafAndServoFixture holds the state for this composite fixture.
type UnigrafAndServoFixture struct {
	// Data from the parent typecUnigraf fixture.
	parentData *FixtureData

	// Servo related state.
	servoProxy  *servo.Proxy
	servoActive bool // True if servo was found and configured.
}

// SetUp initializes the Unigraf (via parent) and then configures Servo.
func (f *UnigrafAndServoFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// Get the Unigraf controller from the parent fixture.
	parentData, ok := s.ParentValue().(*FixtureData)
	if !ok {
		s.Fatal("Failed to get Unigraf data from parent fixture")
	}
	f.parentData = parentData

	dut := s.DUT()

	// Ensure DUT is connected.
	if !dut.Connected(ctx) {
		s.Log("Attempting to connect to DUT in SetUp")
		waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()
		if err := dut.WaitConnect(waitConnectCtx); err != nil {
			s.Fatalf("Failed to reconnect to the DUT: %v", err)
		}
		s.Log("Connected to DUT")
	}

	// Configure Servo if present.
	if servoSpec, present := s.Var("servo"); present {
		s.Log("Servo specified, configuring it as SNK.")
		f.servoActive = true
		pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
		if err != nil {
			s.Fatal("Failed to connect to servo: ", err)
		}
		f.servoProxy = pxy
		svo := pxy.Servo()

		// Configure Servo to be OK with CCD losing connection.
		if err := svo.RemoveCCDWatchdogs(ctx); err != nil {
			s.Fatal("Failed to switch CCD watchdog off: ", err)
		}

		// Set Servo to SNK role so it doesn't interfere as a source.
		if err := svo.ServoCcSnk(ctx); err != nil {
			s.Fatal("Failed to set servo to SNK role: ", err)
		}
		s.Log("Servo set to SNK role.")

		// If Unigraf setup involves Ethernet via Servo, DUT might disconnect and reconnect.
		s.Log("Waiting for DUT to be connectable after Servo/Unigraf setup")
		connectCtx, connectCancel := context.WithTimeout(ctx, 30*time.Second)
		defer connectCancel()
		if err := dut.WaitConnect(connectCtx); err != nil {
			s.Logf("DUT not reachable after Servo/Unigraf setup (this might be expected for some setups): %v", err)
		} else {
			s.Log("DUT is connectable.")
		}
	} else {
		s.Log("No servo specified, proceeding with Unigraf only.")
		f.servoActive = false
	}

	// Return the FixtureData from the parent (which contains the Unigraf controller).
	return f.parentData
}

// TearDown restores Servo state if it was active and closes the proxy.
func (f *UnigrafAndServoFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.servoActive && f.servoProxy != nil {
		s.Log("Restoring Servo state in TearDown.")
		svo := f.servoProxy.Servo()
		// Attempt to set Servo back to SRC role.
		if err := svo.ServoCcSrc(ctx, true); err != nil {
			s.Errorf("Failed to set servo back to SRC role: %v", err)
		} else {
			s.Log("Servo set back to SRC role.")
		}

		f.servoProxy.Close(ctx)
		s.Log("Servo proxy closed.")
	}
}

// Reset is called after each test. Parent's Reset handles Unigraf.
func (f *UnigrafAndServoFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest is called before each test.
func (f *UnigrafAndServoFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest is called after each test.
func (f *UnigrafAndServoFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
