// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"bytes"
	"compress/gzip"
	"context"
	"io/ioutil"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	crash_service "go.chromium.org/tast-tests/cros/services/cros/crash"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type testParameters struct {
	crashCommand   string
	expectCoredump bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ECSafeMode,
		Desc: "Verify EC System Safe Mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"robbarnes@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_unstable"},
		Timeout:      10 * time.Minute,
		Fixture:      fixture.NormalMode,
		ServiceDeps:  []string{"tast.cros.crash.FixtureService"},
		SoftwareDeps: []string{"device_crash", "ec_crash", "pstore", "reboot", "no_qemu"},
		HardwareDeps: hwdep.D(hwdep.ECFeatureSystemSafeMode()),
		Params: []testing.Param{
			{
				Name: "assert",
				Val: testParameters{
					crashCommand: "crash assert",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.ECFeatureAssertsPanic()),
			},
			{
				Name: "divzero",
				Val: testParameters{
					crashCommand: "crash divzero",
				},
			},
			{
				Name: "stack",
				Val: testParameters{
					crashCommand: "crash stack",
				},
			},
			{
				Name: "unaligned",
				Val: testParameters{
					crashCommand: "crash unaligned",
				},
			},
			{
				Name: "watchdog",
				Val: testParameters{
					crashCommand: "crash watchdog",
				},
				ExtraHardwareDeps: hwdep.D(hwdep.ECBuildConfigOptions("PANIC_ON_WATCHDOG_WARNING", "PLATFORM_EC_PANIC_ON_WATCHDOG_WARNING")),
			},
			{
				Name: "coredump",
				Val: testParameters{
					crashCommand:   "crash divzero",
					expectCoredump: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.ECFeatureMemoryDumpCommands()),
			},
		},
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
	const systemCrashDir = "/var/spool/crash"
	var timerInfoLine string

	param := s.Param().(testParameters)

	d := s.DUT()

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	fs := crash_service.NewFixtureServiceClient(cl.Conn)

	req := crash_service.SetUpCrashTestRequest{
		Consent: crash_service.SetUpCrashTestRequest_MOCK_CONSENT,
	}

	// Shorten deadline to leave time for cleanup
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	if _, err := fs.SetUp(ctx, &req); err != nil {
		s.Error("Failed to set up: ", err)
		cl.Close(cleanupCtx)
		return
	}

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

	s.Log("Running crash command: ", param.crashCommand)
	// This should reboot the device
	if err := h.Servo.RunECCommand(ctx, param.crashCommand); err != nil {
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
	fs = crash_service.NewFixtureServiceClient(cl.Conn)

	const base = `embedded_controller\.\d{8}\.\d{6}\.\d+\.0`
	waitReq := &crash_service.WaitForCrashFilesRequest{
		Dirs:    []string{systemCrashDir},
		Regexes: []string{base + `\.eccrash`, base + `\.meta`},
	}

	const coredumpPattern = base + `\.coredump\.gz`
	if param.expectCoredump {
		waitReq.Regexes = append(waitReq.Regexes, coredumpPattern)
	}

	s.Log("Waiting for files to become present")
	crashFiles, err := fs.WaitForCrashFiles(ctx, waitReq)
	if err != nil {
		s.Fatal("Failed to find crash files: " + err.Error())
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
	if panicinfoFlags&panicDataFlagSafeModeFailPreconditions != 0 {
		s.Error("PANIC_DATA_FLAG_SAFE_MODE_FAIL_PRECONDITIONS is set in panic info flags")
	}
	if panicinfoFlags&panicDataFlagSafeModeStarted == 0 {
		s.Error("PANIC_DATA_FLAG_SAFE_MODE_STARTED is not set in panic info flags")
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

	if param.expectCoredump {
		coredump, err := linuxssh.ReadFile(cleanupCtx, d.Conn(), "/var/spool/cros_ec/coredump")
		if err != nil {
			s.Fatal("Failed to read coredump: ", err)
		}
		/* Verify coredump header ID */
		headerID := string(coredump[0:2])
		if headerID != "ZE" {
			s.Fatal("Failed to verify coredump header ID: ", headerID)
		}
		/* Verify coredump panicinfo */
		coredumpPanicinfo, err := linuxssh.ReadFile(cleanupCtx, d.Conn(), "/var/spool/cros_ec/panicinfo")
		if err != nil {
			s.Fatal("Failed to read coredump panicinfo: ", err)
		}
		if bytes.Equal(panicinfo, coredumpPanicinfo) {
			s.Fatal("Coredump panicinfo does not match expected panicinfo")
		}

		/* Decompress coredump gz file and verify it matches */
		var coredumpGzPath string
		for _, match := range crashFiles.Matches {
			if match.Regex == coredumpPattern {
				coredumpGzPath = match.Files[0]
				break
			}
		}
		if coredumpGzPath == "" {
			s.Fatal("Failed to find coredump gz")
		}
		coredumpGzCompressed, err := linuxssh.ReadFile(cleanupCtx, d.Conn(), coredumpGzPath)
		if err != nil {
			s.Fatal("Failed to read compressed coredump gz")
		}

		gz, err := gzip.NewReader(bytes.NewReader(coredumpGzCompressed))
		if err != nil {
			s.Fatal("Failed to initialize gzip reader")
		}
		defer gz.Close()

		coredumpGzDecompressed, err := ioutil.ReadAll(gz)
		if err != nil {
			s.Fatal("Failed to decompress coredump gz")
		}
		if !bytes.Equal(coredumpGzDecompressed, coredump) {
			s.Fatal("Decompressed coredump gz does not match coredump")
		}
	}
}
