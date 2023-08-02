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

type traceMetrics struct {
	CameraCoreMetrics struct {
		Sessions []struct {
			Sid                 int   `json:"sid"`
			OpenDeviceLatencyUs int64 `json:"open_device_latency_us"`
			InitializeLatencyUs int64 `json:"initialize_latency_us"`
			StreamMetrics       []struct {
				E2EConfigureStreamsLatencyUs int64 `json:"e2e_configure_streams_latency_us"`
				HalConfigureStreamsLatencyUs int64 `json:"hal_configure_streams_latency_us"`
				MinE2ERequestLatencyUs       int64 `json:"min_e2e_request_latency_us"`
				AvgE2ERequestLatencyUs       int64 `json:"avg_e2e_request_latency_us"`
				MaxE2ERequestLatencyUs       int64 `json:"max_e2e_request_latency_us"`
				MinHalRequestLatencyUs       int64 `json:"min_hal_request_latency_us"`
				AvgHalRequestLatencyUs       int64 `json:"avg_hal_request_latency_us"`
				MaxHalRequestLatencyUs       int64 `json:"max_hal_request_latency_us"`
				ResultBufferMetrics          []struct {
					Stream struct {
						StreamID int64 `json:"stream_id"`
						Width    int   `json:"width"`
						Height   int   `json:"height"`
						Format   int   `json:"format"`
					} `json:"stream"`
					MinE2ELatencyUs int64 `json:"min_e2e_latency_us"`
					AvgE2ELatencyUs int64 `json:"avg_e2e_latency_us"`
					MaxE2ELatencyUs int64 `json:"max_e2e_latency_us"`
				} `json:"result_buffer_metrics"`
			} `json:"stream_metrics"`
			CloseDeviceLatencyUs int64 `json:"close_device_latency_us"`
		} `json:"sessions"`
	} `json:"camera_core_metrics"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPFrameCapture,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect latency information of frame capturing functions",
		BugComponent: "b:167281",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{caps.BuiltinCamera, "chrome", "camera_app"},
		Fixture:      pnp.StablePowerLacrosGAIA,
		Timeout:      1*time.Minute + traceTime,
	})
}

func setMetric(pv *perf.Values, name, unit string, value float64) {
	pv.Set(perf.Metric{
		Name:      name,
		Unit:      unit,
		Direction: perf.SmallerIsBetter,
	}, value)
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
	session := metrics.CameraCoreMetrics.Sessions[len(metrics.CameraCoreMetrics.Sessions)-1]
	setMetric(pv, "openDeviceLatencyUs", "microsec", float64(session.OpenDeviceLatencyUs))
	setMetric(pv, "initializeLatencyUs", "microsec", float64(session.InitializeLatencyUs))
	setMetric(pv, "closeDeviceLatencyUs", "microsec", float64(session.CloseDeviceLatencyUs))

	// If there are multiple configurations, only use the last subsession to
	// exclude preparation time for redoing ConfigureStream().
	stream := session.StreamMetrics[len(session.StreamMetrics)-1]
	setMetric(pv, "e2eConfigureStreamsLatencyUs", "microsec", float64(stream.E2EConfigureStreamsLatencyUs))
	setMetric(pv, "halConfigureStreamsLatencyUs", "microsec", float64(stream.HalConfigureStreamsLatencyUs))
	setMetric(pv, "minE2ERequestLatencyUs", "microsec", float64(stream.MinE2ERequestLatencyUs))
	setMetric(pv, "e2eConfigureStreamsLatencyUs", "microsec", float64(stream.E2EConfigureStreamsLatencyUs))
	setMetric(pv, "avgE2ERequestLatencyUs", "microsec", float64(stream.AvgE2ERequestLatencyUs))
	setMetric(pv, "maxE2ERequestLatencyUs", "microsec", float64(stream.MaxE2ERequestLatencyUs))
	setMetric(pv, "minHalRequestLatencyUs", "microsec", float64(stream.MinHalRequestLatencyUs))
	setMetric(pv, "avgHalRequestLatencyUs", "microsec", float64(stream.AvgHalRequestLatencyUs))
	setMetric(pv, "maxHalRequestLatencyUs", "microsec", float64(stream.MaxHalRequestLatencyUs))

	for i, resultBuffer := range stream.ResultBufferMetrics {
		setMetric(pv, fmt.Sprintf("resultBuffer_%d_streamID", i), "id", float64(resultBuffer.Stream.StreamID))
		setMetric(pv, fmt.Sprintf("resultBuffer_%d_width", i), "pix", float64(resultBuffer.Stream.Width))
		setMetric(pv, fmt.Sprintf("resultBuffer_%d_height", i), "pix", float64(resultBuffer.Stream.Height))
		setMetric(pv, fmt.Sprintf("resultBuffer_%d_format", i), "category", float64(resultBuffer.Stream.Format))
		setMetric(pv, fmt.Sprintf("resultBuffer_%d_minE2ELatencyUs", i), "microsec", float64(resultBuffer.MinE2ELatencyUs))
		setMetric(pv, fmt.Sprintf("resultBuffer_%d_avgE2ELatencyUs", i), "microsec", float64(resultBuffer.AvgE2ELatencyUs))
		setMetric(pv, fmt.Sprintf("resultBuffer_%d_maxE2ELatencyUs", i), "microsec", float64(resultBuffer.MaxE2ELatencyUs))
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
