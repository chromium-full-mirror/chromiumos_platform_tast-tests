// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"encoding/json"
	"os"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	initTimePNPFeatureBenchmark                = 1 * time.Minute
	featureBenchamrkExecutableExtraRunningTime = 10 * time.Second

	benchmarkExec                   = "cros_camera_feature_benchmark"
	featureBenchmarkMetricsJSONFile = "feature_benchmark_metrics.json"
	faceDetectionTestImageFile      = "person_4032x3024.nv12"
	faceDetectionConfig             = "feature_benchmark_face_detection_config.json"
)

type featureBenchmarkParams struct {
	benchmarkConfig string
	testCaseName    string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPFeatureBenchmark,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect benchmark metrics of ChromeOS Camera features",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:camera_dependent"},
		SoftwareDeps: []string{caps.BuiltinCamera, "chrome", "camera_app"},
		Fixture:      pnp.StablePowerNoUI,
		Timeout:      initTimePNPFeatureBenchmark + pnp.PNPTimeParams.Total + power.RecorderTimeout,
		Params: []testing.Param{{
			Name: "face_detection_1080p",
			Val: featureBenchmarkParams{
				benchmarkConfig: faceDetectionConfig,
				testCaseName:    "face_detection_1080p",
			},
			ExtraData: []string{faceDetectionTestImageFile, faceDetectionTestImageFile + ".json", faceDetectionConfig},
		}, {
			Name: "face_detection_720p",
			Val: featureBenchmarkParams{
				benchmarkConfig: faceDetectionConfig,
				testCaseName:    "face_detection_720p",
			},
			ExtraData: []string{faceDetectionTestImageFile, faceDetectionTestImageFile + ".json", faceDetectionConfig},
		}, {
			Name: "face_detection_480p",
			Val: featureBenchmarkParams{
				benchmarkConfig: faceDetectionConfig,
				testCaseName:    "face_detection_480p",
			},
			ExtraData: []string{faceDetectionTestImageFile, faceDetectionTestImageFile + ".json", faceDetectionConfig},
		}},
	})
}

func parseFeatureBenchmarkMetrics(ctx context.Context, pv *perf.Values, metricPath string) error {
	metricByte, err := os.ReadFile(metricPath)
	if err != nil {
		return errors.Wrap(err, "failed to read file: "+metricPath)
	}

	var metrics []pnp.FunctionMetric
	err = json.Unmarshal([]byte(metricByte), &metrics)
	if err != nil {
		return errors.Wrap(err, "failed to parse JSON")
	}

	for _, metric := range metrics {
		pnp.SetMetricFromFunction(pv, metric, "")
	}
	return nil
}

func PNPFeatureBenchmark(ctx context.Context, s *testing.State) {
	// Reserve some time to cleanup, even if it fails due to ctx timeout.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	pnpRoutine := pnp.Routine{}
	defer pnpRoutine.Close(cleanupCtx)
	if err := pnp.Cooldown(ctx); err != nil {
		s.Fatal("Failed to run pnp cooldown routine: ", err)
	}

	testing.ContextLog(ctx, "[Start Work Phase]")
	featureBenchmarkMetricsJSONPath := filepath.Join(s.OutDir(), featureBenchmarkMetricsJSONFile)
	cmd := testexec.CommandContext(
		ctx, benchmarkExec,
		"--test_config_file_path="+s.DataPath(s.Param().(featureBenchmarkParams).benchmarkConfig),
		"--test_case_name="+s.Param().(featureBenchmarkParams).testCaseName,
		"--min_running_time_sec="+strconv.Itoa(int((pnp.PNPWarmUpTime+pnp.PNPTimeParams.Total+featureBenchamrkExecutableExtraRunningTime).Seconds())),
		"--metrics_output_json_path="+featureBenchmarkMetricsJSONPath)

	err := cmd.Start()
	if err != nil {
		s.Fatal("Failed to run ", benchmarkExec, ": ", err)
	}

	if err := pnp.WarmUp(ctx); err != nil {
		s.Fatal("Failed to run pnp warm up routine: ", err)
	}
	if err := pnpRoutine.MeasurePower(ctx, cleanupCtx, s.OutDir(), s.TestName(), false); err != nil {
		s.Fatal("Failed to run pnp power measuring routine: ", err)
	}

	if err = cmd.Wait(); err != nil {
		cmd.DumpLog(ctx)
		s.Fatal("Failed to wait for feature benchmark termination: ", err)
	}

	// Collect information of logs from benchmark binary.
	cmd.DumpLog(ctx)

	pv := perf.NewValues()
	if err := parseFeatureBenchmarkMetrics(ctx, pv, featureBenchmarkMetricsJSONPath); err != nil {
		s.Fatal("Failed to parse metrics from path ", featureBenchmarkMetricsJSONPath, ": ", err)
	}

	if err := pnpRoutine.UploadMetrics(ctx, cleanupCtx, pv); err != nil {
		s.Fatal("Failed to upload metrics: ", err)
	}

}
