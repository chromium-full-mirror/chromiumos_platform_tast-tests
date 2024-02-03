// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// Default number of boot iterations. Can be overridden by var.
	defaultECRebootEFS2Iterations = 20
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECRebootEFS2,
		Desc: "Check EFS2 jump to EC RW on EC reboot",
		Contacts: []string{
			"chromeos-faft@google.com",
			"ecgh@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		Timeout:      30 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "gsc_reboot",
			Val:  true,
		}, {
			Name: "no_gsc_reboot",
			Val:  false,
		}},
		Vars: []string{"iterations"},
	})
}

// ECRebootEFS2 tests EFS2 EC hash verification by GSC on either GSC
// reboot or EC reboot. This test is only intended to run on devices
// with EFS2 enabled.
func ECRebootEFS2(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Require config")
	}
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Require servo")
	}

	iterations := defaultECRebootEFS2Iterations
	if iterStr, ok := s.Var("iterations"); ok {
		iterInt, err := strconv.Atoi(iterStr)
		if err != nil {
			s.Fatal("Failed to parse iterations value: ", iterStr)
		}
		iterations = iterInt
	}

	closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
	if err != nil {
		s.Fatal("Failed to capture EC UART: ", err)
	}
	defer func() {
		if err := closeUART(ctx); err != nil {
			s.Fatal("Failed to cancel capture EC UART: ", err)
		}
	}()

	// CCD must be open to enable the GSC or EC reboot console commands used below.
	if err := h.OpenCCD(ctx, false, false); err != nil {
		s.Fatal("Failed to open CCD: ", err)
	}

	// Try to open CCD at the end of the test in case something after this test needs it.
	defer func() {
		h.Servo.RunCR50Command(ctx, "ccd testlab open")
	}()

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get output dir: ", err)
	}
	// Use timestamp to separate logs from Tast retries (b/297937181).
	outSubDir := time.Now().Format("20060102-150405")

	s.Logf("Starting %d reboot attempts ", iterations)
	for i := 0; i < iterations; i++ {
		ctx, cancel := context.WithTimeout(ctx, 5*time.Minute)
		defer cancel()
		ecOutput, err := doECRebootEFS2Iteration(ctx, s, h, i)
		if err != nil {
			s.Errorf("Error on attempt %d: %v", i, err)
			// GoBigSleepLint: Wait after unexpected error.
			testing.Sleep(ctx, 10*time.Second)
			// Try a power button push since it might power the DUT up.
			h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurShortPress)
			// GoBigSleepLint: Wait after unexpected error.
			testing.Sleep(ctx, 10*time.Second)
			errorOutput, _ := h.GetECConsoleOutputWithComment(ctx, "after error")
			ecOutput = ecOutput + errorOutput
		}
		logDir := filepath.Join(outDir, outSubDir, strconv.Itoa(i))
		os.MkdirAll(logDir, 0755)
		os.WriteFile(filepath.Join(logDir, "ec_console.txt"), []byte(ecOutput), 0666)
		ctx, cancel = context.WithTimeout(ctx, 30*time.Second)
		defer cancel()
		fwLog, err := h.Reporter.GetCBMEMLogs(ctx)
		if err != nil {
			s.Error("Failed to read FW logs: ", err)
		}
		os.WriteFile(filepath.Join(logDir, "fw_log.txt"), []byte(fwLog), 0666)
		timestamps, err := h.Reporter.GetCBMEMTimestamps(ctx)
		if err != nil {
			s.Error("Failed to read FW timestamps: ", err)
		}
		os.WriteFile(filepath.Join(logDir, "timestamps.txt"), []byte(timestamps), 0666)
		linuxssh.GetFile(ctx, h.DUT.Conn(), "/sys/fs/pstore/", filepath.Join(logDir, "pstore"), linuxssh.PreserveSymlinks)
		linuxssh.GetFile(ctx, h.DUT.Conn(), "/var/log/eventlog.txt", filepath.Join(logDir, "eventlog.txt"), linuxssh.PreserveSymlinks)
	}
}

func doECRebootEFS2Iteration(ctx context.Context, s *testing.State, h *firmware.Helper, i int) (string, error) {
	var ecOutput strings.Builder
	gscReboot := s.Param().(bool)

	bootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get boot id")
	}
	s.Logf("Reboot attempt %d (boot id %s)", i, bootID)

	msg := fmt.Sprintf("ECRebootEFS2 iteration %d boot id %s", i, bootID)
	if out, err := h.DUT.Conn().CommandContext(ctx, "logger", msg).CombinedOutput(); err != nil {
		return ecOutput.String(), errors.Errorf("failed to write to log: %s", out)
	}
	// Sync to preserve /var/log/messages for the current boot in case of unexpected crash.
	if out, err := h.DUT.Conn().CommandContext(ctx, "sync").CombinedOutput(); err != nil {
		return ecOutput.String(), errors.Errorf("failed to sync filesystem: %s", out)
	}
	if err := powercontrol.Shutdown(ctx, h.DUT); err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to shutdown")
	}
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 30*time.Second, "S5", "G3"); err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get S5/G3 powerstate")
	}

	out, err := h.GetECConsoleOutputWithComment(ctx, msg)
	if err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get EC console")
	}
	ecOutput.WriteString(out)

	s.Log("Rebooting")
	if gscReboot {
		// Use GSC console to reboot GSC.
		if err := h.Servo.RunCR50Command(ctx, "reboot"); err != nil {
			return ecOutput.String(), errors.Wrap(err, "failed to reboot GSC")
		}
	} else {
		// Use GSC console to reboot EC.
		if err := h.Servo.RunCR50Command(ctx, "ecrst pulse"); err != nil {
			return ecOutput.String(), errors.Wrap(err, "failed to ecrst pulse")
		}
	}
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 30*time.Second, "S0"); err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get S0 powerstate")
	}
	connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return ecOutput.String(), errors.Wrap(err, "device did not come back up")
	}

	rebootOutput, err := h.GetECConsoleOutputWithComment(ctx, "done waiting for boot")
	if err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get EC console")
	}
	ecOutput.WriteString(rebootOutput)

	// Check EFS2 succeeds without retry.
	if strings.Count(rebootOutput, "VB Verifying hash") != 1 {
		s.Error("Missing VB Verifying hash")
	}
	if strings.Count(rebootOutput, "VB Received 0xec00") != 1 {
		s.Error("Missing VB Received 0xec00")
	}
	if strings.Count(rebootOutput, "VB Timeout") != 0 {
		s.Error("VB Timeout")
	}

	if newBootID, err := h.Reporter.BootID(ctx); err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get boot id")
	} else if newBootID == bootID {
		return ecOutput.String(), errors.New("boot id did not change")
	} else {
		bootID = newBootID
	}
	return ecOutput.String(), nil
}
