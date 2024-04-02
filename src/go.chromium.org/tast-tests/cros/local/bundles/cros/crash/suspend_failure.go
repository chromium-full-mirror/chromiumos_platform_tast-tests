// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"io/ioutil"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/power/suspend"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SuspendFailure,
		Desc: "Verify suspend failures are logged as expected",
		Contacts: []string{
			"chromeos-data-eng@google.com",
			"iby@chromium.org",
		},
		BugComponent: "b:1032705",
		Attr:         []string{"group:mainline"},
	})
}

func SuspendFailure(ctx context.Context, s *testing.State) {
	const suspendFailureName = "suspend-failure"

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Set up the crash test, ignoring non-suspend-failure crashes.
	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent(), crash.FilterCrashes("suspend_failure")); err != nil {
		s.Fatal("SetUpCrashTest failed: ", err)
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	// Restart anomaly detector to clear its cache of recently seen service
	// failures and ensure this one is logged.
	if err := crash.RestartAnomalyDetectorWithSendAll(ctx, true); err != nil {
		s.Fatal("Failed to restart anomaly detector: ", err)
	}

	// Restart anomaly detector to clear its --testonly-send-all flag at the end of execution.
	defer crash.RestartAnomalyDetector(cleanupCtx)

	s.Log("Inducing artificial suspend permission failure")
	perm, err := testexec.CommandContext(ctx, "stat", "--format=%a", "/sys/power/state").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to run 'stat -c='%a' /sys/power/state': ", err)
	}

	// Remove all permissions on /sys/power/state to induce the failure on write
	err = testexec.CommandContext(ctx, "chmod", "000", "/sys/power/state").Run()
	if err != nil {
		s.Fatal("Failed to set permissions on /sys/power/state: ", err)
	}

	// Error is expected here, due to /sys/power/state being inaccessible.
	// Sets a 60 second wakeup just in case suspend somehow works here.
	// Sets a 30 second timeout to fail after allowing time for ~3 suspend
	// attempts.
	// If we were to allow powerd to fail to suspend 10 times (~100 seconds),
	// powerd would shut down the DUT. Since this is a local test, there is no
	// way to recover from a shutdown DUT, so we should not allow that to
	// happen.
	if _, err := suspend.Request(ctx, suspend.For(60*time.Second), suspend.Timeout(30*time.Second)); err == nil {
		s.Error("powerd_dbus_suspend didn't fail when we expected it to")
	}

	// Restart powerd since it's still trying to suspend (which we don't want to
	// happen right now).
	err = testexec.CommandContext(ctx, "restart", "powerd").Run()
	if err != nil {
		s.Error("Failed to restart powerd: ", err)
		// If we fail to restart powerd, we'll shut down in ~100 seconds, so
		// just reboot.
		testexec.CommandContext(ctx, "reboot").Run()
	}

	err = testexec.CommandContext(ctx, "chmod", strings.TrimRight(string(perm), "\r\n"), "/sys/power/state").Run()
	if err != nil {
		s.Errorf("Failed to reset permissions (%v) on /sys/power/state: %v", perm, err)
		// We're messed up enough that rebooting the machine to reset the file
		// permissions on /sys/power/state is best here.
		testexec.CommandContext(ctx, "reboot").Run()
	}

	const (
		logFileRegex  = `suspend_failure\.\d{8}\.\d{6}\.\d+\.0\.log`
		metaFileRegex = `suspend_failure\.\d{8}\.\d{6}\.\d+\.0\.meta`
	)
	expectedRegexes := []string{logFileRegex, metaFileRegex}

	crashDirs, err := crash.GetDaemonStoreCrashDirs(ctx)
	if err != nil {
		s.Fatal("Couldn't get daemon store dirs: ", err)
	}
	// We might not be logged in, so also allow system crash dir.
	crashDirs = append(crashDirs, crash.SystemCrashDir)

	files, err := crash.WaitForCrashFiles(ctx, crashDirs, expectedRegexes, crash.Timeout(30*time.Second))
	if err != nil {
		s.Fatal("Couldn't find expected files: ", err)
	}
	defer func() {
		if err := crash.RemoveAllFiles(cleanupCtx, files); err != nil {
			s.Log("Couldn't clean up files: ", err)
		}
	}()

	for _, meta := range files[metaFileRegex] {
		contents, err := ioutil.ReadFile(meta)
		if err != nil {
			s.Errorf("Couldn't read log file %s: %v", meta, err)
		}
		if !strings.Contains(string(contents), "upload_var_weight=50\n") {
			s.Error("Meta file didn't contain weight=50. Saving file")
			if err := crash.MoveFilesToOut(ctx, s.OutDir(), meta); err != nil {
				s.Error("Could not move meta file to out dir: ", err)
			}
		}
	}
}
