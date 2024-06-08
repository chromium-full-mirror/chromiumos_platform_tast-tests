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
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelCrash,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify artificial kernel crash creates crash files",
		Contacts:     []string{"chromeos-data-eng@google.com", "swboyd@chromium.org", "dianders@chromium.org"},
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
				goodSig:    kernelPanicSig,
				execName:   "kernel",
				earlyCrash: false,
			},
		}, {
			Name: "mock_consent",
			Val: testParams{
				consent:    crash_service.SetUpCrashTestRequest_MOCK_CONSENT,
				panicCmd:   kernelPanicCmd,
				goodSig:    kernelPanicSig,
				execName:   "kernel",
				earlyCrash: false,
			},
		}, {
			Name: "hung_task",
			Val: testParams{
				consent:    crash_service.SetUpCrashTestRequest_MOCK_CONSENT,
				panicCmd:   kernelHungTaskCmd,
				goodSig:    kernelHungTaskSig,
				execName:   "kernel",
				earlyCrash: false,
			},
		}, {
			Name: "soft_lockup",
			Val: testParams{
				consent:    crash_service.SetUpCrashTestRequest_MOCK_CONSENT,
				panicCmd:   kernelSoftLockupCmd,
				goodSig:    kernelSoftLockupSig,
				goodKcrash: kernelSoftLockupKcrash,
				execName:   "kernel",
				earlyCrash: false,
			},
		}, {
			Name:              "hard_lockup",
			ExtraSoftwareDeps: []string{"nmi_backtrace"},
			ExtraHardwareDeps: hwdep.D(hwdep.NmiSupport()),
			Val: testParams{
				consent:    crash_service.SetUpCrashTestRequest_MOCK_CONSENT,
				panicCmd:   kernelHardLockupCmd,
				goodSig:    kernelHardLockupSig,
				goodKcrash: kernelHardLockupKcrash,
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
	goodSig    string
	goodKcrash string // OK if this is blank since that matches everything
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

const kernelPanicCmd = "echo PANIC > /sys/kernel/debug/provoke-crash/DIRECT"
const kernelPanicSig = "kernel-dumptest-[[:xdigit:]]{8}"

// Shorten the hung task timeout both to make the test run faster and to avoid
// hitting a tast timeout. NOTE: it can take up to 2x the timeout for the
// reboot to happen since we have `kernel.hung_task_check_interval_secs = 0`
// and that means we check for hung tasks very infrequently.
//
// NOTE: we only shorten the timeout if it's the expected value of 120 to avoid
// accidentally masking a bug elsewhere in the system that causes
// the hung_task_timeout_secs to fail to set set properly.
//
// ALSO NOTE: we don't want to make this timeout _too_ short since it could
// potentially trigger a false hung task that's not the one we want. There are
// some legitimate reasons why we might block in the kernel for a long time
// (like if we call `sync` to wait for everything to be written to disk).
// Potentially we could go lower than 30 seconds here, but if we went down to
// 1 second that would likely be extreme.
const kernelHungTaskCmd = `
if [ "$(sysctl -b kernel.hung_task_timeout_secs)" = "120" ]; then
  sysctl -w kernel.hung_task_timeout_secs=30;
fi
echo HUNG_TASK > /sys/kernel/debug/provoke-crash/DIRECT
`
const kernelHungTaskSig = "kernel-\\(HANG\\)-lkdtm_HUNG_TASK-[[:xdigit:]]{8}"

// Although we expect lkdtm_SOFTLOCKUP to be in the signature, we don't require
// it. We just require it to be in the kcrash file somewhere. The problem is
// that sometimes softirq functions can be on the stack atop lkdtm_SOFTLOCKUP
// and thus the "top" function is not lkdtm_SOFTLOCKUP. It would be nice if
// we could fix the kernel not to do that because it makes our crash signatures
// not as good (the softirq actually didn't have anything to do with the
// softlockup in this case), but for now we'll accept it.
const kernelSoftLockupCmd = "echo SOFTLOCKUP > /sys/kernel/debug/provoke-crash/DIRECT"
const kernelSoftLockupSig = "kernel-\\(SOFTLOCKUP\\)-.*-[[:xdigit:]]{8}"
const kernelSoftLockupKcrash = "lkdtm_SOFTLOCKUP"

// Although we expect lkdtm_HARDLOCKUP to be in the signature, we don't require
// it. We just require it to be in the kcrash file somewhere. The problem is
// that when we hardlockup one CPU it has a chance of causing a hardlockup on
// another CPU too. The lockup detector could detect either of the two first.
// Since we have options to trace all CPUs we'll still see "lkdtm_HARDLOCKUP"
// traced but it just might not be in the signature.
const kernelHardLockupCmd = "echo HARDLOCKUP > /sys/kernel/debug/provoke-crash/DIRECT"
const kernelHardLockupSig = "kernel-\\(HARDLOCKUP\\)-.*-[[:xdigit:]]{8}"
const kernelHardLockupKcrash = "lkdtm_HARDLOCKUP"

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
	sigRegexp := regexp.MustCompile("sig=(.*)")
	badSigRegexp := regexp.MustCompile("kernel-.+-00000000")
	goodSigRegexp := regexp.MustCompile("kernel-.+-[[:xdigit:]]{8}")
	if crash.goodSig != "" {
		goodSigRegexp = regexp.MustCompile(crash.goodSig)
	}
	goodKcrashRegexp := regexp.MustCompile(crash.goodKcrash)
	savedVersionRegexp := regexp.MustCompile(`ver=99999\.`)
	savedLsbRegexp := regexp.MustCompile(`upload_var_lsb-release=99999\.`)
	for _, match := range res.Matches {
		if !strings.HasSuffix(match.Regex, ".meta") {
			continue
		}
		if err := d.GetFile(ctx, match.Files[0],
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
		s.Log("Checking signature")
		sigMatchResult := sigRegexp.FindSubmatch(f)
		if len(sigMatchResult) == 0 {
			s.Error("Missing signature in meta file ", match.Files[0])
		} else {
			sig := sigMatchResult[1]
			if badSigRegexp.Match(sig) {
				s.Errorf("Found all zero signature (%s) in meta file %s", sig, match.Files[0])
			} else if !goodSigRegexp.Match(f) {
				s.Errorf("Signature mismatch (%s vs %s) in meta file %s", sig, goodSigRegexp, match.Files[0])
			}
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

	for _, match := range res.Matches {
		if !strings.HasSuffix(match.Regex, ".kcrash") {
			continue
		}
		if err := d.GetFile(ctx, match.Files[0],
			filepath.Join(s.OutDir(), path.Base(match.Files[0]))); err != nil {
			s.Error("Failed to save kcrash file")
			continue
		}
		f, err := ioutil.ReadFile(filepath.Join(s.OutDir(), path.Base(match.Files[0])))
		if err != nil {
			s.Error("Failed to read kcrash file", match.Files[0])
			continue
		}
		s.Log("Checking kcrash")
		if !goodKcrashRegexp.Match(f) {
			s.Errorf("kcrash didn't match %s", goodKcrashRegexp)
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
