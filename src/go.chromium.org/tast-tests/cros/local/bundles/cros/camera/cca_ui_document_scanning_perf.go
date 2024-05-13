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
		Func:         CCAUIDocumentScanningPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measure the average latency of saving images as a searchable PDF using OCR in document scanning mode",
		Contacts:     []string{"chromeos-camera-eng@google.com", "chuhsuan@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"camera_app", "chrome", "ondevice_document_scanner_rootfs_or_dlc"},
		Data:         []string{"ocr_full_of_text_3264x2448.jpg"},
		// Some devices (such as dedede/blipper) may take up to 10 seconds to
		// crop a document from an image and 8 seconds to perform OCR on an
		// image.
		Params: []testing.Param{
			{
				Fixture: "ccaTestBridgeReadyWithFakeHALCamera",
				// testOnePage: 60 seconds
				// testTenPage: 600 seconds
				// 660 seconds + 2 minutes `cpu.WaitUntilIdle`.
				Timeout: 13 * time.Minute,
			},
			{
				Name:    "ocr_enabled",
				Fixture: "ccaTestBridgeReadyWithFakeHALCameraWithPDFOCR",
				// testOnePage: 120 seconds
				// testTenPage: 1200 seconds
				// 1320 seconds + 2 minutes `cpu.WaitUntilIdle`.
				Timeout: 24 * time.Minute,
			},
		},
	})
}

type documentScanningPerfSubTest struct {
	name      string
	pageCount int
}

func CCAUIDocumentScanningPerf(ctx context.Context, s *testing.State) {
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	switchScene := s.FixtValue().(cca.FixtureData).SwitchScene

	if err := cpu.WaitUntilIdle(ctx); err != nil {
		s.Log("Failed to wait until CPU became idle; giving up")
	}

	p := perf.NewValues()
	roundsPerSubTest := 5

	for _, tst := range []documentScanningPerfSubTest{
		{"testOnePage", 1},
		{"testTenPage", 10},
	} {
		// pageCount * 20 seconds (10s for cropping + 10s for OCR and saving) * rounds (1 warm up + roundsPerSubTest)
		timeout := time.Duration(tst.pageCount*20*(1+roundsPerSubTest)) * time.Second
		subTestCtx, cancel := context.WithTimeout(ctx, timeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := switchScene(ctx, cca.SceneData{Path: s.DataPath("ocr_full_of_text_3264x2448.jpg"), ScaleMode: "contain"}); err != nil {
				s.Fatal("Failed to setup scene: ", err)
			}
			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				return testDocumentScanningPerf(ctx, app, &tst, roundsPerSubTest, p)
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

func testDocumentScanningPerf(ctx context.Context, app *cca.App, tst *documentScanningPerfSubTest, rounds int, p *perf.Values) error {
	if err := app.EnterDocumentMode(ctx); err != nil {
		return errors.Wrap(err, "failed to enter document sub-mode")
	}

	// Warm up for 1 round.
	if err := takeImagesAndSave(ctx, app, tst.pageCount); err != nil {
		return errors.Wrap(err, "failed to take images and save as PDF")
	}

	accumulatedLatency := 0.0

	for i := 0; i < rounds; i++ {
		if err := takeImagesAndSave(ctx, app, tst.pageCount); err != nil {
			return errors.Wrap(err, "failed to take images and save as PDF")
		}

		latency, err := app.GetDocumentReviewLastFileLatency(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get the latency of saving the PDF")
		}
		testing.ContextLogf(ctx, "Round %v => %v ms", i+1, latency)
		accumulatedLatency += latency
	}

	latency := accumulatedLatency / float64(rounds)
	testing.ContextLogf(ctx, "%v: Average latency => %v ms", tst.name, latency)

	p.Set(perf.Metric{
		Name:      tst.name,
		Unit:      "milliseconds",
		Direction: perf.SmallerIsBetter,
	}, latency)

	return nil
}

// takeImagesAndSave takes `pageCount` images in document sub-mode and click "Save as PDF".
func takeImagesAndSave(ctx context.Context, app *cca.App, pageCount int) error {
	timeoutPerPage := 10 * time.Second

	if err := app.ClickShutter(ctx); err != nil {
		return errors.Wrap(err, "failed to click the shutter button")
	}
	if err := app.WaitForVisibleStateFor(ctx, cca.DocumentReview, true, timeoutPerPage); err != nil {
		return errors.Wrap(err, "failed to wait for document review")
	}
	for pageIndex := 1; pageIndex < pageCount; pageIndex++ {
		if err := app.Click(ctx, cca.DocumentAddPageButton); err != nil {
			return errors.Wrap(err, "failed to click the add page button")
		}
		if err := app.WaitForVisibleState(ctx, cca.DocumentReview, false); err != nil {
			return errors.Wrap(err, "failed to wait for review UI to show up")
		}
		if err := app.WaitForState(ctx, "camera-configuring", false); err != nil {
			return errors.Wrap(err, "failed to wait for state camera-configuring to turn off")
		}
		if err := app.ClickShutter(ctx); err != nil {
			return errors.Wrap(err, "failed to click the shutter button")
		}
		if err := app.WaitForVisibleStateFor(ctx, cca.DocumentReview, true, timeoutPerPage); err != nil {
			return errors.Wrap(err, "failed to wait for document review")
		}
	}

	start := time.Now()
	if err := app.Click(ctx, cca.DocumentSaveAsPdfButton); err != nil {
		return errors.Wrap(err, "failed to click \"Save as PDF\" button")
	}
	if err := app.WaitForVisibleStateFor(ctx, cca.DocumentReview, false, time.Duration(pageCount)*timeoutPerPage); err != nil {
		return errors.Wrap(err, "failed to wait for document review")
	}
	if err := app.WaitForState(ctx, "camera-configuring", false); err != nil {
		return errors.Wrap(err, "failed to wait for state camera-configuring to turn off")
	}
	dir, err := app.SavedDir(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get CCA default saved path")
	}
	if _, err := app.WaitForFileSaved(ctx, dir, cca.DocumentPDFPattern, start); err != nil {
		return errors.Wrap(err, "failed to wait for the PDF")
	}

	return nil
}
