// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pnp provides fixture for stable power and performance evaluation.
package pnp

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	traceScript            = "/usr/local/share/camera/tracing/bin/trace.py"
	traceQueryJSONFileName = "trace_query.json"
)

type traceMetrics struct {
	PerfettoProtosCameraCoreMetrics struct {
		SessionMetrics []struct {
			Sid             int              `json:"sid"`
			FunctionMetrics []FunctionMetric `json:"function_metrics"`
			ConfigMetrics   []struct {
				FunctionMetrics     []FunctionMetric `json:"function_metrics"`
				ResultBufferMetrics []struct {
					Stream struct {
						StreamID int64 `json:"stream_id"`
						Width    int   `json:"width"`
						Height   int   `json:"height"`
						Format   int   `json:"format"`
					} `json:"stream"`
					FunctionMetrics []FunctionMetric `json:"function_metrics"`
				} `json:"result_buffer_metrics"`
			} `json:"config_metrics"`
		} `json:"session_metrics"`
	} `json:"perfetto.protos.camera_core_metrics"`
}

// FrameCaptureRecorder uses perfetto to collect traces.
type FrameCaptureRecorder struct {
	recordCmd     *testexec.Cmd
	traceDataPath string
}

// StartRecording starts tracing.
func (f *FrameCaptureRecorder) StartRecording(ctx context.Context, traceDataPath string) error {
	f.traceDataPath = traceDataPath
	f.recordCmd = testexec.CommandContext(
		ctx, traceScript, "record", "--output_file", traceDataPath)
	if err := f.recordCmd.Start(); err != nil {
		return errors.Wrapf(err, "failed to run %s record --output_file %s", traceScript, traceDataPath)
	}

	// GoBigSleepLint: Wait for trace.pb to start the process.
	if err := testing.Sleep(ctx, 2*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}

// StopRecording ends tracing and parses the trace.
func (f *FrameCaptureRecorder) StopRecording(cleanupCtx context.Context) error {
	if err := f.recordCmd.Signal(unix.SIGINT); err != nil {
		return errors.Wrapf(err, "faled to send SIGINT to the process executing : %s", traceScript)
	}
	if err := f.recordCmd.Wait(); err != nil {
		return errors.Wrapf(err, "faled to wait for the process executing %s", traceScript)
	}
	return nil
}

// CollectMetrics parses the trace and save it.
func (f *FrameCaptureRecorder) CollectMetrics(cleanupCtx context.Context, outDir string) error {
	pv := perf.NewValues()
	traceQueryJSONPath := filepath.Join(outDir, traceQueryJSONFileName)
	reportCmd := testexec.CommandContext(
		cleanupCtx, traceScript, "report", "-i", f.traceDataPath, "--metrics",
		"camera_core_metrics", "--metrics_output", "json", "--output_file",
		traceQueryJSONPath)
	_, err := reportCmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "failed to run %s report -i %s --metrics camera_core_metrics --metrics_output json --output_file %s", traceScript, f.traceDataPath, traceQueryJSONPath)

	}

	traceQueryByte, err := os.ReadFile(traceQueryJSONPath)
	if err != nil {
		return errors.Wrapf(err, "failed to read %s", traceQueryJSONPath)
	}

	metrics := traceMetrics{}
	err = json.Unmarshal(traceQueryByte, &metrics)
	if err != nil {
		return errors.Wrap(err, "failed to convert JSON to metrics")
	}

	// If there are multiple camera sessions, only use the last session to
	// exclude preparation time for redoing OpenDevice().
	if len(metrics.PerfettoProtosCameraCoreMetrics.SessionMetrics) == 0 {
		return errors.Wrap(err, "failed to find any session")
	}
	session := metrics.PerfettoProtosCameraCoreMetrics.SessionMetrics[len(metrics.PerfettoProtosCameraCoreMetrics.SessionMetrics)-1]
	for _, functionMetric := range session.FunctionMetrics {
		SetMetricFromFunction(pv, functionMetric, "")
	}

	// If there are multiple configurations, only use the last subsession to
	// exclude preparation time for redoing ConfigureStream().
	if len(session.ConfigMetrics) == 0 {
		return errors.Wrap(err, "failed to find any stream in the last session")
	}
	stream := session.ConfigMetrics[len(session.ConfigMetrics)-1]
	for _, functionMetric := range stream.FunctionMetrics {
		SetMetricFromFunction(pv, functionMetric, "")
	}

	for i, resultBuffer := range stream.ResultBufferMetrics {
		SetMetric(pv, fmt.Sprintf("ResultBuffer_%d_streamID", i), "id", float64(resultBuffer.Stream.StreamID), false)
		SetMetric(pv, fmt.Sprintf("ResultBuffer_%d_width", i), "pixel", float64(resultBuffer.Stream.Width), false)
		SetMetric(pv, fmt.Sprintf("ResultBuffer_%d_height", i), "pixel", float64(resultBuffer.Stream.Height), false)
		SetMetric(pv, fmt.Sprintf("ResultBuffer_%d_format", i), "category", float64(resultBuffer.Stream.Format), false)
		for _, functionMetric := range resultBuffer.FunctionMetrics {
			SetMetricFromFunction(pv, functionMetric, fmt.Sprintf("ResultBuffer_%d_", i))
		}
	}
	return pv.Save(outDir)
}
