// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	audiofixture "go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/loopback/lifecycle"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasPlaybackUnderLoad,
		Desc:         "Verifies CRAS playback function works correctly",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "htcheong@chromium.org"},
		Fixture:      audiofixture.AloopLoaded{Channels: 2, Parent: fixture.FakeCrasClient}.Instance(),
		BugComponent: "b:776546",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
	})
}

func CrasPlaybackUnderLoad(ctx context.Context, s *testing.State) {
	param := &lifecycle.Param{
		Playback: lifecycle.Playback(1, 6),
		Capture:  lifecycle.Capture(1, 6),
		ExtraActions: []lifecycle.Action{
			lifecycle.StressSystem(0, 7),
		},
		Checks: []lifecycle.Checker{
			lifecycle.CheckSineAnomaly(2, 5),
		},
	}
	lifecycle.TestLifecycle(ctx, s, param)
}
