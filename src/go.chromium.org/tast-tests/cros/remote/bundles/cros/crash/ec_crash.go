// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"os"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	crash_service "go.chromium.org/tast-tests/cros/services/cros/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECCrash,
		Desc: "Verify artificial EC crash creates crash files",
		Contacts: []string{
			"chromeos-faft@google.com",
			"chromeos-data-eng@google.com",
			"robbarnes@google.com",
			"troywang@google.com",
		},
		BugComponent: "b:167114",
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw"},
		Timeout:      10 * time.Minute,
		Fixture:      fixture.NormalMode,
		ServiceDeps:  []string{"tast.cros.crash.FixtureService"},
		// no_qemu because the servo is not available in VMs, and tast does
		// not (yet) support skipping tests if required vars are not provided.
		// TODO(crbug.com/967901): Remove no_qemu dep once servo var is sufficient.
		SoftwareDeps: []string{"device_crash", "ec_crash", "pstore", "reboot", "no_qemu"},
		Params: []testing.Param{
			{
				Name:              "assert",
				Val:               "crash assert",
				ExtraHardwareDeps: hwdep.D(hwdep.ECFeatureAssertsPanic()),
			},
			{
				Name: "divzero",
				Val:  "crash divzero",
			},
			{
				Name: "unaligned",
				Val:  "crash unaligned",
			},
			{
				Name: "watchdog",
				Val:  "crash watchdog",
			},
		},
	})
}

func isPanicLogEnabled(ctx context.Context, s *testing.State) (bool, error) {
	if s.Features("").GetHardware().GetHardwareFeatures().GetEmbeddedController().GetBuildConfig() == nil {
		// BuildConfig is missing, fallback to using ectool to for Panic Log support
		err := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain).Command(ctx, "paniclog", "info").Run()
		if err != nil {
			return false, nil
		}
		return true, nil
	}
	satisfied, _, err := hwdep.ECBuildConfigOptions("PANIC_LOG", "PLATFORM_EC_PANIC_LOG").Satisfied(s.Features("").GetHardware())
	if err != nil {
		return false, errors.Wrap(err, "failed to check PANIC_LOG")
	}
	if !satisfied {
		return false, nil
	}

	return true, nil
}

// ECCrash verifies that crash files are generated when the EC crashes.
func ECCrash(ctx context.Context, s *testing.State) {
	const systemCrashDir = "/var/spool/crash"
	const base = `embedded_controller\.\d{8}\.\d{6}\.\d+\.0`
	suffixes := []string{"eccrash", "meta", "log"}

	d := s.DUT()

	h := s.FixtValue().(*fixture.Value).Helper

	// Skip checking for EC crashes in this test because it crashes intentionally.
	h.CheckECCrash = false

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
		if fs != nil {
			if _, err := fs.TearDown(cleanupCtx, &empty.Empty{}); err != nil {
				s.Error("Couldn't tear down: ", err)
			}
		}
		if cl != nil {
			cl.Close(cleanupCtx)
		}
	}()

	panicLogEnabled, err := isPanicLogEnabled(ctx, s)
	if err != nil {
		s.Fatal("Failed to check if panic log is enabled: ", err)
	}
	if panicLogEnabled {
		s.Log("Panic log is enabled")
		suffixes = append(suffixes, ".panic.log")
	} else {
		s.Log("Panic log is disabled")
	}

	if out, err := d.Conn().CommandContext(ctx, "logger", "Running ECCrash").CombinedOutput(); err != nil {
		s.Logf("WARNING: Failed to log info message: %s", out)
	}

	// Sync filesystem to minimize impact of the panic on other tests
	if out, err := d.Conn().CommandContext(ctx, "sync").CombinedOutput(); err != nil {
		s.Fatalf("Failed to sync filesystems: %s", out)
	}

	// When we crash, these connections will break.
	cl.Close(ctx)
	cl = nil
	fs = nil

	// Rebooting the EC can make servod fail if the CCD watchdog is not removed.
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove CCD watchdog: ", err)
	}

	s.Log("Running crash command: ", s.Param().(string))
	// This should reboot the device
	if err := h.Servo.RunECCommand(ctx, s.Param().(string)); err != nil {
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

	waitReq := &crash_service.WaitForCrashFilesRequest{
		Dirs:    []string{systemCrashDir},
		Regexes: []string{base + `\.(` + strings.Join(suffixes, "|") + `)`},
	}
	s.Log("Waiting for crash files to become present")
	res, err := fs.WaitForCrashFiles(ctx, waitReq)
	if err != nil {
		if err := linuxssh.GetFile(cleanupCtx, d.Conn(), "/var/log/messages", filepath.Join(s.OutDir(), "messages"), linuxssh.PreserveSymlinks); err != nil {
			s.Log("Failed to save messages log")
		}
		s.Fatal("Failed to find crash files: " + err.Error())
	}
	if err := h.UpdateECCrashCache(ctx); err != nil {
		s.Fatal("Failed to update EC crash file cache after test: ", err)
	}

	for _, match := range res.Matches {
		b, err := linuxssh.ReadFile(ctx, d.Conn(), match.Files[0])
		if err != nil {
			s.Error("Failed to read crash file: ", match.Files[0])
			continue
		}

		// Verify crash files are not empty
		if len(b) == 0 {
			s.Errorf("Crash file %s is empty", match.Files[0])
		}

		// Verify that parsed EC crash does not contain WARNING/ERROR
		failureRegexp := regexp.MustCompile(`^(ERROR|WARNING):.*$`)
		if strings.HasSuffix(match.Regex, ".eccrash") {
			hasError := false
			lines := strings.Split(string(b), "\n")
			for _, line := range lines {
				if err := failureRegexp.FindString(line); err != "" {
					hasError = true
					s.Error("EC crash contains ", string(err))
				}
			}

			if hasError {
				localFile := filepath.Join(s.OutDir(), path.Base(match.Files[0]))
				if err := os.WriteFile(localFile, b, 0644); err != nil {
					s.Log("Error writing local copy of the crash: ", err)
				}
			}
		}
	}

	removeReq := &crash_service.RemoveAllFilesRequest{
		Matches: res.Matches,
	}
	if _, err := fs.RemoveAllFiles(ctx, removeReq); err != nil {
		s.Error("Error removing files: ", err)
	}
}
