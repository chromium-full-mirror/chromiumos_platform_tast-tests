// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"chromiumos/tast/local/shill"
	"chromiumos/tast/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		BugComponent: "b:1166446",
		Func:         ShillStability,
		Desc:         "Checks that shill isn't respawning",
		Contacts: []string{
			"cros-network-health-team@google.com", // Network Health team
			"stevenjb@google.com",                 // Test author
		},
		Attr: []string{"group:mainline"},
	})
}

func ShillStability(ctx context.Context, s *testing.State) {
	const (
		stabilityInterval = time.Second
		stabilityDuration = 20 * time.Second
	)

	// Returns PID of main shill process. Calls s.Fatal if we can't find
	// shill.
	getPID := func() int {
		_, _, pid, err := upstart.JobStatus(ctx, shill.JobName)
		if err != nil {
			s.Fatal("Failed to find shill job: ", err)
		} else if pid == 0 {
			s.Fatal("Shill is not running")
		}
		return pid
	}

	pid := getPID()

	testing.Poll(ctx, func(ctx context.Context) error {
		if latest := getPID(); latest != pid {
			s.Fatalf("PID changed: %v -> %v", pid, latest)
		}
		s.Log("PID stable at: ", pid)
		return errors.New("keep polling")
	}, &testing.PollOptions{Timeout: stabilityDuration, Interval: stabilityInterval})
}
