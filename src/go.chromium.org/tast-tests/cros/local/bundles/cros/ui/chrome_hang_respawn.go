// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/ash/ashproc"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast-tests/cros/local/syslog"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ChromeHangRespawn,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Hangs the browser and verifies that session_manager detects the hang and restarts it",
		BugComponent: "b:1331478", // ChromeOS > Software > Core > SessionManager
		Contacts: []string{
			"chromeos-devices-team@google.com",
			"korneld@chromium.org",
		},
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		// On my redrix this test usually takes around ~15s to complete.
		// The total timeout below is 3m, just to be safe.
		Timeout: 2*upstart.UIRestartTimeout + time.Minute,
	})
}

const (
	// hangDetectionFlagFile is checked by session_manager when it starts,
	// and if it exists:
	// 1. The browser will always be restarted when a hang is detected.
	// 2. The hang detection interval is lowered from 1 minute to 10 seconds.
	hangDetectionFlagFile = "/run/session_manager/enable_hang_detection"
)

// ChromeHangRespawn first prepares the environment by restarting session_manager,
// and calling SetUpCrashTest. Then it stops the browser by sending SIGSTOP,
// waits for session_manager to detect the hang, resumes it with SIGCONT,
// and finally checks if it was successfully restarted.
func ChromeHangRespawn(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	timeForCleanUp := 5*time.Second + upstart.UIRestartTimeout
	ctx, cancel := ctxutil.Shorten(ctx, timeForCleanUp)
	defer cancel()

	// Depending on image type session_manager might not restart chrome
	// when it's detects a hang. To get a consistent behavior create
	// a file that is used to tell it to override that setting for testing.
	f, err := os.Create(hangDetectionFlagFile)
	if err != nil {
		s.Fatal("Failed to create hang detection flag file: ", err)
	}
	f.Close()
	defer func(cleanupCtx context.Context) {
		if err := os.Remove(hangDetectionFlagFile); err != nil {
			s.Error("Failed to remove hang detection flag file: ", err)
		}
		if err := upstart.RestartJob(cleanupCtx, "ui"); err != nil {
			s.Error("Failed to restart ui job: ", err)
		}
	}(cleanupCtx)

	// Now restart session_manager so that it picks up the change.
	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to restart ui job: ", err)
	}

	// Set up crash test environment to prevent leaking browser coredumps.
	// SetUpCrashTest assumes that chrome is up, which might not be the
	// case as we've just restarted session_manager, so wait for that.
	if _, err := ashproc.WaitForRoot(ctx, time.Minute); err != nil {
		s.Fatal("Browser not up after ui job reset: ", err)
	}
	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent()); err != nil {
		s.Fatal("Failed to setup crash context: ", err)
	}
	defer crash.TearDownCrashTest(cleanupCtx)

	// SetUpCrashTest might have restarted chrome.
	// Make sure it's up and get the process handle.
	proc, err := ashproc.WaitForRoot(ctx, time.Minute)
	if err != nil {
		s.Fatal("Browser not up after SetUpCrashTest: ", err)
	}
	reader, err := syslog.NewReader(ctx)
	if err != nil {
		s.Fatal("Failed to start SessionManager reader: ", err)
	}
	defer reader.Close()

	// Send SIGSTOP to the browser.
	if err := proc.SuspendWithContext(ctx); err != nil {
		s.Fatal("Failed to suspend browser process: ", err)
	}

	// With the test flag file set the watchdog interval is lowered to 5s,
	// meaning that session_manager sends a ping every 5s, with a 5s timeout.
	// This gives us an upper bound for expected hang detection time.
	// Double that for our local timeout.
	const timeout = 2 * (10 * time.Second)
	predicate := func(e *syslog.Entry) bool {
		return e.Severity == string(syslog.Warning) &&
			e.Program == string(syslog.SessionManager) &&
			strings.Contains(e.Content, "Browser hang detected!")
	}
	s.Log("Waiting for SessionManager to detect browser hang")
	_, err = reader.Wait(ctx, timeout, predicate)
	if err != nil {
		s.Fatal("Timeout while waiting for SessionManager hang detector: ", err)
	}

	// Resume the browser by sending SIGCONT and wait for its process
	// to be terminated. Note that SIGABORT, used by session_manager will
	// be blocked until SIGCONT is received. Because of that we don't expect
	// to fail here.
	if err := proc.ResumeWithContext(ctx); err != nil {
		s.Fatal("Failed to resume browser process: ", err)
	}
	if err := procutil.WaitForTerminated(ctx, proc, time.Minute); err != nil {
		s.Fatal("Browser process not terminated in time: ", err)
	}

	// Finally make sure that the browser is up after being restarted.
	if _, err := ashproc.WaitForRoot(ctx, time.Minute); err != nil {
		s.Fatal("Browser not up after hang restart: ", err)
	}
}
