// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// TODO(b/244254621): remove "sasukette" when b/244254621 is fixed.
// TODO(b/312097873): brya models are known to be unstable due to soundwire, but we still need them in the lab. b/309904720
var crasPlayUnstableModels = []string{"sasukette", "brya"}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasPlay,
		Desc:         "Verifies CRAS playback function works correctly",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "yuhsuan@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:776546",

		HardwareDeps: hwdep.D(hwdep.Speaker()),
		// Not to require 'rebootForAudioDSPFixture' to monitor the DSP failure rate. Hatch and Octopus is known to have 2% failure rate. (b/240269271)
		Fixture: fixture.UIStopped{}.Instance(),
		Attr:    []string{"group:mainline"},
		Params: []testing.Param{{
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(crasPlayUnstableModels...)),
			ExtraAttr:         []string{"group:cq-medium", "group:cq-minimal"},
		}, {
			Name:              "unstable_model",
			ExtraHardwareDeps: hwdep.D(hwdep.Model(crasPlayUnstableModels...)),
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func CrasPlay(ctx context.Context, s *testing.State) {
	const duration = 5 // second

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}

	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_SPEAKER"); err != nil {
		crastestclient.DumpAudioDiagnostics(ctx, s.OutDir())
		s.Fatal("Failed to set internal speaker to active: ", err)
	}

	// Set timeout to duration + 1s, which is the time buffer to complete the normal execution.
	runCtx, cancel := context.WithTimeout(ctx, (duration+1)*time.Second)
	defer cancel()

	// Playback function by CRAS.
	command := testexec.CommandContext(
		runCtx, "cras_test_client",
		"--playback_file", "/dev/zero",
		"--duration", strconv.Itoa(duration),
		"--num_channels", "2",
		"--rate", "48000")
	command.Start()

	defer func() {
		if err := command.Wait(); err != nil {
			if err := crastestclient.DumpAudioDiagnostics(ctx, s.OutDir()); err != nil {
				s.Error("Failed to dump audio diagnostics: ", err)
			}
			s.Fatal("Playback did not finish in time: ", err)
		}
	}()

	devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
	if err != nil {
		s.Fatal("Failed to detect running output device: ", err)
	}

	s.Log("Output device: ", devName)

	if strings.Contains(devName, "Silent") {
		s.Fatal("Fallback to the silent device")
	}
}
