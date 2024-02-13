// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/ui/cliprectperf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	localPerf "go.chromium.org/tast-tests/cros/local/perf"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"

	"go.chromium.org/tast/core/ctxutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ClipRectPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Performance test to test Clip Rect Delegation for Lacros",
		Contacts: []string{
			"cros-sw-perf@google.com",
			"vincentchiang@google.com",
		},
		BugComponent: "b:1045832", // ChromeOS > Software > Performance > TPS
		SoftwareDeps: []string{"chrome", "lacros"},
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Timeout:      10 * time.Minute,
		Fixture:      "lacros",
		Params: []testing.Param{
			{
				Name: "video",
				Val: cliprectperf.ClipRectParam{
					Variant: "video",
				},
			},
			{
				// Number of videos that would be played during the test.
				Name: "multi_video",
				Val: cliprectperf.ClipRectParam{
					Variant: "multi_video",
				},
			},
		},
	})
}

func ClipRectPerf(ctx context.Context, s *testing.State) {
	// Shortened context for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	pv, err := localPerf.CaptureDeviceSnapshot(ctx, "Initial")
	if err != nil {
		s.Fatal("Failed to capture device snapshot: ", err)
	}

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	const videoURL = "https://storage.googleapis.com/watk/v"
	videoConn, br, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browser.TypeLacros, videoURL)
	if err != nil {
		s.Fatal("Failed to set up Chrome: ", err)
	}
	defer closeBrowser(closeCtx)
	defer videoConn.Close()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API connection: ", err)
	}

	// Maximize browser window.
	ws, err := ash.GetAllWindows(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get windows: ", err)
	}
	if wsCount := len(ws); wsCount != 1 {
		s.Fatalf("Unexepcted window count; got %d, expected 1", wsCount)
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, ws[0].ID, ash.WindowStateMaximized); err != nil {
		s.Fatal("Failed to maximize window: ", err)
	}

	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to browser test API connection: ", err)
	}

	defer faillog.DumpUITreeWithScreenshotOnError(closeCtx, s.OutDir(), s.HasError, cr, "ui_dump")

	recorder, err := cujrecorder.NewRecorder(ctx, cr, bTconn, nil, cujrecorder.RecorderOptions{})
	if err != nil {
		s.Fatal("Failed to create a recorder: ", err)
	}
	defer recorder.Close(closeCtx)

	if err := recorder.AddCommonMetrics(tconn, bTconn); err != nil {
		s.Fatal("Failed to add common metrics to the recorder: ", err)
	}

	// Add an empty screenshot recorder.
	if err := recorder.AddScreenshotRecorder(ctx, 0, 1); err != nil {
		s.Log("Failed to add screenshot recorder: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in clamshell mode: ", err)
	}
	defer cleanup(closeCtx)

	pc := pointer.NewMouse(tconn)
	defer pc.Close(closeCtx)

	// Set the page to the desired state.
	script, err := cliprectperf.GetScript(s.Param().(cliprectperf.ClipRectParam).Variant)
	if err != nil {
		s.Fatal("Failed to get setup script: ", err)
	}
	if err := videoConn.Eval(ctx, script, nil); err != nil {
		s.Fatal("Failed to set up the page: ", err)
	}

	videoWindow, err := ash.GetActiveWindow(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get active window: ", err)
	}

	if err := recorder.Run(ctx, func(ctx context.Context) error {
		if err := pc.ClickAt(videoWindow.BoundsInRoot.CenterPoint())(ctx); err != nil {
			return errors.Wrap(err, "failed to focus the video window")
		}

		// Play the video.
		if err := videoConn.Eval(ctx, `
			for (const v of vids) {
				v.play();
			}
		`, nil); err != nil {
			return errors.Wrap(err, "failed to play video")
		}

		s.Log("Sleeping for 5 minutes to record metrics")
		// GoBigSleepLint: Run until total test time has passed.
		if err := testing.Sleep(ctx, 5*time.Minute); err != nil {
			return errors.Wrap(err, "failed to sleep to close out the test")
		}
		return nil
	}); err != nil {
		s.Fatal("Failed to conduct performance test: ", err)
	}

	frameCounts := make(map[string]float64)
	if err := videoConn.Eval(ctx, `
	new Promise(resolve => {
		let frameMap = new Map();

		let droppedFrames = 0;
		let decodedFrames = 0;
		let count = 1;
		for (const v of vids) {
			let name = "video" + String(count);
			let dropped = v.webkitDroppedFrameCount;
			let decoded = v.webkitDecodedFrameCount;
			droppedFrames += v.webkitDroppedFrameCount;
			decodedFrames += v.webkitDecodedFrameCount;
			frameMap["DroppedFrames." + name] = dropped;
			frameMap["DecodedFrames." + name] = decoded;
			frameMap["PercentDroppedFrames." + name] = dropped/ decoded;
			count += 1;
		}
		frameMap["DroppedFrames.total"] = droppedFrames;
		frameMap["DecodedFrames.total"] = decodedFrames;
		frameMap["PercentDroppedFrames.total"] = droppedFrames / decodedFrames;
		resolve(frameMap);
	})`, &frameCounts); err != nil {
		s.Fatal("Failed to get frame metrics: ", err)
	}

	for metric, value := range frameCounts {
		pv.Set(perf.Metric{
			Name:      "Video." + metric,
			Unit:      "frames",
			Direction: perf.SmallerIsBetter,
		}, float64(value))
	}

	if err := recorder.Record(ctx, pv); err != nil {
		s.Fatal("Failed to report: ", err)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Error("Failed to store values: ", err)
	}
}
