// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SSHStability,
		Desc: "Runs tets and checks the Servo power state accordingly",
		Contacts: []string{
			"peep-fleet-infra-sw@google.com",
		},
		BugComponent: "b:1032353", // Chrome Operations > Fleet > Software > OS Fleet Automation
		Fixture:      fixture.NormalMode,
		Attr:         []string{"group:labqual_informational", "group:labqual_stable"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// SSHStability restarts dut and verifies the dut can still be ssh'd up to a certain time limit
func SSHStability(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	// Reboot dut
	s.Log("Rebooting DUT")
	if err := s.DUT().Reboot(ctx); err != nil {
		s.Fatalf("Failed to reboot DUT: %s", err)
	}

	// Checking dut connection intermittently
	s.Log("Checking DUT SSH connection intermittently")
	if err := waitAndRetryDUTConnection(ctx, s); err != nil {
		s.Fatalf("Failed to reconnect to DUT: %s", err)
	}

	s.Log("DUT SSH connection stability verified")
}

// waitAndRetryDUTConnection waits and retries dut ssh connection until the specified timeout has passed
func waitAndRetryDUTConnection(ctx context.Context, s *testing.State) error {
	const sshConnectTimeout = 3 * time.Minute
	const retryConnectDelay = 30 * time.Second
	retryConnectCtx, cancelRetryConnect := context.WithTimeout(ctx, sshConnectTimeout)
	defer cancelRetryConnect()
	dut := s.DUT()
	// Try the ssh connection a few times with sleep
	for i := 0; i < 5; i++ {
		s.Log("Waiting for DUT to connect")
		if err := dut.WaitConnect(retryConnectCtx); err != nil {
			return errors.Wrap(err, "failed to reconnect to DUT")
		}
		s.Log("DUT connection established")

		// GoBigSleepLint: Sleep for 30 seconds before trying to connect to dut again.
		if err := testing.Sleep(ctx, retryConnectDelay); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
	}
	return nil
}
