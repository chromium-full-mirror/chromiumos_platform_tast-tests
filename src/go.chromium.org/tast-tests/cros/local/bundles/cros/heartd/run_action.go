// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package heartd tests the system daemon heartd functionality.
package heartd

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RunAction,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that we can run the action without crash",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"kerker@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:heartd", "heartd_perbuild"},
		SoftwareDeps: []string{"no_arm"},
		Params: []testing.Param{{
			Name: "sync_data",
			Val:  "kSyncData", // Action enum string in heartd.
		}},
	})
}

func RunAction(ctx context.Context, s *testing.State) {
	// Activate heartd.
	if err := testexec.CommandContext(ctx, "dbus-send", "--system", "--type=signal", "--dest=org.chromium.Heartd", "/org/chromium/Heartd", "org.chromium.Heartd").Run(); err != nil {
		s.Fatal("Failed to activate heartd through D-Bus: ", err)
	}

	// Wait for heartd running and record the pid.
	if err := upstart.WaitForJobStatus(ctx, "heartd", upstartcommon.StartGoal, upstartcommon.RunningState, upstart.TolerateWrongGoal, 5*time.Second); err != nil {
		s.Fatal("Failed waiting for heartd to start: ", err)
	}
	_, _, pid, err := upstart.JobStatus(ctx, "heartd")
	if err != nil {
		s.Fatal("Failed to get heartd PID: ", err)
	}

	// Run action.
	action := s.Param().(string)
	if err := testexec.CommandContext(ctx, "/usr/local/bin/heartd-tool", "--action1="+action, "--run_action_now").Run(); err != nil {
		s.Fatal("Failed to run heartd-tool: ", err)
	}

	// Make sure heartd doesn't crash.
	_, status, pidAfterTest, err := upstart.JobStatus(ctx, "heartd")
	if err != nil {
		s.Fatal("Failed to get the heartd status: ", err)
	}
	if status != upstartcommon.RunningState || pid != pidAfterTest {
		s.Fatal("Heartd crashes after registration")
	}

	// Stop heartd.
	if err := upstart.StopJob(ctx, "heartd"); err != nil {
		s.Fatal("Failed to stop upstart job heartd: ", err)
	}
}
