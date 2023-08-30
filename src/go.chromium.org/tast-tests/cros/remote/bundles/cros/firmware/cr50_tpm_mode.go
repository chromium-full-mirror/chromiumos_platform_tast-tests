// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type tpmMode string

const (
	enabledDefaultTPMMode  tpmMode = "enabled (0)"
	enabledManuallyTPMMode tpmMode = "enabled (1)"
	disabledTPMMode        tpmMode = "disabled (2)"
	unknownTPMMode         tpmMode = ""
)

type tpmControl string

const (
	enableTPMControl  tpmControl = "enable"
	disableTPMControl tpmControl = "disable"
)

type keyLadderState string

const (
	enabledKeyLadderState  keyLadderState = "enabled"
	prodKeyLadderState     keyLadderState = "prod"
	devKeyLadderState      keyLadderState = "dev"
	disabledKeyLadderState keyLadderState = "disabled"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: Cr50TPMMode,
		Desc: "Verify TPM disabling and getting back enabled after reset",
		Contacts: []string{
			"chromeos-faft@google.com",
			"pf@semihalf.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.GSCUART()),
		Fixture:      fixture.NormalMode,
		Timeout:      15 * time.Minute,
		SoftwareDeps: []string{"gsc", "reboot"},
		Params: []testing.Param{
			{
				Name: "long_opt",
				Val:  "--tpm_mode",
			},
			{
				Name: "short_opt",
				Val:  "-m",
			},
		},
	})
}

func Cr50TPMMode(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := initCheck(ctx, h, s); err != nil {
		s.Fatal("Failed to perform successfully initial TPM's status check: ", err)
	}

	s.Log("Disabling TPM")
	if tpmMode, err := setGSCToolTPMMode(ctx, h, s.Param().(string), disableTPMControl); err != nil {
		s.Fatal("Failed to set TPM Mode: ", err)
	} else if tpmMode != disabledTPMMode {
		s.Fatalf("Incorrect TPM Mode: got %q want %q", tpmMode, disabledTPMMode)
	}
	// TPM should not be responsive after disabling it.
	if tpmResponsive, err := isTPMResponsive(ctx, h); err != nil {
		s.Fatal("Failed to check TPM responsiveness: ", err)
	} else if tpmResponsive {
		s.Fatal("TPM should be unresponsive after disabling it")
	}

	// If Cr50 is present then H1 Key Ladder should be disabled.
	if cr50Present, err := isCr50Present(ctx, h); err != nil {
		s.Fatal("Failed to check GSC type: ", err)
	} else if cr50Present {
		if err := checkGSCKeyLadder(ctx, h, disabledKeyLadderState); err != nil {
			s.Fatal("Failed to revoke H1 Key Ladder")
		}
	}

	if err := initCheck(ctx, h, s); err != nil {
		s.Fatal("Failed to perform successfully initial TPM's status check: ", err)
	}

	s.Log("Enabling TPM")
	if tpmMode, err := setGSCToolTPMMode(ctx, h, s.Param().(string), enableTPMControl); err != nil {
		s.Fatal("Failed to set TPM Mode: ", err)
	} else if tpmMode != enabledManuallyTPMMode {
		s.Fatal("Failed to enable TPM")
	}

	// TPM should be responsive after enabling it.
	if tpmResponsive, err := isTPMResponsive(ctx, h); err != nil {
		s.Fatal("Failed to check TPM responsiveness: ", err)
	} else if !tpmResponsive {
		s.Fatal("TPM should be responsive after enabling")
	}
	if cr50Present, err := isCr50Present(ctx, h); err != nil {
		s.Fatal("Failed to check GSC type: ", err)
	} else if cr50Present {
		// On Cr50, disabling should fail after enabling.
		if _, err := setGSCToolTPMMode(ctx, h, s.Param().(string), disableTPMControl); err == nil {
			s.Fatal("Unexpected result in disabling TPM mode, it should fail: ", err)
		}
	}
}

// initCheck resets the DUT and checks if TPM's status is correct after reset.
func initCheck(ctx context.Context, h *firmware.Helper, s *testing.State) error {
	testing.ContextLog(ctx, "Checking TPM status after reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		return errors.Wrap(err, "faild to reset DUT")
	}
	if err := h.WaitConnect(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for device to connect")
	}
	if err := checkGSCTPMMode(ctx, h, enabledDefaultTPMMode); err != nil {
		return errors.Wrap(err, "TPM is not enabled after reset")
	}

	// Check if Key Ladder is enabled (prod & dev values also considered as enabled).
	if err := checkGSCKeyLadder(ctx, h, enabledKeyLadderState, prodKeyLadderState, devKeyLadderState); err != nil {
		return errors.Wrap(err, "failed to restore H1 Key Ladder")
	}
	if err := checkGSCToolTPMMode(ctx, h, s.Param().(string), enabledDefaultTPMMode); err != nil {
		return errors.Wrap(err, "from gsctool perspective TPM is not enabled after reset")
	}
	if tpmResponsive, err := isTPMResponsive(ctx, h); err != nil {
		return errors.Wrap(err, "failed to check TPM responsiveness")
	} else if !tpmResponsive {
		return errors.New("failed to get TPM response")
	}
	return nil
}

// checkGSCTPMMode verifies iff TPM Mode returned by GSC's sysinfo command is equal to expectedMode.
func checkGSCTPMMode(ctx context.Context, h *firmware.Helper, expectedMode tpmMode) error {
	if out, err := h.Servo.RunCR50CommandGetOutput(ctx, "sysinfo", []string{`TPM\s+MODE:\s+(enabled \(\d\)|disabled \(\d\))`}); err != nil {
		return errors.Wrap(err, "failed to run sysinfo command on the GSC")
	} else if out[0][1] != string(expectedMode) {
		return errors.Errorf("incorrect TPM mode info from GSC: got %q want %q", out[0][1], expectedMode)
	}
	return nil
}

// parseGSCToolTPMMode returns atpmModeextracted from a gsctool's tpmModeLine.
func parseGSCToolTPMMode(ctx context.Context, tpmModeLine string) tpmMode {
	modeStr := strings.TrimSpace(strings.Split(tpmModeLine, ":")[1])
	return tpmMode(modeStr)
}

// checkGSCToolTPMMode checks if TPM mode returned by gsctool is equal to the expectedMode.
func checkGSCToolTPMMode(ctx context.Context, h *firmware.Helper, opt string, expectedMode tpmMode) error {
	out, err := h.DUT.Conn().CommandContext(ctx, "gsctool", "-a", opt).Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to run gsctool command")
	}
	outMode := parseGSCToolTPMMode(ctx, string(out))
	if outMode == expectedMode {
		return nil
	}
	return errors.Errorf("incorrect TPM mode info from gsctool: got %q want %q", outMode, expectedMode)
}

// setGSCToolTPMMode uses a gsctool to set the TPM mode.
func setGSCToolTPMMode(ctx context.Context, h *firmware.Helper, opt string, reqControl tpmControl) (tpmMode, error) {
	out, err := h.DUT.Conn().CommandContext(ctx, "gsctool", "-a", opt, string(reqControl)).Output(ssh.DumpLogOnError)
	if err != nil {
		return unknownTPMMode, errors.Wrapf(err, "failed to set TPM mode %s by gsctool command", reqControl)
	}
	outMode := parseGSCToolTPMMode(ctx, string(out))
	return outMode, nil
}

// checkGSCKeyLadder verifies iff Key Ladder State returned by GSC's sysinfo command is present in expectedKeyLadders.
func checkGSCKeyLadder(ctx context.Context, h *firmware.Helper, expectedKeyLadders ...keyLadderState) error {
	out, err := h.Servo.RunCR50CommandGetOutput(ctx, "sysinfo", []string{`Key\s+Ladder:\s+(enabled|prod|dev|disabled)`})
	if err != nil {
		return errors.Wrap(err, "failed to run sysinfo command on the GSC")
	}
	for _, keyLadder := range expectedKeyLadders {
		if out[0][1] == string(keyLadder) {
			return nil
		}
	}
	return errors.Errorf("incorrect Key Ladder State from GSC: got %q want %v", out[0][1], expectedKeyLadders)
}

// isTPMResponsive checks if TPM is responsive by executing tpm_version command.
// This can take 30s.
//
// This function should not be moved to a library package without making it more robust.
func isTPMResponsive(ctx context.Context, h *firmware.Helper) (bool, error) {
	unresponsiveErr := errors.New("TPM is unresponsive")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if out, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", "tpm_version || echo tpm unresponsive").Output(); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to run tpm_version command"))
		} else if string(out) == "tpm unresponsive\n" {
			return unresponsiveErr
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 5 * time.Second}); err != nil {
		if errors.Is(err, unresponsiveErr) {
			return false, nil
		}
		return false, errors.Wrap(err, "timed out waiting for a TPM response")
	}
	return true, nil
}

// isCr50Present checks if DUT has a Cr50.
func isCr50Present(ctx context.Context, h *firmware.Helper) (bool, error) {
	version, err := h.Servo.GetString(ctx, servo.GSCVersion)
	if err != nil {
		return false, errors.Wrap(err, "failed to get GSC version")
	}
	re := regexp.MustCompile(`.*\/(cr50)\_.*`)
	return re.MatchString(version), nil
}
