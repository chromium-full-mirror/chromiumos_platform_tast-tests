// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"path/filepath"
	"strconv"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	watchdogSysfsDir = "/sys/class/watchdog/watchdog0"
	wdiofCardReset   = 32 // WDIOF_CARDRESET in <linux/watchdog.h>
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         APHangDetect,
		Desc:         "Verifies EC-backed AP hang detect watchdog normal pet, magic-close cancel, keepalive timeout warm reset, clean reboot status clearing, and chipset-reset cancellation of cold-reboot escalation",
		Contacts:     []string{"chromeos-faft@google.com", "jpmurphy@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_ec_ro", "firmware_ec_rw"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"watchdog"},
		Timeout:      12 * time.Minute,
	})
}

// ecUARTAccumulator drains and searches the EC UART stream buffer.
type ecUARTAccumulator struct {
	h   *firmware.Helper
	buf strings.Builder
}

func (u *ecUARTAccumulator) drain(ctx context.Context) error {
	u.buf.Reset()
	return testing.Poll(ctx, func(ctx context.Context) error {
		if lines, err := u.h.Servo.GetQuotedString(ctx, servo.ECUARTStream); err != nil {
			return testing.PollBreak(errors.Wrap(err, "draining EC UART"))
		} else if lines != "" {
			return errors.New("EC UART has pending data")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 100 * time.Millisecond})
}

func (u *ecUARTAccumulator) readPending(ctx context.Context) (string, error) {
	err := testing.Poll(ctx, func(ctx context.Context) error {
		if chunk, err := u.h.Servo.GetQuotedString(ctx, servo.ECUARTStream); err != nil {
			return testing.PollBreak(errors.Wrap(err, "reading EC UART"))
		} else if chunk != "" {
			u.buf.WriteString(chunk)
			return errors.New("EC UART has pending data")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 100 * time.Millisecond})
	return u.buf.String(), err
}

func (u *ecUARTAccumulator) waitForAll(ctx context.Context, timeout time.Duration, subs ...string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		chunk, err := u.h.Servo.GetQuotedString(ctx, servo.ECUARTStream)
		if err != nil {
			return errors.Wrap(err, "reading EC UART")
		}
		u.buf.WriteString(chunk)
		captured := u.buf.String()
		for _, s := range subs {
			if !strings.Contains(captured, s) {
				return errors.Errorf("waiting for %q in UART (%q)", s, captured)
			}
		}
		return nil
	}, &testing.PollOptions{Interval: 200 * time.Millisecond, Timeout: timeout})
}

func readSysfsTrimmed(ctx context.Context, h *firmware.Helper, name string) (string, error) {
	path := filepath.Join(watchdogSysfsDir, name)
	out, err := h.DUT.Conn().CommandContext(ctx, "cat", path).Output(ssh.DumpLogOnError)
	if err != nil {
		return "", errors.Wrapf(err, "reading %s", path)
	}
	return strings.TrimSpace(string(out)), nil
}

func pollWatchdogSysfs(ctx context.Context, h *firmware.Helper, timeout time.Duration, want map[string]string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		for name, expected := range want {
			if val, err := readSysfsTrimmed(ctx, h, name); err != nil || val != expected {
				return errors.Errorf("%s = %q (err=%v), want %q", name, val, err, expected)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout, Interval: 200 * time.Millisecond})
}

func readWatchdogBootstatus(ctx context.Context, h *firmware.Helper) (int, error) {
	raw, err := readSysfsTrimmed(ctx, h, "bootstatus")
	if err != nil {
		return 0, err
	}
	return strconv.Atoi(raw)
}

func triggerWatchdogWarmReset(ctx context.Context, h *firmware.Helper, uart *ecUARTAccumulator, timeoutSec int) error {
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		return errors.Wrap(err, "removing CCD watchdogs")
	}
	if err := uart.drain(ctx); err != nil {
		return err
	}
	startScript := fmt.Sprintf("stop daisydog 2>/dev/null; killall -KILL daisydog 2>/dev/null; dmesg -C; /usr/sbin/daisydog -i %d >/dev/null 2>&1 &", timeoutSec)
	if err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", startScript).Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "starting daisydog")
	}
	if err := pollWatchdogSysfs(ctx, h, 5*time.Second, map[string]string{"state": "active"}); err != nil {
		return errors.Wrap(err, "waiting for active watchdog")
	}
	out, err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", "killall -KILL daisydog; sync; dmesg").Output(ssh.DumpLogOnError)
	if err != nil || !strings.Contains(string(out), "watchdog did not stop!") {
		return errors.Errorf("crashing daisydog (err=%v, out=%q)", err, string(out))
	}
	if err := h.CloseRPCConnection(ctx); err != nil {
		return errors.Wrap(err, "closing RPC")
	}
	return uart.waitForAll(ctx, 15*time.Second, "APHD: Triggering reboot")
}

func verifyRebootAndBootstatus(ctx context.Context, h *firmware.Helper, prevBootID string, wantBootstatus int) (string, error) {
	waitCtx, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	if err := h.WaitConnect(waitCtx, firmware.ResetEthernetDongle); err != nil {
		return "", errors.Wrap(err, "reconnecting to DUT")
	}
	newBootID, err := h.Reporter.BootID(ctx)
	if err != nil || newBootID == prevBootID {
		return "", errors.Wrapf(err, "DUT did not reboot (new=%q, prev=%q)", newBootID, prevBootID)
	}
	gotBootstatus, err := readWatchdogBootstatus(ctx, h)
	if err != nil || gotBootstatus != wantBootstatus {
		return "", errors.Wrapf(err, "bootstatus = %d, want %d", gotBootstatus, wantBootstatus)
	}
	return newBootID, nil
}

func APHangDetect(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to create config: ", err)
	}

	closeUART, err := h.Servo.EnableUARTCapture(ctx, servo.ECUARTCapture)
	if err != nil {
		s.Fatal("Failed to enable EC UART capture: ", err)
	}
	defer func() {
		if err := closeUART(cleanupCtx); err != nil {
			s.Error("Failed to close EC UART capture: ", err)
		}
	}()

	defer func() {
		s.Log("Cleanup: restoring power_state:on and daisydog service")
		if !h.DUT.Connected(cleanupCtx) {
			if err := h.Servo.SetPowerState(cleanupCtx, servo.PowerStateOn); err != nil {
				s.Error("Failed to set power state on during cleanup: ", err)
			}
			waitCtx, waitCancel := context.WithTimeout(cleanupCtx, h.Config.DelayRebootToPing)
			defer waitCancel()
			if err := h.WaitConnect(waitCtx, firmware.ResetEthernetDongle); err != nil {
				s.Error("Failed to reconnect to DUT during cleanup: ", err)
				return
			}
		}
		restoreCmd := `if ! status daisydog 2>/dev/null | grep -q "start/running"; then killall -TERM daisydog 2>/dev/null || true; for _ in 1 2 3 4 5; do pidof daisydog >/dev/null || break; sleep 0.1; done; killall -KILL daisydog 2>/dev/null || true; echo V > /dev/watchdog 2>/dev/null || true; ectool hangdetect cancel 2>/dev/null || true; start daisydog; fi`
		if err := h.DUT.Conn().CommandContext(cleanupCtx, "sh", "-c", restoreCmd).Run(ssh.DumpLogOnError); err != nil {
			s.Error("Failed to restore daisydog during cleanup: ", err)
		}
	}()

	uart := &ecUARTAccumulator{h: h}
	if initStatus, err := readWatchdogBootstatus(ctx, h); err != nil {
		s.Fatal("Failed to read initial bootstatus: ", err)
	} else if initStatus != 0 {
		s.Logf("Initial bootstatus is %d; rebooting cleanly to establish 0 baseline", initStatus)
		if err := h.RebootWithSSHCommand(ctx, fwCommon.BootModeNormal); err != nil {
			s.Fatal("Failed baseline clean reboot: ", err)
		}
	}

	// Step 1: Normal pet & magic-close ('V') cancel.
	s.Log("Step 1: Verifying normal watchdog pet and magic-close ('V') disarm")
	step1BootID, err := h.Reporter.BootID(ctx)
	if err != nil {
		s.Fatal("Failed to get BootID before Step 1: ", err)
	}
	step1StartScript := `stop daisydog 2>/dev/null; killall -KILL daisydog 2>/dev/null; /usr/sbin/daisydog -i 6 >/dev/null 2>&1 &`
	if err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", step1StartScript).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Step 1 failed to start daisydog: ", err)
	}
	if err := pollWatchdogSysfs(ctx, h, 5*time.Second, map[string]string{"state": "active", "timeout": "6"}); err != nil {
		s.Fatal("Step 1 failed waiting for daisydog to arm watchdog with timeout=6: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "killall", "-TERM", "daisydog").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Step 1 failed to stop daisydog cleanly: ", err)
	}
	if err := pollWatchdogSysfs(ctx, h, 5*time.Second, map[string]string{"state": "inactive"}); err != nil {
		s.Fatal("Step 1 failed waiting for daisydog SIGTERM to disarm watchdog: ", err)
	}
	if err := uart.drain(ctx); err != nil {
		s.Fatal("Failed to drain EC UART before Step 1 magic-close check: ", err)
	}
	step1MagicCloseScript := `set -e; exec 3>/dev/watchdog; echo 1 >&3; test "$(cat /sys/class/watchdog/watchdog0/state)" = "active"; echo V >&3; exec 3>&-; test "$(cat /sys/class/watchdog/watchdog0/state)" = "inactive"`
	if err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", step1MagicCloseScript).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Step 1 failed to magic-close /dev/watchdog: ", err)
	}
	if err := uart.waitForAll(ctx, 5*time.Second, "APHD: Stop on AP request"); err != nil {
		s.Fatal("Step 1 did not observe EC disarm log after magic close: ", err)
	}
	if afterStep1BootID, err := h.Reporter.BootID(ctx); err != nil || afterStep1BootID != step1BootID {
		s.Fatalf("Step 1 BootID = %q (err=%v), want %q", afterStep1BootID, err, step1BootID)
	}
	if status, err := readWatchdogBootstatus(ctx, h); err != nil || status != 0 {
		s.Fatalf("Step 1 bootstatus = %d (err=%v), want 0", status, err)
	}

	// Step 1b: ectool hangdetect set_timeout / reload / cancel and <5s rejection.
	s.Log("Step 1b: Verifying ectool hangdetect timeout rejection and reload/cancel controls")
	if err := uart.drain(ctx); err != nil {
		s.Fatal("Step 1b failed to drain EC UART: ", err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "ectool", "hangdetect", "set_timeout", "4").Run(); err == nil {
		s.Fatal("Step 1b expected ectool hangdetect set_timeout 4 to fail (<5s), but succeeded")
	}
	if err := uart.waitForAll(ctx, 5*time.Second, "APHD: Invalid reboot timeout: 4s"); err != nil {
		s.Fatal("Step 1b did not observe invalid timeout log on EC UART: ", err)
	}
	if err := uart.drain(ctx); err != nil {
		s.Fatal("Step 1b failed to drain EC UART: ", err)
	}
	step1bScript := `set -e; ectool hangdetect set_timeout 10; ectool hangdetect reload; ectool hangdetect cancel`
	if err := h.DUT.Conn().CommandContext(ctx, "sh", "-c", step1bScript).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Step 1b failed to execute ectool hangdetect reload/cancel: ", err)
	}
	if err := uart.waitForAll(ctx, 5*time.Second, "APHD: reboot timeout: 10(s)", "APHD: Stop on AP request"); err != nil {
		s.Fatal("Step 1b did not observe reload/cancel logs on EC UART: ", err)
	}

	// Step 2: Unexpected petter exit without 'V' -> Stage 1 warm reset & WDIOF_CARDRESET (32).
	s.Log("Step 2: Verifying unexpected petter exit without 'V' triggers EC warm reset and sets bootstatus=32")
	if err := triggerWatchdogWarmReset(ctx, h, uart, 6); err != nil {
		s.Fatal("Step 2 failed to trigger EC watchdog warm reset: ", err)
	}
	step2BootID, err := verifyRebootAndBootstatus(ctx, h, step1BootID, wdiofCardReset)
	if err != nil {
		s.Fatal("Step 2 post-reboot verification failed: ", err)
	}

	// Step 2b: ectool hangdetect get_status / clear_status.
	s.Log("Step 2b: Verifying ectool hangdetect get_status and clear_status")
	if err := uart.drain(ctx); err != nil {
		s.Fatal("Step 2b failed to drain EC UART: ", err)
	}
	if out, err := h.DUT.Conn().CommandContext(ctx, "ectool", "hangdetect", "get_status").Output(ssh.DumpLogOnError); err != nil || !strings.Contains(string(out), "boot status=0") {
		s.Fatalf("Step 2b get_status = %q (err=%v), want 'boot status=0'", string(out), err)
	}
	if err := h.DUT.Conn().CommandContext(ctx, "ectool", "hangdetect", "clear_status").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Step 2b failed to run ectool hangdetect clear_status: ", err)
	}
	if err := uart.waitForAll(ctx, 5*time.Second, "APHD: EC Watchdog status 0", "APHD: Clearing bootstatus"); err != nil {
		s.Fatal("Step 2b did not observe status logs on EC UART: ", err)
	}

	// Step 3: Clean reboot while watchdog is armed (daisydog running) clears bootstatus to 0.
	s.Log("Step 3: Verifying clean reboot while daisydog is running clears bootstatus to 0")
	if err := pollWatchdogSysfs(ctx, h, 15*time.Second, map[string]string{"state": "active"}); err != nil {
		s.Fatal("Step 3 watchdog was not active before clean reboot: ", err)
	}
	if err := h.RebootWithSSHCommand(ctx, fwCommon.BootModeNormal); err != nil {
		s.Fatal("Step 3 clean reboot failed: ", err)
	}
	step3BootID, err := verifyRebootAndBootstatus(ctx, h, step2BootID, 0)
	if err != nil {
		s.Fatal("Step 3 post-reboot verification failed: ", err)
	}

	// Step 4: Chipset reset during Stage 1 warm reset cancels Stage 2 cold-reboot escalation.
	s.Log("Step 4: Verifying chipset reset cancels Stage 2 cold-reboot escalation and preserves bootstatus=32")
	if err := triggerWatchdogWarmReset(ctx, h, uart, 5); err != nil {
		s.Fatal("Step 4 failed to trigger Stage 1 warm reset: ", err)
	}
	if err := uart.waitForAll(ctx, 15*time.Second, "APHD: Stop on Chipset reset"); err != nil {
		s.Fatal("Step 4 did not observe chipset-reset disarm on EC UART: ", err)
	}
	if _, err := verifyRebootAndBootstatus(ctx, h, step3BootID, wdiofCardReset); err != nil {
		s.Fatal("Step 4 post-warm-reset verification failed: ", err)
	}
	captured, err := uart.readPending(ctx)
	if err != nil {
		s.Fatal("Step 4 failed to collect trailing EC UART output: ", err)
	}
	const coldRebootMsg = "APHD: Warm reset failed; forcing cold reboot"
	if strings.Contains(captured, coldRebootMsg) {
		s.Fatalf("Step 4 EC UART output = %q, want no %q", captured, coldRebootMsg)
	}
}
