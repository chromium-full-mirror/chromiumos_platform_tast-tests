// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECPDConnect,
		Desc: "USB PD connection test",
		Contacts: []string{
			"chromeos-faft@google.com", // Owning team list
			"alevkoy@chromium.org",     // Test author
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Data:         []string{firmware.ConfigFile},
		Vars:         []string{"servo"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      6 * time.Minute,
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw", "firmware_bios_pdc"},
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "dts",
			Val: firmware.PDTestParams{
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "flipcc_dts",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOn,
			},
		}},
	})
}

func reconnectPD(ctx context.Context, svo *servo.Servo) error {
	testing.ContextLog(ctx, "Disable Servo PD communication (cc off)")
	if err := svo.SetCC(ctx, servo.Off); err != nil {
		return errors.Wrap(err, "failed to switch off CC")
	}
	// Wait for VBUS to discharge after disconnecting. See
	// go.chromium.org/tast-tests/cros/remote/bundles/cros/typec/typecutils for
	// rationale and calculation.
	// GoBigSleepLint: There is no Servo control to poll for VBUS discharge.
	if err := testing.Sleep(ctx, 3*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep after CC off")
	}

	testing.ContextLog(ctx, "Enable Servo PD communication (cc on)")
	if err := svo.SetCC(ctx, servo.On); err != nil {
		return errors.Wrap(err, "failed to set CC on")
	}

	// Wait for PD connection to be reestablished.
	testing.ContextLog(ctx, "Waiting for PD communication to re-establish")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdComm, err := svo.GetPDCommunication(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get Servo PD status")
		}
		if servo.OnOffValue(pdComm) != servo.On {
			return errors.New("PD not connected after CC reconnection")
		}
		return nil
	}, &testing.PollOptions{Timeout: 4 * time.Second, Interval: 1 * time.Second}); err != nil {
		return errors.Wrap(err, "timed out waiting for PD connection")
	}

	testing.ContextLog(ctx, "PD communication re-established")
	return nil
}

func setServoPowerRole(ctx context.Context, svo *servo.Servo, role servo.PDRoleValue) error {
	testing.ContextLogf(ctx, "Set Servo PD role to %q", role)
	if err := svo.SetPDRole(ctx, role); err != nil {
		return err
	}

	// The above would be sufficient, except that servod does not enable PD with
	// the Servo as Sink by default. Enable it directly if needed.
	testing.ContextLog(ctx, "Set Servo PD communication on")
	if err := svo.SetPDCommunication(ctx, servo.On); err != nil {
		return err
	}

	return nil
}

// ECPDConnect verifies that the DUT port can establish a PD connection as Sink
// and as Source. This test is based on firmware_PDConnect in Autotest.
func ECPDConnect(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	p := s.Param().(firmware.PDTestParams)

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}
	if err := firmware.SetupPDTester(ctx, h, p, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}
	defer cleanup(ctx, s)

	// Check the PD connection with Servo as Source. This is likely the initial
	// state, in which case this role setting is a no-op.
	if err := setServoPowerRole(ctx, h.Servo, servo.PDRoleSrc); err != nil {
		s.Fatal("Could not set servo power role to Source: ", err)
	}
	// Disconnect and reconnect via Servo CC, and check the PD connection.
	if err := reconnectPD(ctx, h.Servo); err != nil {
		s.Fatal("Could not toggle PD connection: ", err)
	}

	// Check the PD connection with Servo as Sink.
	testing.ContextLog(ctx, "Set Servo to be a sink and repeat test")
	if err := setServoPowerRole(ctx, h.Servo, servo.PDRoleSnk); err != nil {
		s.Fatal("Could not set servo power role to Sink: ", err)
	}
	if err := reconnectPD(ctx, h.Servo); err != nil {
		s.Fatal("Could not toggle PD connection: ", err)
	}
}

func cleanup(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	testing.ContextLog(ctx, "Cleaning up after test")

	// Leave the Servo port as Source with DTS enabled. This is needed to make
	// sure that the DUT can connect to Ethernet after the test.
	if err := h.Servo.SetPDRole(ctx, servo.PDRoleSrc); err != nil {
		s.Fatal("Failed to set Servo power role: ", err)
	}
	if err := h.Servo.SetOnOff(ctx, servo.DTSMode, servo.On); err != nil {
		s.Fatal("Failed to set Servo DTS mode: ", err)
	}

	// Modifying CC and DTS settings causes the Servo DUT port to reset. Wait a bit until
	// it reaches SRC.Ready.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		pdState, err := h.Servo.GetServoPDState(ctx)
		if err != nil {
			return testing.PollBreak(err)
		}
		testing.ContextLogf(ctx, "Servo DUT port PE State: %s", pdState.PEStateName)
		if !pdState.IsSourceReady() {
			return errors.New("Servo DUT port is not ready")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		s.Fatal("timed out waiting for Servo DUT port to be ready: ", err)
	}

	// Restore the network connection. DisconnectDUT is usually a no-op, but in
	// its absence, the next test may fail to connect to the DUT.
	if err := h.DisconnectDUT(ctx); err != nil {
		testing.ContextLog(ctx, "Error closing connections to DUT: ", err)
	}
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
}
