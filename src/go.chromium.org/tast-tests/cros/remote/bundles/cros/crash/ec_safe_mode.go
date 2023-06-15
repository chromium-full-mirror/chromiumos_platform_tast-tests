// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECSafeMode,
		Desc: "Verify EC System Safe Mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"robbarnes@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_unstable"},
		Timeout:      10 * time.Minute,
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"device_crash", "ec_crash", "pstore", "reboot", "no_qemu"},
		HardwareDeps: hwdep.D(hwdep.ECFeatureSystemSafeMode()),
	})
}

const (
	panicDataFlagFrameValid                = 1 << iota
	panicDataFlagOldConsole                = 1 << iota
	panicDataFlagOldHostcmd                = 1 << iota
	panicDataFlagOldHostevent              = 1 << iota
	panicDataFlagTruncated                 = 1 << iota
	panicDataFlagSafeModeStarted           = 1 << iota
	panicDataFlagSafeModeFailPreconditions = 1 << iota
)

// ECSafeMode verifies that EC safe mode runs and Kernel syncs logs
func ECSafeMode(ctx context.Context, s *testing.State) {
	var timerInfoLine string
	d := s.DUT()

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// This is a bit delicate. If the test fails _before_ we panic the machine,
	// we need to do TearDown then, and on the same connection (so we can close Chrome).
	//
	// If it fails to reconnect, we do not need to clean these up.
	//
	// Otherwise, we need to re-establish a connection to the machine and
	// run TearDown.
	defer func() {
		s.Log("Cleaning up")
		if cl != nil {
			cl.Close(cleanupCtx)
		}
	}()

	if out, err := d.Conn().CommandContext(ctx, "logger", "Running ECSafeMode").CombinedOutput(); err != nil {
		s.Logf("WARNING: Failed to log info message: %s", out)
	}

	// Sync filesystem to minimize impact of the panic on other tests
	if out, err := d.Conn().CommandContext(ctx, "sync").CombinedOutput(); err != nil {
		s.Fatalf("Failed to sync filesystems: %s", out)
	}

	// When we crash, these connections will break.
	cl.Close(ctx)
	cl = nil

	// Rebooting the EC can make servod fail if the CCD watchdog is not removed.
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove CCD watchdog: ", err)
	}

	s.Log("Start EC log capture")
	if err := h.Servo.SetOnOff(ctx, servo.ECUARTCapture, servo.On); err != nil {
		s.Fatal("Failed to capture EC UART: ", err)
	}
	defer func() {
		s.Log("Stop EC capture")
		if err := h.Servo.SetOnOff(ctx, servo.ECUARTCapture, servo.Off); err != nil {
			s.Fatal("Failed to disable capture EC UART: ", err)
		}
	}()

	s.Log("Running 'timerinfo' command to get marker in EC log")
	if err := h.Servo.RunECCommand(ctx, "timerinfo"); err != nil {
		s.Fatal("Failed to run EC command: ", err)
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		lines, err := h.Servo.GetQuotedString(ctx, servo.ECUARTStream)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to read UART"))
		}
		for _, l := range strings.Split(lines, "\r\n") {
			timerInfoLine = regexp.MustCompile(`Time:.*`).FindString(l)
			if timerInfoLine == "" {
				continue
			}
			s.Log("Found timer info line: ", timerInfoLine)
			return nil
		}
		return errors.New("Timer info not found")
	}, &testing.PollOptions{Interval: 100 * time.Millisecond, Timeout: time.Second}); err != nil {
		s.Error("Failed to find timer info line from EC log: ", err)
	}

	s.Log("Running crash command")
	// This should reboot the device
	if err := h.Servo.RunECCommand(ctx, "crash divzero"); err != nil {
		s.Fatal("Failed to run EC command: ", err)
	}

	s.Log("Waiting for DUT to become unreachable")
	if err := d.WaitUnreachable(ctx); err != nil {
		s.Fatal("Failed to wait for DUT to become unreachable: ", err)
	}
	s.Log("DUT became unreachable (as expected)")

	s.Log("Reconnecting to DUT")
	if err := d.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}
	s.Log("Reconnected to DUT")

	cl, err = rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	/* Verify panicinfo flags */
	panicinfo, err := linuxssh.ReadFile(cleanupCtx, d.Conn(), "/sys/kernel/debug/cros_ec/panicinfo")
	if err != nil {
		s.Fatal("Failed to read cros_ec.previous: ", err)
	}
	if len(panicinfo) < 3 {
		s.Fatal("Panic info length is too short")
	}
	panicinfoFlags := panicinfo[2]
	if panicinfoFlags&panicDataFlagTruncated != 0 {
		s.Error("PANIC_DATA_FLAG_TRUNCATED is set in panic info flags")
	}
	if panicinfoFlags&panicDataFlagSafeModeStarted == 0 {
		s.Error("PANIC_DATA_FLAG_SAFE_MODE_STARTED is not set in panic info flags")
	}
	if panicinfoFlags&panicDataFlagSafeModeFailPreconditions != 0 {
		s.Error("PANIC_DATA_FLAG_SAFE_MODE_FAIL_PRECONDITIONS is set in panic info flags")
	}
	/* Get cros_ec log from previous boot */
	ecPreviousLog, err := linuxssh.ReadFile(cleanupCtx, d.Conn(), "/var/log/cros_ec.previous")
	if err != nil || len(ecPreviousLog) == 0 {
		s.Fatal("Failed to read cros_ec.previous: ", err)
	}
	/* Verify timer info line is present */
	if !strings.Contains(string(ecPreviousLog), timerInfoLine) {
		s.Fatalf("Time info line %q is missing from cros_ec.previous", timerInfoLine)
	}
}
