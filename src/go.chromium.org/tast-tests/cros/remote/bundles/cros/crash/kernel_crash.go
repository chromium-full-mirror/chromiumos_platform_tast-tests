// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"io/ioutil"
	"path"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	crash_service "go.chromium.org/tast-tests/cros/services/cros/crash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelCrash,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify artificial kernel crash creates crash files",
		Contacts:     []string{"cros-telemetry@google.com", "mutexlox@chromium.org"},
		BugComponent: "b:1032705",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"device_crash", "pstore", "reboot"},
		ServiceDeps:  []string{"tast.cros.crash.FixtureService"},
		Params: []testing.Param{{
			Name:              "real_consent",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"chrome", "metrics_consent"},
			Val: testParams{
				consent:    crash_service.SetUpCrashTestRequest_REAL_CONSENT,
				panicCmd:   kernelPanicCmd,
				execName:   "kernel",
				earlyCrash: false,
			},
		}, {
			Name: "mock_consent",
			Val: testParams{
				consent:    crash_service.SetUpCrashTestRequest_MOCK_CONSENT,
				panicCmd:   kernelPanicCmd,
				execName:   "kernel",
				earlyCrash: false,
			},
		}, {
			Name:      "early_crash",
			ExtraAttr: []string{"informational"},
			Val: testParams{
				consent:    crash_service.SetUpCrashTestRequest_MOCK_CONSENT,
				panicCmd:   "", // We reboot to cause a panic for the early panic.
				execName:   "kernel",
				earlyCrash: true,
			},
		}},
		Timeout: 10 * time.Minute,
	})
}

type testParams struct {
	consent    crash_service.SetUpCrashTestRequest_ConsentType
	panicCmd   string
	execName   string
	earlyCrash bool
}

const (
	lsbPath      = "/etc/lsb-release"
	lsbSavedPath = "/var/lib/crash_reporter/lsb-release"
)

// messUpLsbRelease overwrites 5-digit version numbers in the saved lsb-release with invalid version values (99999),
// so that we can determine which lsb-release crash-reporter used to generate the .meta file.
func messUpLsbRelease(ctx context.Context, d *dut.DUT) error {
	// Find any string of 5 digits after an equals sign, and replace
	// them with "99999" to create a saved lsb-release with different
	// version values.
	const regex = `s/^(.*)=[0-9]{5}(\b.*)$/\1=99999\2/`
	if out, err := d.Conn().CommandContext(ctx, "/bin/sed", "-i", "-E", regex, lsbSavedPath).CombinedOutput(); err != nil {
		testing.ContextLogf(ctx, "Failed to edit lsb-release: %s", out)
		return errors.Wrap(err, "failed to edit lsb-release")
	}
	return nil
}

// restoreLsbRelease restores the saved lsb-release with the copy from /etc.
func restoreLsbRelease(ctx context.Context, d *dut.DUT) error {
	if out, err := d.Conn().CommandContext(ctx, "/bin/cp", lsbPath, lsbSavedPath).CombinedOutput(); err != nil {
		testing.ContextLogf(ctx, "Failed to rstore lsb-release: %s", out)
		return errors.Wrap(err, "failed to restore lsb-release")
	}
	return nil
}

const kernelPanicCmd = `
  if [ -f /sys/kernel/debug/provoke-crash/DIRECT ]; then
    echo PANIC > /sys/kernel/debug/provoke-crash/DIRECT
  else
    echo panic > /proc/breakme
  fi`

func KernelCrash(ctx context.Context, s *testing.State) {
	const systemCrashDir = "/var/spool/crash"

	d := s.DUT()

	cl, err := rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	fs := crash_service.NewFixtureServiceClient(cl.Conn)
	crash := s.Param().(testParams)

	req := crash_service.SetUpCrashTestRequest{
		Consent: crash.consent,
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
	// Otherwise, we need to run TearDown on the re-established connection to the machine.
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

	if out, err := d.Conn().CommandContext(ctx, "logger", "Running", s.TestName()).CombinedOutput(); err != nil {
		s.Log("Invoking 'logger' failed: ", err)
		s.Logf("WARNING: Failed to log info message: %s", out)
	}

	if err := messUpLsbRelease(ctx, d); err != nil {
		s.Error("Couldn't set up lsb-release: ", err)
	}
	defer func() {
		// Crash reporter *should* reset the lsb-release copy automatically when it runs the boot collector.
		// However, in case it does not, manually copy the file.
		if err := restoreLsbRelease(cleanupCtx, d); err != nil {
			s.Error("Couldn't restore lsb-release: ", err)
		}
	}()

	if crash.earlyCrash {
		// Create a file indicating that we should crash early in boot, before the boot collector runs.
		if out, err := d.Conn().CommandContext(ctx, "/usr/bin/touch", "/mnt/stateful_partition/unencrypted/preserve/crash-kernel-early").CombinedOutput(); err != nil {
			s.Fatalf("Couldn't create crash-kernel-early: %v. %s", err, out)
		}

		// Shortly after the reboot, the device should panic.
		if err := d.Reboot(ctx); err != nil {
			s.Fatal("Couldn't reboot dut: ", err)
		}
	} else {

		// Sync filesystem to minimize impact of the panic on other tests
		if out, err := d.Conn().CommandContext(ctx, "sync").CombinedOutput(); err != nil {
			s.Log("Invoking 'sync' failed: ", err)
			s.Fatalf("Failed to sync filesystems: %s", out)
		}

		// Trigger a panic. By the time RebootWithCommand() returns the DUT will be reconnected.
		if err := d.RebootWithCommand(ctx, "sh", "-c", crash.panicCmd); err != nil {
			s.Fatal("Failed to panic DUT: ", err)
		}
	}

	// When we lost the connection, these connections broke.
	cl.Close(ctx)
	cl = nil
	fs = nil

	cl, err = rpc.Dial(ctx, d, s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	fs = crash_service.NewFixtureServiceClient(cl.Conn)

	const base = `kernel\.\d{8}\.\d{6}\.\d+\.0`
	waitReq := &crash_service.WaitForCrashFilesRequest{
		Dirs:    []string{systemCrashDir},
		Regexes: []string{base + `\.kcrash`, base + `\.meta`, base + `\.log`},
	}
	s.Log("Waiting for files to become present")
	res, err := fs.WaitForCrashFiles(ctx, waitReq)
	if err != nil {
		if err := d.GetFile(cleanupCtx, "/var/log/messages",
			filepath.Join(s.OutDir(), "messages")); err != nil {
			s.Log("Failed to save messages log")
		}
		s.Fatal("Failed to find crash files: " + err.Error())
	}

	execNameRegexp := regexp.MustCompile("(?m)^exec_name=" + crash.execName + "$")
	badSigRegexp := regexp.MustCompile("sig=kernel-.+-00000000")
	goodSigRegexp := regexp.MustCompile("sig=kernel-.+-[[:xdigit:]]{8}")
	savedVersionRegexp := regexp.MustCompile(`ver=99999\.`)
	savedLsbRegexp := regexp.MustCompile(`upload_var_lsb-release=99999\.`)
	for _, match := range res.Matches {
		if !strings.HasSuffix(match.Regex, ".meta") {
			continue
		}
		s.Log("Checking signature line for non-zero")
		if err := d.GetFile(cleanupCtx, match.Files[0],
			filepath.Join(s.OutDir(), path.Base(match.Files[0]))); err != nil {
			s.Error("Failed to save meta file")
			continue
		}
		f, err := ioutil.ReadFile(filepath.Join(s.OutDir(), path.Base(match.Files[0])))
		if err != nil {
			s.Error("Failed to read meta file", match.Files[0])
			continue
		}
		s.Log("Checking exec_name")
		if !execNameRegexp.Match(f) {
			s.Error("Found wrong exec_name in meta file ", match.Files[0])
		}
		if badSigRegexp.Match(f) {
			s.Error("Found all zero signature in meta file ", match.Files[0])
		} else if !goodSigRegexp.Match(f) {
			s.Error("Couldn't find unique signature in meta file ", match.Files[0])
		}

		if crash.earlyCrash {
			// Should not have used saved lsb, but /etc/
			if savedVersionRegexp.Match(f) {
				s.Error("Found wrong version in meta file ", match.Files[0])
			}
			if savedLsbRegexp.Match(f) {
				s.Error("Found wrong lsb-release in meta file ", match.Files[0])
			}
		} else {
			// Should have used saved lsb, and not /etc/
			if !savedVersionRegexp.Match(f) {
				s.Error("Found wrong version in meta file ", match.Files[0])
			}
			if !savedLsbRegexp.Match(f) {
				s.Error("Found wrong lsb-release in meta file ", match.Files[0])
			}
		}
	}

	// Also remove the bios log if it was created.
	biosLogMatches := &crash_service.RegexMatch{
		Regex: base + `\.bios_log`,
		Files: nil,
	}
	for _, f := range res.Matches[0].Files {
		biosLogMatches.Files = append(biosLogMatches.Files, strings.TrimSuffix(f, filepath.Ext(f))+".bios_log")
	}
	removeReq := &crash_service.RemoveAllFilesRequest{
		Matches: append(res.Matches, biosLogMatches),
	}

	if _, err := fs.RemoveAllFiles(ctx, removeReq); err != nil {
		s.Error("Error removing files: ", err)
	}
}
