// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"regexp"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MonitorUnuploadedCrashEvent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Monitors unuploaded crash events detected properly or not",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097",
		// TODO(b/266018436): Promote to critical.
		Attr:         []string{"group:criticalstaging", "group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
	})
}

func MonitorUnuploadedCrashEvent(ctx context.Context, s *testing.State) {
	// Trigger unuploaded crash event: Run the sleep command and crash it.
	sleepCmd := testexec.CommandContext(ctx, "sleep", "100")
	if err := sleepCmd.Start(); err != nil {
		s.Fatal("Failed to start the sleep command: ", err)
	}
	if err := sleepCmd.Signal(unix.SIGSEGV); err != nil {
		s.Fatal("Failed to crash the sleep command: ", err)
	}
	sleepCmd.Wait()

	// Consent for metrics.
	if err := testexec.CommandContext(ctx, "metrics_client", "-C").Run(); err != nil {
		s.Fatal("Failed to consent for metrics: ", err)
	}

	// Run monitor command in background.
	monitorCmd := testexec.CommandContext(ctx, "cros-health-tool", "event", "--category=crash",
		// Wait for 40 seconds because crash_sender would wait for 30
		// seconds after a crash meta file is written. See the comment
		// above hold_off_time in
		// platform2/crash-reporter/crash_sender_base.h.
		"--length_seconds=40")
	stdout, stderr, err := monitorCmd.SeparatedOutput()
	if err != nil {
		s.Fatal("Failed to run healthd monitor command: ", err)
	}

	if len(stderr) > 0 {
		s.Fatal("Failed to detect unuploaded crash event, stderr: ", string(stderr), ", stdout: ", string(stdout))
	}

	// The pattern will not match uploaded crashes due to the lack of the
	// "uploaded_info" field.
	crashOutputPattern := regexp.MustCompile(`\{"capture_time":\d+,"crash_type":\d+,"local_id":"\S+"\}`)
	if !crashOutputPattern.MatchString(string(stdout)) {
		s.Fatal("Failed to detect unuploaded event, event output: ", string(stdout))
	}
}
