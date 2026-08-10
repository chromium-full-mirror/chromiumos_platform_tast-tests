// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"sync"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/crastests"
	audiofixture "go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/audio/nodematch"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     CrasLoopbackCaptureTimestamp,
		Desc:     "Flexible loopback should provide samples in correct timestamp order",
		Contacts: []string{"htcheong@google.com", "chromeos-audio-bugs@google.com"},
		Fixture: audiofixture.AloopLoaded{
			Channels: 2,
			Parent:   audiofixture.UIStopped{}.Instance(),
		}.Instance(),
		BugComponent:    "b:776546",
		Attr:            []string{"group:mainline", "informational"},
		VariantCategory: `{"name": "Audio_Board"}`,
	})
}

func CrasLoopbackCaptureTimestamp(ctx context.Context, s *testing.State) {
	const duration = 10 * time.Second // In Seconds
	cras, err := audio.RestartCras(ctx)
	if err != nil {
		s.Fatal("Failed to restart CRAS: ", err)
	}

	if err := audio.SelectIODevices(ctx, cras, nodematch.Type("ALSA_LOOPBACK"), nodematch.Type("ALSA_LOOPBACK")); err != nil {
		s.Fatal("Failed to set internal device as selected device: ", err)
	}

	floopDev, _ := crastestclient.RequestFloopMask(ctx, 255)
	defer audio.RestartCras(ctx)

	// Add 5 second buffer.
	runCtx, cancel := context.WithTimeout(ctx, duration+5*time.Second)
	defer cancel()

	playbackCmd := crastests.PlaybackFileCommand(runCtx, "/dev/urandom", int(duration.Seconds()), 2, 48000)
	captureCmd := crastestclient.PinCaptureCommand(runCtx, floopDev, int(duration.Seconds()), 480)
	captureCmd.Args = append(captureCmd.Args, "--show_ooo_timestamp")

	// Add a wait group to ensure both playback and capture command executed.
	waitGroup := sync.WaitGroup{}
	waitGroup.Add(2)
	go func() {
		defer waitGroup.Done()
		s.Log("Running command: ", playbackCmd)
		if err := playbackCmd.Run(); err != nil {
			s.Fatal("Failed in running playback: ", err)
		}
	}()
	go func() {
		defer waitGroup.Done()
		s.Log("Running command: ", captureCmd)
		if err := captureCmd.Run(); err != nil {
			s.Fatal("Failed to obtain ordered timestamp with capture: ", err)
		}
	}()

	waitGroup.Wait()
}
