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
		Func:         MonitorUploadedCrashEvent,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Monitors uploaded crash events detected properly or not",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097",
		// TODO(b/266018436): Promote to critical.
		Attr:         []string{"group:criticalstaging", "group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
	})
}

func MonitorUploadedCrashEvent(ctx context.Context, s *testing.State) {
	// Trigger unuploaded crash event: Run the sleep command and crash it.
	sleepCmd := testexec.CommandContext(ctx, "sleep", "100")
	if err := sleepCmd.Start(); err != nil {
		s.Fatal("Failed to start the sleep command: ", err)
	}
	if err := sleepCmd.Signal(unix.SIGSEGV); err != nil {
		s.Fatal("Failed to crash the sleep command: ", err)
	}
	err := sleepCmd.Wait()
	waitStatus, ok := testexec.GetWaitStatus(err)
	if !ok {
		s.Fatal("Failed to get sleep's wait status: ", err)
	}
	if !waitStatus.Signaled() || waitStatus.Signal() != unix.SIGSEGV {
		s.Fatal("Failed to crash sleep: ", err)
	}

	// Consent for metrics.
	if err := testexec.CommandContext(ctx, "metrics_client", "-C").Run(); err != nil {
		s.Fatal("Failed to consent for metrics: ", err)
	}

	// Convert the unuploaded crash event to uploaded crash event.
	if err := testexec.CommandContext(ctx, "crash_sender", "--dev", "--max_spread_time=0").Run(); err != nil {
		s.Fatal("Failed to upload the crash: ", err)
	}

	// Run monitor command in background.
	monitorCmd := testexec.CommandContext(ctx, "cros-health-tool", "event", "--category=crash",
		"--length_seconds=10")
	stdout, stderr, err := monitorCmd.SeparatedOutput()
	if err != nil {
		s.Fatal("Failed to run healthd monitor command: ", err)
	}

	if len(stderr) > 0 {
		s.Fatal("Failed to detect uploaded crash event, stderr: ", string(stderr), ", stdout: ", string(stdout))
	}

	// The pattern will not match unuploaded crashes due to the presence of
	// the "uploaded_info" field.
	crashOutputPattern := regexp.MustCompile(`\{"capture_time":\d+,"crash_type":\d+,"local_id":"\S+","upload_info":{"crash_report_id":"\S+","creation_time":[\d\.]+,"offset":\d+}\}`)
	if !crashOutputPattern.MatchString(string(stdout)) {
		s.Fatal("Failed to detect uploaded event, event output: ", string(stdout))
	}
}
