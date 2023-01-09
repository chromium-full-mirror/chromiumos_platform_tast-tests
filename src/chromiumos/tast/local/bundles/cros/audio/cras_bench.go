// Copyright 2021 The ChromiumOS Authors
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
	// NOTE: when modifying a test here please also mirror the changes to audio.CrasBenchSmoke.
	// audio.CrasBench uploads results to crosbolt but does not prevent breakage.
	testing.AddTest(&testing.Test{
		Func:         CrasBench,
		Desc:         "Micro-benchmarks for the ChromeOS audio server",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "paulhsia@chromium.org", "cychiang@chromium.org"},
		BugComponent: "b:875484",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
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
