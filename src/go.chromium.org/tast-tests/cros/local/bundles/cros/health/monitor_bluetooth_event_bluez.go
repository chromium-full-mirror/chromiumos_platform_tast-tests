// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"bytes"
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MonitorBluetoothEventBluez,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Monitors whether Bluetooth events are detected properly when the system is using Bluez",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"byronlee@chromium.org",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health
		BugComponent: "b:982097",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"diagnostics"},
		Fixture:      "crosHealthdRunningAndBluetoothEnabledWithBlueZ",
		TestBedDeps:  []string{tbdep.BluetoothStateNormal},
	})
}

func initiateBluetoothStatus(ctx context.Context, s *testing.State) error {
	// Set the power off first.
	b, err := testexec.CommandContext(ctx, "bluetoothctl", "power", "off").Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "failed to trigger Bluetooth power off: %s", string(b))
	}
	s.Log("bluetoothctl: ", strings.Trim(string(b), "\n"))

	return nil
}

func MonitorBluetoothEventBluez(ctx context.Context, s *testing.State) {
	if err := initiateBluetoothStatus(ctx, s); err != nil {
		s.Fatal("Failed to initiate bluetooth status, err: ", err)
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
		stdout := stdoutBuf.String()
		if !strings.Contains(stdout, "Subscribe to bluetooth events successfully") {
			return errors.Errorf("failed to subscirbe Bluetooth event, stdout: %s", stdout)
		}
		return nil
	}, &testing.PollOptions{Interval: 200 * time.Millisecond, Timeout: 1 * time.Second}); err != nil {
		s.Fatal("Failed to subscirbe event in healthd: ", err)
	}

	// Trigger Bluetooth event.
	b, err := testexec.CommandContext(ctx, "bluetoothctl", "power", "on").Output(testexec.DumpLogOnError)
	if err != nil {
		if cmdErr := monitorCmd.Kill(); cmdErr != nil {
			s.Log(ctx, "Error killing healthd monitor command: ", cmdErr)
		}
		monitorCmd.Wait()
		s.Fatal("Failed to trigger Bluetooth power on event: ", err)
	}
	s.Log("bluetoothctl: ", strings.Trim(string(b), "\n"))

	if err := monitorCmd.Wait(); err != nil {
		s.Fatal("Failed to wait healthd monitor command: ", err)
	}

	stderr := stderrBuf.String()
	if stderr != "" {
		s.Fatal("Failed to detect Bluetooth on event, stderr: ", stderr)
	}

	stdout := stdoutBuf.String()
	if !strings.Contains(stdout, "Bluetooth event received") {
		s.Fatal("Failed to detect Bluetooth on event, event output: ", stdout)
	}
}
