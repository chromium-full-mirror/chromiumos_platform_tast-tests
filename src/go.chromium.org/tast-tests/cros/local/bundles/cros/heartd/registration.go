// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package heartd tests the system daemon heartd functionality.
package heartd

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Registration,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that we can register heartd service",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"kerker@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:heartd", "heartd_perbuild"},
	})
}

func Registration(ctx context.Context, s *testing.State) {
	// Activate heartd.
	if err := testexec.CommandContext(ctx, "dbus-send", "--system", "--type=signal", "--dest=org.chromium.Heartd", "/org/chromium/Heartd", "org.chromium.Heartd").Run(); err != nil {
		s.Fatal("Failed to activate heartd through D-Bus: ", err)
	}

	// Wait for heartd running.
	if err := upstart.WaitForJobStatus(ctx, "heartd", upstartcommon.StartGoal, upstartcommon.RunningState, upstart.TolerateWrongGoal, 5*time.Second); err != nil {
		s.Fatal("Failed waiting for heartd to start: ", err)
	}

	// Register action.
	output, err := testexec.CommandContext(ctx, "/usr/bin/heartd-tool", "--action1=kNoOperation", "--simulate_client_missing").Output()
	if err != nil {
		s.Fatal("Failed to run heartd-tool: ", err)
	}

	// Verification.
	if !strings.Contains(string(output), "Registration success") {
		s.Fatal("Failed to register heartbeat service, heartd-tool output: ", output)
	}

	// Make sure heartd doesn't crash.
	_, status, _, err := upstart.JobStatus(ctx, "heartd")
	if err != nil {
		s.Fatal("Failed to get the heartd status: ", err)
	}
	if status != upstartcommon.RunningState {
		s.Fatal("Heartd crashes after registration")
	}

	// Stop heartd.
	if err := upstart.StopJob(ctx, "heartd"); err != nil {
		s.Fatal("Failed to stop upstart job heartd: ", err)
	}
}
