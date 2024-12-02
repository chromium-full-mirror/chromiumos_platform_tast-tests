// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/debug"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FloopRecordStress,
		Desc:         "Verifies floop works while stressing capture streams",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "yuhsuan@google.com", "htcheong@google.com"},
		BugComponent: "b:776546",
		Fixture: fixture.AloopLoaded{
			Channels:    2,
			DevicePairs: 1,
			Parent:      fixture.UIStopped{}.Instance(),
		}.Instance(),
		Attr: []string{"group:mainline", "informational"},
	})
}

func FloopRecordStress(ctx context.Context, s *testing.State) {
	const duration = 60                   // seconds
	const toleranceUnderrunDuration = 1.0 // seconds

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}

	floop, err := crastestclient.RequestFloopMask(ctx, 4)
	if err != nil {
		s.Fatal("RequestFloopMask failed for mask=4: ", err)
	}

	if err := cras.SetActiveNodeByMatcher(ctx,
		nodematch.Name("Loopback Playback"),
	); err != nil {
		s.Fatal("Failed to SetActiveNodeByMatcher: ", err)
	}

	// Set timeout to duration * 2, which is the time buffer to complete the normal execution.
	runCtx, cancel := context.WithTimeout(ctx, duration*2*time.Second)
	defer cancel()

	// Playback function by CRAS.
	stream1 := crastestclient.PlaybackCommand(runCtx, duration, 512)
	stream2 := crastestclient.PlaybackCommand(runCtx, duration, 4096)

	stream1.Start()
	stream2.Start()

	defer crastestclient.DumpAudioDiagnosticsOnError(ctx, s.OutDir(), s.HasError)

	switchRecord := func() {
		record := crastestclient.PinCaptureCommand(runCtx, floop, 5, 480)
		record.Start()
		// GoBigSleepLint: Need some sleep to check whether underruns happen
		if err := testing.Sleep(ctx, 3*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
		debugInfo, err := debug.Dump(ctx)
		if err != nil {
			s.Fatal("Failed to dump debug Info: ", err)
		}
		if numStreams := len(debugInfo.Streams); numStreams != 5 {
			s.Fatalf("Expected to have 5 streams but got %d", numStreams)
		}
		for i := 0; i < 5; i++ {
			if debugInfo.Streams[i].UnderrunDurationSec > toleranceUnderrunDuration {
				s.Fatalf("The underrun duration %f > threshold %fs", debugInfo.Streams[i].UnderrunDurationSec, toleranceUnderrunDuration)
			}
		}
		if err := record.Wait(); err != nil {
			s.Fatal("Capture stream did not finish in time: ", err)
		}
	}

	for i := 0; i < 10; i++ {
		s.Logf("Record iteration: %d", i)
		switchRecord()
	}

	if err := stream1.Wait(); err != nil {
		s.Fatal("Stream1 did not finish in time: ", err)
	}

	if err := stream2.Wait(); err != nil {
		s.Fatal("Stream2 did not finish in time: ", err)
	}
}
