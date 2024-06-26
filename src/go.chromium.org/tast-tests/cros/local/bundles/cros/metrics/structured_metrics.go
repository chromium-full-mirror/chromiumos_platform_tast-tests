// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package metrics

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/metrics"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         StructuredMetrics,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that processes can log Structured Metrics events",
		Contacts: []string{
			"chromeos-data-eng@google.com",
		},
		BugComponent: "b:1087262", // ChromeOS > Data > Engineering > Metrics
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
	})
}

// Name of the test event to be used.
const testStructuredMetricName = "cros::TestProjectOne::TestEventTwo"

// Binary paths to be executed.
const (
	minijailPath      = "/sbin/minijail0"
	metricsClientPath = "/usr/bin/metrics_client"
)

// Minijail configurations to test Structured metrics with.
const (
	flossPolicy                  = "/usr/share/policy/floss-seccomp.policy"
	rollbackCleanupConfiguration = "/usr/share/minijail/rollback_cleanup.conf"
	usbBouncerPolicy             = "/usr/share/policy/usb_bouncer-seccomp.policy"
)

// StructuredMetrics runs the metrics_client CLI to record a Structured Metric using various different policies and configurations to be used in other processes in minijail.
func StructuredMetrics(ctx context.Context, s *testing.State) {
	if err := enableMetrics(ctx, s); err != nil {
		s.Error("Failed to enable metrics: ", err)
		return
	}

	for _, tc := range []struct {
		name     string // test case name
		path     string // file path to configuration or policy
		isConfig bool   // whether path is a minijail config
	}{
		// Test cases.
		{"floss", flossPolicy, false},
		{"rollbackCleanup", rollbackCleanupConfiguration, true},
		{"usbBouncer", usbBouncerPolicy, false},
	} {
		if ctx.Err() != nil {
			s.Error("Aborting testing: ", ctx.Err())
			break
		}
		if err := metrics.CleanStructuredEvents(ctx); err != nil {
			s.Error("Failed to clean structured events directory")
			break
		}

		recordTestStructuredMetric(ctx, s, tc.path, tc.isConfig)
		result, err := metrics.HasStructuredEventBeenReported(ctx, testStructuredMetricName)
		if err != nil {
			s.Error("Failure when checking that test event has been recorded for case ", tc.name)
		}
		if !result {
			s.Error("Failed to record test event for case ", tc.name)
		}

		// Cleanup structured events directory for the next case.
		metrics.CleanStructuredEvents(ctx)
	}

	// Disable metrics after the test since metrics should not normally be enabled for tests.
	if err := disableMetrics(ctx, s); err != nil {
		s.Error("Failed to disable metrics: ", err)
	}
}

// enableMetrics creates a consent file.
func enableMetrics(ctx context.Context, s *testing.State) error {
	var args []string
	args = append(args, "-C")

	cmd := testexec.CommandContext(ctx, metricsClientPath, args...)
	cmdStr := shutil.EscapeSlice(cmd.Args)
	s.Log("Enabling metrics with ", cmdStr)
	err := cmd.Run()

	return err
}

// disableMetrics deletes the consent file.
func disableMetrics(ctx context.Context, s *testing.State) error {
	var args []string
	args = append(args, "-D")

	cmd := testexec.CommandContext(ctx, metricsClientPath, args...)
	cmdStr := shutil.EscapeSlice(cmd.Args)
	s.Log("Disabling metrics with ", cmdStr)
	err := cmd.Run()

	return err
}

// recordTestStructuredMetric runs the metrics_client CLI to record a test StructuredMetric.
// The tests are run in minijail with a policy specified at configPath to ensure that the process has proper permissions to record StructuredMetrics.
func recordTestStructuredMetric(ctx context.Context, s *testing.State, path string, isConfig bool) {
	var args []string
	// Command to execute in minijail.
	var metricsClientCommand = []string{
		"--",
		metricsClientPath,
		"--structured",
		"TestProjectOne",
		"TestEventTwo",
		"--TestMetricThree=5",
	}

	if isConfig {
		args = append(args, "--config", path)
	} else { // A seccomp policy
		args = append(args, "-S", path)
	}
	args = append(args, metricsClientCommand...)

	cmd := testexec.CommandContext(ctx, minijailPath, args...)
	cmdStr := shutil.EscapeSlice(cmd.Args)
	s.Log("Running ", cmdStr)
	err := cmd.Run()

	if st, ok := testexec.GetWaitStatus(err); !ok {
		s.Errorf("(%v) failed (no exit status): %v", cmdStr, err)
	} else if st.ExitStatus() != 0 {
		s.Errorf("(%v) failed with exit code %d: %v", cmdStr, st.ExitStatus(), err)
	}
}
