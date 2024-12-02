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
		Func:         FloopDeviceSwitch,
		Desc:         "Verifies floop works while switching output devices",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "yuhsuan@google.com", "htcheong@google.com"},
		BugComponent: "b:776546",
		Fixture: fixture.AloopLoaded{
			Channels:    2,
			DevicePairs: 2,
			Parent:      fixture.UIStopped{}.Instance(),
		}.Instance(),
		Attr: []string{"group:mainline", "informational"},
	})
}

func FloopDeviceSwitch(ctx context.Context, s *testing.State) {
	const duration = 30                   // seconds
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
		nodematch.Name("Loopback Capture"),
	); err != nil {
		s.Fatal("Failed to SetActiveNodeByMatcher: ", err)
	}

	// Set timeout to duration * 2, which is the time buffer to complete the normal execution.
	runCtx, cancel := context.WithTimeout(ctx, duration*2*time.Second)
	defer cancel()

	// Record from floop
	record := crastestclient.PinCaptureCommand(runCtx, floop, duration, 480)

	// Playback function by CRAS.
	stream1 := crastestclient.PlaybackCommand(runCtx, duration, 4096)
	stream2 := crastestclient.PlaybackCommand(runCtx, duration, 512)

	record.Start()
	stream1.Start()
	stream2.Start()

	defer crastestclient.DumpAudioDiagnosticsOnError(ctx, s.OutDir(), s.HasError)

	switchDeviceAndCheck := func(dev string) {
		if err := cras.SetActiveNodeByMatcher(ctx,
			nodematch.Name(dev),
		); err != nil {
			s.Fatal("Failed to SetActiveNodeByMatcher: ", err)
		}

		// GoBigSleepLint: Need some sleep to play audio after switching devices
		if err := testing.Sleep(ctx, 1*time.Second); err != nil {
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
	}

	for i := 0; i < 10; i++ {
		switchDeviceAndCheck("Loopback Playback 1")
		switchDeviceAndCheck("Loopback Playback")
	}

	if err := stream1.Wait(); err != nil {
		s.Fatal("Stream1 did not finish in time: ", err)
	}

	if err := stream2.Wait(); err != nil {
		s.Fatal("Stream2 did not finish in time: ", err)
	}
}
