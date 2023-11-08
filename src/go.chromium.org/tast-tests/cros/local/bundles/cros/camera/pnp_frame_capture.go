// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	traceBin           = "/usr/local/share/camera/tracing/bin/trace.py"
	traceQueryJSONFile = "trace_query.json"
	traceTime          = 20 * time.Second
)

type functionMetric struct {
	FunctionName   string `json:"function_name"`
	MetricName     string `json:"metric_name"`
	Unit           string `json:"unit"`
	Value          int64  `json:"value"`
	BiggerIsBetter bool   `json:"direction"`
}

type traceMetrics struct {
	PerfettoProtosCameraCoreMetrics struct {
		Sessions []struct {
			Sid             int              `json:"sid"`
			FunctionMetrics []functionMetric `json:"function_metrics"`
			StreamMetrics   []struct {
				FunctionMetrics     []functionMetric `json:"function_metrics"`
				ResultBufferMetrics []struct {
					Stream struct {
						StreamID int64 `json:"stream_id"`
						Width    int   `json:"width"`
						Height   int   `json:"height"`
						Format   int   `json:"format"`
					} `json:"stream"`
					FunctionMetrics []functionMetric `json:"function_metrics"`
				} `json:"result_buffer_metrics"`
				ResultMetrics []struct {
					PartialResultNumber int              `json:"partial_result_number"`
					NumOutputBuffers    int              `json:"num_output_buffers"`
					FunctionMetrics     []functionMetric `json:"function_metrics"`
				} `json:"result_metrics"`
			} `json:"stream_metrics"`
		} `json:"sessions"`
	} `json:"perfetto.protos.camera_core_metrics"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPFrameCapture,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect latency information of frame capturing functions",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{caps.BuiltinCamera, "chrome", "camera_app"},
		Fixture:      pnp.StablePowerLacrosGAIA,
		Timeout:      1*time.Minute + traceTime,
	})
}

func setMetric(pv *perf.Values, name, unit string, value float64, direction bool) {
	// perf.Values valid metric names only allow "^[a-zA-Z0-9._-]{1,256}$"
	name = strings.Replace(name, "::", "-", -1)
	name = strings.Replace(name, " ", "-", -1)
	name = strings.Replace(name, "~", "Destructor-", -1)
	perfDirection := perf.SmallerIsBetter
	if direction {
		perfDirection = perf.BiggerIsBetter
	}

	pv.Set(perf.Metric{
		Name:      name,
		Unit:      unit,
		Direction: perfDirection,
	}, value)
}

func setMetricFromFunction(pv *perf.Values, fm functionMetric, prefix string) {
	setMetric(
		pv, fmt.Sprintf("%s%s_%s", prefix, fm.FunctionName, fm.MetricName),
		fm.Unit, float64(fm.Value), fm.BiggerIsBetter)
}

func parseMetrics(ctx context.Context, pv *perf.Values, traceDataAbsPath, outDir string) error {

	traceQueryJSONPath := filepath.Join(outDir, traceQueryJSONFile)
	reportCmd := testexec.CommandContext(
		ctx, traceBin, "report", "-i", traceDataAbsPath, "--metrics",
		"camera_core_metrics", "--metrics_output", "json", "--output_file",
		traceQueryJSONPath)
	_, err := reportCmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "failed to run metrics with trace_processor_shell")
	}

	traceQueryByte, err := os.ReadFile(traceQueryJSONPath)
	if err != nil {
		return errors.Wrap(err, "failed to cast json to byte array")
	}

	metrics := traceMetrics{}
	err = json.Unmarshal([]byte(traceQueryByte), &metrics)
	if err != nil {
		return errors.Wrap(err, "failed to convert metrics to json format")
	}

	// If there are multiple camera sessions, only use the last session to
	// exclude preparation time for redoing OpenDevice().
	if len(metrics.PerfettoProtosCameraCoreMetrics.Sessions) == 0 {
		return errors.Wrap(err, "failed to find any session")
	}
	session := metrics.PerfettoProtosCameraCoreMetrics.Sessions[len(metrics.PerfettoProtosCameraCoreMetrics.Sessions)-1]
	for _, functionMetric := range session.FunctionMetrics {
		setMetricFromFunction(pv, functionMetric, "")
	}

	// If there are multiple configurations, only use the last subsession to
	// exclude preparation time for redoing ConfigureStream().
	if len(session.StreamMetrics) == 0 {
		return errors.Wrap(err, "failed to find any stream in the last session")
	}
	stream := session.StreamMetrics[len(session.StreamMetrics)-1]
	for _, functionMetric := range stream.FunctionMetrics {
		setMetricFromFunction(pv, functionMetric, "")
	}

	for i, resultBuffer := range stream.ResultBufferMetrics {
		setMetric(pv, fmt.Sprintf("ResultBuffer_%d_streamID", i), "id", float64(resultBuffer.Stream.StreamID), false)
		setMetric(pv, fmt.Sprintf("ResultBuffer_%d_width", i), "pix", float64(resultBuffer.Stream.Width), false)
		setMetric(pv, fmt.Sprintf("ResultBuffer_%d_height", i), "pix", float64(resultBuffer.Stream.Height), false)
		setMetric(pv, fmt.Sprintf("ResultBuffer_%d_format", i), "category", float64(resultBuffer.Stream.Format), false)
		for _, functionMetric := range resultBuffer.FunctionMetrics {
			setMetricFromFunction(pv, functionMetric, fmt.Sprintf("ResultBuffer_%d_", i))
		}
	}

	for i, result := range stream.ResultMetrics {
		setMetric(pv, fmt.Sprintf("Result_%d_partial_result_number", i), "id", float64(result.PartialResultNumber), false)
		setMetric(pv, fmt.Sprintf("Result_%d_num_output_buffers", i), "count", float64(result.NumOutputBuffers), false)
		for _, functionMetric := range result.FunctionMetrics {
			setMetricFromFunction(pv, functionMetric, fmt.Sprintf("Result_%d_", i))
		}
	}

	return pv.Save(outDir)
}

func PNPFrameCapture(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr

	// Start Perfetto.
	traceDataPath := filepath.Join(s.OutDir(), "trace.pb")
	recordCmd := testexec.CommandContext(
		ctx, traceBin, "record", "--output_file", traceDataPath)
	if err := recordCmd.Start(); err != nil {
		s.Fatal("Failed to run trace.py record: ", err)
	}
	defer func() {
		// GoBigSleepLint: Wait for CCA closing to trigger cros-camera device
		// closing functions.
		if err := testing.Sleep(cleanupCtx, 2*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
		recordCmd.Signal(unix.SIGINT)
		recordCmd.Wait()
		pv := perf.NewValues()
		if err := parseMetrics(cleanupCtx, pv, traceDataPath, s.OutDir()); err != nil {
			s.Fatal("Failed to parse metrics: ", err)
		}
	}()
	// GoBigSleepLint: Wait for trace.pb to start the process.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Start CCA.
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		s.Fatal("Failed to get remote output directory")
	}
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseRealCamera)
	if err != nil {
		s.Fatal("Failed to construct test bridge: ", err)
	}
	defer tb.TearDown(cleanupCtx)
	if err := cca.ClearSavedDir(ctx, cr); err != nil {
		s.Fatal("Failed to clear saved directory: ", err)
	}
	app, err := cca.New(ctx, cr, outDir, tb)
	if err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer app.Close(cleanupCtx)
	if err := app.FullscreenWindow(ctx); err != nil {
		s.Fatal("Failed to enter full screen of CCA: ", err)
	}

	s.Log("Start recording trace for ", traceTime)
	// GoBigSleepLint: Collect power metrics.
	if err := testing.Sleep(ctx, traceTime); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
}
