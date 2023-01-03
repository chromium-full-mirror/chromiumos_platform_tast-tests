// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"chromiumos/tast/local/bundles/cros/audio/crasbench"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	// NOTE: when modifying a test here please also mirror the changes to audio.CrasBench.
	// audio.CrasBenchSmoke prevent breakage in CQ but does not upload results to crosbolt.
	testing.AddTest(&testing.Test{
		Func:         CrasBenchSmoke,
		Desc:         "Smoke test of cras_bench",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "paulhsia@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:875484",
		Attr:         []string{"group:mainline"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{
			{
				Name: "apm",
				Val:  crasbench.APM,
			},
			{
				Name: "dsp",
				Val:  crasbench.DSP,
			},
			{
				Name: "cras_mixer_ops",
				Val:  crasbench.MixerOps,
			},
			{
				Name:              "alsa",
				ExtraHardwareDeps: hwdep.D(hwdep.Speaker()),
				Val:               crasbench.Alsa,
			},
		},
	})
}

func CrasBenchSmoke(ctx context.Context, s *testing.State) {
	crasbench.Run(ctx, s)
}
