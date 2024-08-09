// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"bytes"
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bluetooth/floss"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MonitorBluetoothEventFloss,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Monitors whether Bluetooth events are detected properly when the system is using Floss",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"byronlee@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"diagnostics"},
		Fixture:      "crosHealthdRunningAndBluetoothEnabledWithFloss",
	})
}

func MonitorBluetoothEventFloss(ctx context.Context, s *testing.State) {
	managerClient, err := floss.DefaultManagerClient(ctx)
	if err != nil {
		s.Fatal("Failed to access Bluetooth manager, err: ", err)
	}

	defaultHci := managerClient.DefaultAdapterHCI()
	// Set the power off first.
	if err := managerClient.Stop(ctx, defaultHci); err != nil {
		s.Fatal("Failed to power off bluetooth, err: ", err)
	}

	// Run monitor command in background.
	var stdoutBuf, stderrBuf bytes.Buffer
	monitorCmd := testexec.CommandContext(ctx, "cros-health-tool", "event", "--category=bluetooth", "--length_seconds=10")
	monitorCmd.Stdout = &stdoutBuf
	monitorCmd.Stderr = &stderrBuf

	if err := monitorCmd.Start(); err != nil {
		s.Fatal("Failed to run healthd monitor command: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		stdout := string(stdoutBuf.Bytes())
		if !strings.Contains(stdout, "Subscribe to bluetooth events successfully") {
			return errors.Errorf("failed to subscirbe Bluetooth event, stdout: %s", stdout)
		}
		return nil
	}, &testing.PollOptions{Interval: 200 * time.Millisecond, Timeout: 1 * time.Second}); err != nil {
		s.Fatal("Failed to subscirbe event in healthd: ", err)
	}

	// Trigger Bluetooth event by setting the power on.
	if err := managerClient.Start(ctx, defaultHci); err != nil {
		if killErr := monitorCmd.Kill(); killErr != nil {
			s.Log(ctx, "Error killing healthd monitor command: ", killErr)
		}
		if waitErr := monitorCmd.Wait(); waitErr != nil {
			s.Log(ctx, "Error waiting healthd monitor command: ", waitErr)
		}
		s.Fatal("Failed to trigger Bluetooth power on event: ", err)
	}

	if err := monitorCmd.Wait(); err != nil {
		s.Fatal("Failed to wait healthd monitor command: ", err)
	}

	if stderr := string(stderrBuf.Bytes()); stderr != "" {
		s.Fatal("Failed to detect Bluetooth on event, stderr: ", stderr)
	}

	if stdout := string(stdoutBuf.Bytes()); !strings.Contains(stdout, "Bluetooth event received") {
		s.Fatal("Failed to detect Bluetooth on event, event output: ", stdout)
	}
}
