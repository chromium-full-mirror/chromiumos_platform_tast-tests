// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os/exec"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: DBusStreamCounts,
		Desc: "Checks D-Bus methods for stream counts",
		Contacts: []string{
			"chromeos-audio-bugs@google.com",
			"aaronyu@google.com",
		},
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline"},
	})
}

// Time we are willing to wait until the stream count changes.
const streamCountChangeTimeout = 10 * time.Second

// checkNonChromeOutputStreamCount checks for non-chrome output stream counts to match wantCount.
func checkNonChromeOutputStreamCount(ctx context.Context, cras *audio.Cras, wantCount int) error {
	streams, err := cras.GetNumberOfNonChromeOutputStreams(ctx)
	if err != nil {
		return errors.Wrap(err, "cras.GetNumberOfNonChromeOutputStreams failed")
	}
	if streams != wantCount {
		return errors.Errorf("unexpected GetNumberOfNonChromeOutputStreams() = %d; want %d", streams, wantCount)
	}
	return nil
}

// pollNonChromeOutputStreamCount polls for non-chrome output stream counts to match wantCount.
func pollNonChromeOutputStreamCount(ctx context.Context, cras *audio.Cras, wantCount int) error {
	return testing.Poll(
		ctx,
		func(ctx context.Context) error {
			return checkNonChromeOutputStreamCount(ctx, cras, wantCount)
		},
		&testing.PollOptions{
			Timeout: streamCountChangeTimeout,
		},
	)
}

// stopClient cleans up the client and fails the test if the client is terminated for other unknown reason
func stopClient(s *testing.State, client *testexec.Cmd) {
	client.Kill()
	err := client.Wait()
	if exitError, ok := err.(*exec.ExitError); ok && exitError != nil && !exitError.Exited() {
		// The client killed by a signal is expected.
		return
	}
	// Report everything else, including err == nil, as an error.
	s.Errorf("Client %s should be terminated by SIGKILL; cmd.Wait() got %s", client, err)
}

func DBusStreamCounts(ctx context.Context, s *testing.State) {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS via D-Bus: ", err)
	}
	if err := checkNonChromeOutputStreamCount(ctx, cras, 0); err != nil {
		s.Error("Error checking non-chrome stream count initially: ", err)
	}

	testClient := testexec.CommandContext(ctx, "cras_test_client", "-P", "/dev/zero")
	if err := testClient.Start(); err != nil {
		s.Fatal("Cannot start testClient playback: ", err)
	}
	defer stopClient(s, testClient)
	if err := pollNonChromeOutputStreamCount(ctx, cras, 1); err != nil {
		s.Error("Error checking non-chrome stream count, with clients: testClient: ", err)
	}

	// Start uncounted clients.
	chromeClient := testexec.CommandContext(ctx, "cras_test_client", "-P", "/dev/zero",
		"--client_type=4", // Chrome
	)
	if err := chromeClient.Start(); err != nil {
		s.Fatal("Cannot start chromeClient playback: ", err)
	}
	defer stopClient(s, chromeClient)
	lacrosClient := testexec.CommandContext(ctx, "cras_test_client", "-P", "/dev/zero",
		"--client_type=8", // LaCrOS
	)
	if err := lacrosClient.Start(); err != nil {
		s.Fatal("Cannot start lacrosClient playback: ", err)
	}
	defer stopClient(s, lacrosClient)

	// GoBigSleepLint: Check that the stream count is the same after streamCountChangeTimeout.
	// We assumed that if a bug causes the stream count to change incorrectly,
	// it changes soon and changes only once.
	// If we observed the same stream count after streamCountChangeTimeout,
	// we are confident that it won't change anymore and
	// assume the implementation is correct.
	if err := testing.Sleep(ctx, streamCountChangeTimeout); err != nil {
		s.Fatal("Cannot sleep: ", err)
	}
	if err := checkNonChromeOutputStreamCount(ctx, cras, 1); err != nil {
		s.Error("Error checking non-chrome stream count, with clients: testClient + chromeClient + lacrosClient: ", err)
	}

	if err := testClient.Kill(); err != nil {
		s.Fatal("Cannot kill testClient: ", err)
	}
	if err := pollNonChromeOutputStreamCount(ctx, cras, 0); err != nil {
		s.Error("Error checking non-chrome stream count = 0, with clients: chromeClient + lacrosClient: ", err)
	}
}
