// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package a11y

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/gtest"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const exec = "screen_ai_ocr_perf_test"
const dlcID = "screen-ai"

var images = []string{"no_text_3264x2448_20240320.jpg", "one_line_3264x2448_20240320.jpg", "full_of_text_3264x2448_20240320.jpg"}

func init() {
	testing.AddTest(&testing.Test{
		Func: OCRPerf,
		Desc: "Measures the time performance of Screen AI OCR library",
		Contacts: []string{
			"chrome-screen-ai@google.com", // Mailing list
			"chuhsuan@google.com",         // Test author
		},
		BugComponent: "b:1272894", // ChromeOS Public Tracker > Experiences > Accessibility > Machine Intelligence
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Data:         images,
		// Some devices took 10 seconds to perform OCR on an image with full of
		// text, and the test binary performs OCR for 8 times on an image. Set
		// total timeout to 10 * 8 * 3 (images) = 240 seconds.
		Timeout: 4 * time.Minute,
	})
}

// OCRPerf runs the perf test which exercises document scanner library
// directly and send the performance metrics to CrosBolt.
func OCRPerf(ctx context.Context, s *testing.State) {
	if err := dlc.Install(ctx, dlcID, ""); err != nil {
		s.Fatal("Failed to install Screen AI DLC: ", err)
	}

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Log("Give up waiting for CPU to become idle: ", err)
	}

	p := perf.NewValues()

	s.Log("Start measuring Screen AI OCR performance")
	outputDir := s.OutDir()
	reportPath := filepath.Join(outputDir, "perf.json")
	for _, image := range images {
		s.Logf("Run performance test on image: %s", image)
		if err := runOcrPerformanceTest(ctx, outputDir, reportPath, s.DataPath(image), p); err != nil {
			s.Fatal("Failed to run performance test on the image: ", err)
		}
		metricNameSuffix := strings.Split(image, ".")[0]
		if err := parseReportAndRecordMetrics(ctx, reportPath, metricNameSuffix, p); err != nil {
			s.Fatal("Failed to parse and set metrics: ", err)
		}
	}
	if err := p.Save(outputDir); err != nil {
		s.Fatal("Failed to save perf data: ", err)
	}
}

func runOcrPerformanceTest(ctx context.Context, outputDir, reportPath, imagePath string, p *perf.Values) error {
	report, err := gtest.New(
		filepath.Join(chrome.BinTestDir, exec),
		gtest.Logfile(filepath.Join(outputDir, exec+".log")),
		gtest.ExtraArgs(
			"--output_path="+reportPath,
			"--jpeg_image="+imagePath,
		),
	).Run(ctx)
	if err != nil {
		if report != nil {
			for _, name := range report.FailedTestNames() {
				testing.ContextLogf(ctx, "Failed test name: %s", name)
			}
		}
		return errors.Wrap(err, "failed to run test binary")
	}
	return nil
}

// parseReportAndRecordMetrics parses metrics from `reportPath` and` adds metrics to `p`.
func parseReportAndRecordMetrics(ctx context.Context, reportPath, metricNameSuffix string, p *perf.Values) error {
	b, err := os.ReadFile(reportPath)
	if err != nil {
		return errors.Wrap(err, "cannot read log file")
	}
	var metrics map[string]float64
	if err := json.Unmarshal(b, &metrics); err != nil {
		return errors.Wrap(err, "failed to unmarshal performance metrics")
	}
	for testName, milliseconds := range metrics {
		if testName != "PerformOcr" {
			return errors.Errorf("unrecognized test name %v", testName)
		}
		testing.ContextLogf(ctx, "Perf: %v => %v ms", testName, milliseconds)
		p.Set(perf.Metric{
			Name:      testName + "_" + metricNameSuffix,
			Unit:      "milliseconds",
			Direction: perf.SmallerIsBetter,
		}, milliseconds)
	}
	return nil
}
