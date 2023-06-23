// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"bytes"
	"context"
	"strings"
	"time"

	"chromiumos/tast/common/testexec"
	upstartcommon "chromiumos/tast/common/upstart"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IPPeripheralService,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the ip-peripheral service is running and can be started and stopped",
		Contacts: []string{
			"core-devices@google.com",
			"joshuapius@google.com", // Test author
		},
		BugComponent: "b:543707", // Communications > Video (Meet) > Platforms > Rooms > Core Devices (OS & Hardware)
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model("ambassador", "endeavour", "endeavour_c")),
	})
}

func IPPeripheralService(ctx context.Context, s *testing.State) {
	const (
		service = "ip-peripheral"
	)
	var stderr bytes.Buffer
	var stdout bytes.Buffer

	_, state, _, err := upstart.JobStatus(ctx, service)
	if err != nil {
		s.Fatalf("'status %s' command failed: %v", service, err)
	}

	if state == upstartcommon.RunningState {
		// Confirmed the service is running.
		return
	}

	if state != upstartcommon.WaitingState {
		// Capture non errors with unique state output.
		s.Errorf("'status %s' command failed, service not supported/available: %s", service, state)
	}

	// Service is waiting or not running.
	// Wait until the condition is true.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cmd := testexec.CommandContext(ctx, "start", service)
		cmd.Stderr = &stderr
		cmd.Stdout = &stdout
		if err := cmd.Run(testexec.DumpLogOnError); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to start service"))
		}
		if !strings.Contains(stdout.String(), string(upstartcommon.RunningState)) {
			return errors.Errorf("'status %s' command failed, %s service not started: %q", service, service, cmd.DumpLog(ctx))
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 3 * time.Second,
	}); err != nil {
		s.Fatal("Did not reach expected state: ", err)
	}

}
