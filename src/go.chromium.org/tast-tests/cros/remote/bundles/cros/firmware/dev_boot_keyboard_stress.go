// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
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
	defaultDevBootKeyboardStressIterations = 100
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DevBootKeyboardStress,
		Desc: "Check keyboard is enabled during developer firmware screen",
		Contacts: []string{
			"chromeos-faft@google.com",
			"ecgh@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.DevMode,
		Timeout:      3 * time.Hour,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.X86()),
		LacrosStatus: testing.LacrosVariantUnneeded,
		Vars:         []string{"iterations"},
	})
}

// DevBootKeyboardStress test checks that the EC thinks the keyboard is
// enabled while depthcharge shows the dev mode screen before booting
// the OS. This test is only intended to run on x86 devices using the
// 8042 EC keyboard and with depthcharge configured to show the dev
// mode screen for the default 30 seconds.
func DevBootKeyboardStress(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Require config")
	}
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Require servo")
	}

	iterations := defaultDevBootKeyboardStressIterations
	if iterStr, ok := s.Var("iterations"); ok {
		iterInt, err := strconv.Atoi(iterStr)
		if err != nil {
			s.Fatal("Failed to parse iterations value: ", iterStr)
		}
		iterations = iterInt
	}

	if !checkKeyboard(ctx, s, h.Servo) {
		s.Fatal("8042 keyboard must be enabled to start")
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
		ecOutput, err := doDevBootKeyboardStressIteration(ctx, s, h, i)
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
		// Check that log shows waiting at dev screen.
		matches := regexp.MustCompile(`developer_mode_action: Booting default target after 30s`).FindAllStringSubmatch(fwLog, -1)
		if len(matches) != 1 {
			s.Error("Failed to find dev screen line in fw log: ", matches)
		}
		timestamps, err := h.Reporter.GetCBMEMTimestamps(ctx)
		if err != nil {
			s.Error("Failed to read FW timestamps: ", err)
		}
		os.WriteFile(filepath.Join(logDir, "timestamps.txt"), []byte(timestamps), 0666)
		linuxssh.GetFile(ctx, h.DUT.Conn(), "/sys/fs/pstore/", filepath.Join(logDir, "pstore"), linuxssh.PreserveSymlinks)
		linuxssh.GetFile(ctx, h.DUT.Conn(), "/var/log/eventlog.txt", filepath.Join(logDir, "eventlog.txt"), linuxssh.PreserveSymlinks)
	}
}

func doDevBootKeyboardStressIteration(ctx context.Context, s *testing.State, h *firmware.Helper, i int) (string, error) {
	var ecOutput strings.Builder

	bootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get boot id")
	}
	s.Logf("Reboot attempt %d (boot id %s)", i, bootID)

	msg := fmt.Sprintf("DevBootKeyboardStress iteration %d boot id %s", i, bootID)
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

	s.Log("Power on to developer firmware screen")
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurShortPress); err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to press power")
	}

	// Check the keyboard is enabled while the developer screen is up.
	// GoBigSleepLint: Wait long enough for screen to be shown.
	testing.Sleep(ctx, 20*time.Second)
	checkKeyboard(ctx, s, h.Servo)
	// GoBigSleepLint: Check keyboard again while screen is still shown.
	testing.Sleep(ctx, 5*time.Second)
	checkKeyboard(ctx, s, h.Servo)
	// GoBigSleepLint: Wait for screen to timeout and boot to OS.
	testing.Sleep(ctx, 25*time.Second)

	devScreenOutput, err := h.GetECConsoleOutputWithComment(ctx, "done waiting for boot")
	if err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get EC console")
	}
	ecOutput.WriteString(devScreenOutput)

	// Expect to see "scancode set to 2" twice (at start and end of dev screen).
	matches := regexp.MustCompile(`scancode set to (\d+)`).FindAllStringSubmatch(devScreenOutput, -1)
	if len(matches) != 2 {
		s.Error("Failed to find 2 scancode lines: ", matches)
	}
	for _, v := range matches {
		if v[1] != "2" {
			s.Error("Wrong scancode: ", matches)
		}
	}

	connectCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(connectCtx); err != nil {
		return ecOutput.String(), errors.Wrap(err, "device did not come back up")
	}

	if newBootID, err := h.Reporter.BootID(ctx); err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get boot id")
	} else if newBootID == bootID {
		return ecOutput.String(), errors.New("boot id did not change")
	} else {
		bootID = newBootID
	}

	out, err = h.GetECConsoleOutputWithComment(ctx, "done waiting for connect")
	if err != nil {
		return ecOutput.String(), errors.Wrap(err, "failed to get EC console")
	}
	ecOutput.WriteString(out)

	return ecOutput.String(), nil
}

func checkKeyboard(ctx context.Context, s *testing.State, svo *servo.Servo) bool {
	kbEn, err := svo.RunECCommandGetOutput(ctx, "8042 kbd", []string{`Enabled: (\d)`})
	if err != nil {
		s.Error("Failed 8042 kbd: ", err)
		return false
	}
	if len(kbEn) == 0 || kbEn[0][1] != "1" {
		s.Error("Keyboard not enabled: ", kbEn)
		return false
	}
	s.Log("Keyboard is enabled")
	return true
}
