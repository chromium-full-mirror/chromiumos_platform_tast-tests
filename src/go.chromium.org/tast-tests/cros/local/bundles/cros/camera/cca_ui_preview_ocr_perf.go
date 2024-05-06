// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIPreviewOCRPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure the average OCR scanning latency on preview in photo mode",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Data:         []string{"ocr_no_text_3264x2448.jpg", "ocr_one_line_3264x2448.jpg", "ocr_full_of_text_3264x2448.jpg"},
		Fixture:      "ccaTestBridgeReadyWithFakeHALCameraWithPreviewOCR",
		// 3 subtests each has 2 minutes 20 seconds of timeout.
		// 3 * (2min 20 secs) + 2 minutes `cpu.WaitUntilIdle`.
		Timeout: 9 * time.Minute,
	})
}

type previewOCRPerfSubTest struct {
	name  string
	scene string
}

func CCAUIPreviewOCRPerf(ctx context.Context, s *testing.State) {
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Fatal("Failed to wait until CPU become idle")
	}

	p := perf.NewValues()

	measureTime := 2 * time.Minute
	subTestTimeout := measureTime + 20*time.Second
	for _, tst := range []previewOCRPerfSubTest{{
		"testFullOfText",
		"ocr_full_of_text_3264x2448.jpg",
	}, {
		"testOneLine",
		"ocr_one_line_3264x2448.jpg",
	}, {
		"testNoText",
		"ocr_no_text_3264x2448.jpg",
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := switchScene(ctx, cca.SceneData{Path: s.DataPath(tst.scene), ScaleMode: "contain"}); err != nil {
				s.Fatal("Failed to setup scene: ", err)
			}
			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				return testPreviewOCRPerf(ctx, app, &tst, measureTime, p)
			}, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to run %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}
	if err := p.Save(s.OutDir()); err != nil {
		s.Fatal("Failed to save perf data: ", err)
	}
}

func testPreviewOCRPerf(ctx context.Context, app *cca.App, tst *previewOCRPerfSubTest, measureTime time.Duration, p *perf.Values) error {
	// GoBigSleepLint: Keep the camera stream to measure the average latency of OCR scanning on preview.
	if err := testing.Sleep(ctx, measureTime); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	averageLatency, err := app.GetAverageOCRScanningLatency(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get the average latency of OCR scanning")
	}

	testing.ContextLogf(ctx, "%v: Average OCR scanning latency => %v ms", tst.name, averageLatency)
	p.Set(perf.Metric{
		Name:      tst.name,
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, averageLatency)

	return nil
}
