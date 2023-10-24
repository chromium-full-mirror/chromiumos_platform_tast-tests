// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    ServoGSCFlags,
		Desc:    "Reads GSC flags via the servo interface",
		Timeout: 8 * time.Minute,
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Attr:         []string{"group:labqual_informational"},
		Fixture:      fixture.NormalMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

const (
	cr50Reboot        string = "cr50_reboot"
	cr50CcdStateFlags string = "cr50_ccd_state_flags"
	cr50Servo         string = "cr50_servo"
)

// ServoGSCFlags has the Diagnostics and checks on GSC by Servo
func ServoGSCFlags(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequirePlatform(ctx); err != nil {
		s.Fatal("Failed to require platform: ", err)
	}
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	if err := h.Servo.RequireCCD(ctx); err != nil {
		s.Fatal("Servo does not have CCD: ", err)
	}

	// read cr50_ccd_level
	if val, err := h.Servo.GetString(ctx, servo.GSCCCDLevel); err != nil {
		s.Fatal("Failed to get gsc_ccd_level")
	} else if val != servo.Open {
		s.Logf("CCD is not open, got %q. Attempting to unlock", val)
		if err := h.Servo.SetString(ctx, servo.CR50Testlab, servo.Open); err != nil {
			s.Fatal("Failed to unlock CCD")
		}
		s.Logf("GSC CCD Level from Servo : %s", val)
	}

	// read servo type
	servoType, err := h.Servo.GetServoType(ctx)
	if err != nil {
		s.Fatal("Failed to get servo type: ", err)
	}
	s.Logf("Servo type : %s", servoType)

	// read cr50_testlab state
	testLabState, err := h.GetTestlabState(ctx)
	if err != nil {
		s.Fatal("Failed to get cr50 test lab state: ", err)
	}
	s.Logf("cr50 test lab state : %s", testLabState)

	// check if servod has cr50_reboot control
	hasCR50Reboot, err := h.Servo.HasControl(ctx, cr50Reboot)
	if err != nil {
		s.Fatal("Failed to get cr50_reboot control state: ", err)
	}
	s.Log("Servo has cr50_reboot control : ", hasCR50Reboot)

	// read ccd serial number
	ccdSerial, err := h.Servo.GetCCDSerial(ctx)
	if err != nil {
		s.Fatal("Failed to get servo serials: ", err)
	}
	s.Logf("CCD serial read from Servo : %s", ccdSerial)

	// read cr50_ccd_state_flags
	val, err := h.Servo.HasControl(ctx, cr50CcdStateFlags)
	if err != nil {
		s.Fatal("Failed to get cr50_ccd_state_flags")
	}
	s.Log("cr50_ccd_state_flags can be read from Servo : ", val)

	// read cr50_ccd_state_flags
	val, err = h.Servo.HasControl(ctx, cr50Servo)
	if err != nil {
		s.Fatal("Failed to get cr50_servo")
	}
	s.Log("cr50_servo can be read from Servo: ", val)

	// read pwr_button value
	expectedPwrButtonVal := "release"
	pwrBtnVal, err := h.Servo.GetString(ctx, servo.PwrButtonCtrl)
	if err != nil {
		s.Fatal("Failed to get pwr_button value")
	} else if pwrBtnVal != expectedPwrButtonVal {
		s.Fatalf("Pwr button value read from Servo not matching the expected value, expected: %s; got: %s", expectedPwrButtonVal, pwrBtnVal)
	}
	s.Logf("Pwr button value read from Servo : %s", expectedPwrButtonVal)
}
