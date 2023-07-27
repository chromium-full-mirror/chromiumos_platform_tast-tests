// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hibernate

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/hibernate/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpdateEngineCancelsHibernate,
		Desc: "Starts hibernate with update engine running and checks that hibernate aborts",
		Contacts: []string{
			"chromeos-hibernate@google.com", "dvjimenez@google.com",
		},
		BugComponent: "b:167191",
		Timeout:      3 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      fixture.UpdateEngine, // Ensures update engine is ready and resets its status.
	})
}

// UpdateEngineCancelsHibernate starts hibernate with update engine running and checks that hibernate aborts.
func UpdateEngineCancelsHibernate(ctx context.Context, s *testing.State) {
	s.Log("Start utils.StreamLogs()")
	logCmd, logCh, err := utils.StreamLogs(ctx)
	if err != nil {
		s.Fatal("Failed to start utils.StreamLogs(): ", err)
	}
	defer logCmd.Wait()
	defer logCmd.Kill()

	updateEngineCmd := []string{"update_engine_client", "--interactive=false", "--set_status=1"}

	// Set up Update Engine to run
	_, err = testexec.CommandContext(ctx, updateEngineCmd[0], updateEngineCmd[1:]...).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to execute %v: %v", updateEngineCmd, err)
	}

	// Try to hibernate
	hibermanCmd := []string{"hiberman", "hibernate", "--test-keys", "--dry-run"}
	_, err = testexec.CommandContext(ctx, hibermanCmd[0], hibermanCmd[1:]...).Output(testexec.DumpLogOnError)
	// We expect the command to fail! If it doesn't fail, it means we ignored update_client's state
	if err == nil {
		s.Fatal("Expected hiberman to fail but didn't: ", hibermanCmd)
	}

	// Check for messages log declaring hiberman aborted the hibernation process
	var hibermanHibernateLogs = []string{
		"Beginning hibernate",
		"Failed to hibernate: Update engine is active",
	}
	if err := utils.DetectLogs(ctx, s, logCh, hibermanHibernateLogs); err != nil {
		s.Fatal("Failed to execute utils.DetectLog(): ", err)
	}
}
