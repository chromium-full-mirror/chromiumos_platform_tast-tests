// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/crasbench"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasBench,
		Desc:         "Micro-benchmarks for the ChromeOS audio server",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "paulhsia@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"group:crosbolt", "crosbolt_perbuild",
		},
		Timeout: 2 * time.Minute,
		Fixture: "rebootForAudioDSPFixture",
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
				Name:              "dsp_am",
				ExtraSoftwareDeps: []string{"dlc"},
				Val:               crasbench.AM,
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

func CrasBench(ctx context.Context, s *testing.State) {
	crasbench.Run(ctx, s)
}
