// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package example

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/upstart"
	"go.chromium.org/tast-tests/cros/services/cros/platform"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: UpstartArgs,
		Desc: "Demonstrates how args can be used with the upstart service",
		Contacts: []string{
			"chromeos-fingerprint@google.com",
			"hesling@chromium.org",
		},
		// ChromeOS > Platform > Services > Fingerprint
		BugComponent: "b:782045",
		Attr:         []string{"group:mainline", "group:fingerprint-cq", "informational"},
		ServiceDeps:  []string{"tast.cros.platform.UpstartService"},
	})
}

const timberslideLogPath = "/sys/kernel/debug/cros_ec/console_log"

// UpstartArgs demonstrates how you can use remote upstart commands
// with Args.
//
// This demonstration checks if the EC's timberslide instance
// has the correct "start" goal. This should always be the case, unless
// there is no CrOS-EC or timberslide hasn't been started properly.
//
// The timberslide job definition is in timberslide.conf. The specific
// instances are launched from ecloggers.conf.
func UpstartArgs(ctx context.Context, s *testing.State) {
	rpcClient, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	// Get upstartClient service client instance.
	upstartClient := platform.NewUpstartServiceClient(rpcClient.Conn)

	status, err := upstartClient.JobStatus(ctx, &platform.JobStatusRequest{
		JobName: "timberslide",
		Args: []*platform.Arg{
			{Key: "LOG_PATH", Value: timberslideLogPath},
		},
	})
	if err != nil {
		s.Fatal("Failed to get job status for timberslide LOG_PATH=",
			timberslideLogPath)
	}

	actualGoal := upstart.Goal(status.GetGoal())
	expectedGoal := upstart.StartGoal
	s.Logf("The EC timberslide job's goal is %q", actualGoal)
	if actualGoal != expectedGoal {
		s.Errorf("Got goal %q; expected %q", actualGoal, expectedGoal)
	}

	actualState := upstart.State(status.GetState())
	expectedState := upstart.RunningState
	s.Logf("The EC timberslide job's state is %q", expectedState)
	if actualState != expectedState {
		s.Errorf("Got state %q; expected %q", actualState, expectedState)
	}
}
