// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"bytes"
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	upstartcommon "go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/meet/constants"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/shutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ServiceActivity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the Meet services needed for CFM functionality is running and can be started and stopped",
		Contacts: []string{
			"core-devices@google.com",
			"joshuapius@google.com", // Test author
		},
		BugComponent: "b:543707", // Communications > Video (Meet) > Platforms > Rooms > Core Devices (OS & Hardware)
		Attr:         []string{"group:meet", "group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "meets_device"},
	})
}

func ServiceActivity(ctx context.Context, s *testing.State) {
	for _, service := range constants.ConstantServices {
		var stderr bytes.Buffer
		var stdout bytes.Buffer

		// Wait until the condition is true.
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			_, state, _, err := upstart.JobStatus(ctx, service)
			if err != nil {
				s.Errorf("'status %s' command failed: %v", service, err)
			}

			if state == upstartcommon.RunningState {
				// Confirmed the service is running.
				return nil
			}

			if state != upstartcommon.WaitingState {
				// Capture non errors with unique state output.
				return errors.Errorf("'status %s' command failed, service not supported/available: %s", service, state)
			}

			// Service is waiting or not running.
			cmd := testexec.CommandContext(ctx, "start", service)
			cmd.Stderr = &stderr
			cmd.Stdout = &stdout
			if err := cmd.Run(testexec.DumpLogOnError); err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to start service"))
			}
			if !strings.Contains(stdout.String(), string(upstartcommon.RunningState)) {
				return errors.Errorf("%q command failed, %s service not started: %q", shutil.EscapeSlice(cmd.Args), service, cmd.DumpLog(ctx))
			}
			return nil
		}, &testing.PollOptions{
			Timeout: 3 * time.Second,
		}); err != nil {
			s.Errorf("%s failed to start. Did not reach expected state: %v", service, err)
		}
	}
}
