// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
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
		// TODO(b/194910901): When stable, move to firmware_pd.
		Attr:         []string{"group:firmware", "firmware_pd_unstable"},
		Vars:         []string{"servo"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      5 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "normal",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityStandard,
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "flipcc",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOn,
			},
		}, {
			Name: "dtsoff",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityStandard,
				DTS: firmware.DTSModeOff,
			},
		}, {
			Name: "flipcc_dtsoff",
			Val: firmware.PDTestParams{
				CC:  firmware.CCPolarityFlipped,
				DTS: firmware.DTSModeOff,
			},
		}},
	})
}

func reconnectPD(ctx context.Context, svo *servo.Servo) error {
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
	if err := svo.SetCC(ctx, servo.On); err != nil {
		return errors.Wrap(err, "failed to set CC on")
	}

	// Wait for PD connection to be reestablished.
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
	return nil
}

func setServoPowerRole(ctx context.Context, svo *servo.Servo, role servo.PDRoleValue) error {
	if err := svo.SetPDRole(ctx, role); err != nil {
		return err
	}

	// The above would be sufficient, except that servod does not enable PD with
	// the Servo as Sink by default. Enable it directly if needed.
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
	if err := firmware.SetupPDTester(ctx, h, p.CC, p.DTS, p.RequiredPort); err != nil {
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
	if err := setServoPowerRole(ctx, h.Servo, servo.PDRoleSnk); err != nil {
		s.Fatal("Could not set servo power role to Sink: ", err)
	}
	if err := reconnectPD(ctx, h.Servo); err != nil {
		s.Fatal("Could not toggle PD connection: ", err)
	}
}

func cleanup(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

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
		if pdState.PEStateName != "PD_STATE_SRC_READY" {
			return errors.New("Servo DUT port is not ready")
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}); err != nil {
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
