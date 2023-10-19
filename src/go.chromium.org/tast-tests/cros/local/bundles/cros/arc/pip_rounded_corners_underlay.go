// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/arcpipvideotest"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PIPRoundedCornersUnderlay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that ARC++ PIP rounded corners are implemented with a hardware underlay",
		Contacts:     []string{"chromeos-perf@google.com", "petermcneeley@chromium.org", "oshima@chromium.org"},
		BugComponent: "b:1021073",
		Attr:         []string{"group:mainline"},
		// Video playback doesn't work well on VM boards.
		SoftwareDeps: []string{"chrome", "no_qemu", "proprietary_codecs"},
		HardwareDeps: hwdep.D(hwdep.SupportsNV12Overlays()),
		Data:         []string{"180p_60fps_600frames.h264.mp4"},
		Fixture:      "gpuWatchDog",
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 2*time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"informational"},
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

func PIPRoundedCornersUnderlay(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.ARCEnabled())
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	device, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer device.Close(cleanupCtx)

	cleanUp, err := arcpipvideotest.EstablishARCPIPVideo(ctx, tconn, a, device, s.DataFileSystem(), false)
	if err != nil {
		s.Fatal("Failed to establish ARC PIP video: ", err)
	}
	defer cleanUp(cleanupCtx)

	hists, err := metrics.Run(ctx, tconn, func(ctx context.Context) error {
		// GoBigSleepLint: TODO(b/306491449): Justify sleep
		if err := testing.Sleep(ctx, time.Second); err != nil {
			return errors.Wrap(err, "failed to wait a second")
		}
		return nil
	}, "Viz.DisplayCompositor.OverlayStrategy")
	if err != nil {
		s.Fatal("Failed to record histogram Viz.DisplayCompositor.OverlayStrategy: ", err)
	}

	hist := hists[0]
	if len(hist.Buckets) == 0 {
		s.Fatal("Got no overlay strategy data")
	}

	for _, bucket := range hist.Buckets {
		// bucket.Min will be from enum OverlayStrategies as defined
		// in tools/metrics/histograms/enums.xml in the chromium
		// code base. We want the PIP video promoted to overlay with
		// the underlay overlay strategy (4) or not at all (1,6,7).
		if bucket.Min != 1 && bucket.Min != 4 && bucket.Min != 6 && bucket.Min != 7 {
			s.Errorf("Found %d frame(s) with an unexpected overlay strategy: got %d; want 1, 4, 6, or 7", bucket.Count, bucket.Min)
		}
	}
}
