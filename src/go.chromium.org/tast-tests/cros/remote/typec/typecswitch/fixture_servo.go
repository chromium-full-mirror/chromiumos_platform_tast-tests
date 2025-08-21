// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package typecswitch contains the usb switch fixture and helper functions for the tests in the typec directory.
package typecswitch

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:     "typecSwitchAndServo",
		Desc:     "Initializes and provides a Type-C switch (MCCI or utc) interface while deactivating the servo if present.",
		Contacts: []string{"chromeos-usb-champs@google.com", "bszpila@google.com"},
		// ChromeOS > Platform > Connectivity > USB
		BugComponent:    "b:958036",
		Impl:            &SwitchAndServoFixture{},
		SetUpTimeout:    20 * time.Second, // For switch initialization and initial DisablePorts
		ResetTimeout:    15 * time.Second, // For DisablePorts and potential mode reset
		TearDownTimeout: 15 * time.Second, // For closing the switch
		Vars: []string{
			"typec.McciSerial",
			"typec.McciPath",
			"typec.utcUri",
			"typec.SwitchPort",
			"servo",
		},
		Parent: "typecSwitch",
	})
}

// SwitchFixture holds the state for the Type-C switch fixture.
type SwitchAndServoFixture struct {
	sw           *FixtureData
	servoPresent bool
	keepaliveEn  bool
	pxy          *servo.Proxy
}

func (f *SwitchAndServoFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	f.sw = s.ParentValue().(*FixtureData)

	d := s.DUT()

	if !d.Connected(ctx) {
		s.Log("Attempting to connect to DUT")
		waitConnectCtx, cancel := context.WithTimeout(ctx, 2*time.Minute)
		defer cancel()

		if err := s.DUT().WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to the DUT at the beginning: ", err)
		}
		s.Log("Connected to DUT")
	}

	if servoSpec, present := s.Var("servo"); present {
		pxy, err := servo.NewProxy(ctx, servoSpec, d.KeyFile(), d.KeyDir())
		if err != nil {
			s.Fatal("Failed to connect to servo: ", err)
		}
		f.servoPresent = true
		f.pxy = pxy
		svo := pxy.Servo()

		// Configure Servo to be OK with CC being off. Only bother doing this if the device has CCD.
		if ret, err := svo.HasCCD(ctx); err != nil {
			s.Fatal("Failed to check servo CCD capability: ", err)
		} else if ret {
			if err := svo.SetOnOff(ctx, servo.CCDKeepaliveEn, servo.Off); err != nil {
				s.Log("Failed to disable CCD keepalive: ", err)
			} else {
				f.keepaliveEn = true
			}
		}

		// GoBigSleepLint: Wait for servo control to take effect.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			s.Fatal("Failed to sleep after CCD keepalive disable: ", err)
		}

		if err := svo.RemoveCCDWatchdogs(ctx); err != nil {
			s.Fatal("Failed to switch CCD watchdog off: ", err)
		}

		// GoBigSleepLint: Wait for servo control to take effect.
		if err := testing.Sleep(ctx, time.Second); err != nil {
			s.Fatal("Failed to sleep after CCD watchdog off: ", err)
		}

		// Switch servo to snk role.
		if err := svo.ServoCcSnk(ctx); err != nil {
			s.Fatal("Failed to change servo power role to snk: ", err)
		}

		// On utc setup, ethernet is connected by servo, wait for the connection to resume.
		connectCtx, connectCtxCancel := context.WithTimeout(ctx, 10*time.Second)
		if err := d.WaitConnect(connectCtx); err != nil {
			s.Fatal("DUT not reachable in time: ", err)
		}
		connectCtxCancel()
	} else {
		f.servoPresent = false
	}

	return f.sw
}

func (f *SwitchAndServoFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if f.servoPresent {
		svo := f.pxy.Servo()

		// Switch servo to src role.
		if err := svo.ServoCcSrc(ctx, true); err != nil {
			s.Fatal("Failed to change servo power role to src: ", err)
		}

		// GoBigSleepLint: Wait for DTS-on PD negotiation to complete.
		if err := testing.Sleep(ctx, 2500*time.Millisecond); err != nil {
			s.Fatal("Failed to sleep for DTS-on power negotiation: ", err)
		}

		if f.keepaliveEn {
			if err := svo.SetOnOff(ctx, servo.CCDKeepaliveEn, servo.On); err != nil {
				s.Log("Failed to enable CCD keepalive: ", err)
			}
		}
		f.pxy.Close(ctx)
	}
}

func (f *SwitchAndServoFixture) Reset(ctx context.Context) error {
	return nil
}

func (f *SwitchAndServoFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (f *SwitchAndServoFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}
