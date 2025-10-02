// Copyright 2018 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/camera/getusermedia"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/tracing"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type metricsPath struct {
	trace     string
	query     string
	outputDir string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         GetUserMediaPerf,
		Desc:         "Captures performance data about getUserMedia video capture",
		Contacts:     []string{"chromeos-camera-app-eng@google.com", "shik@chromium.org", "seannli@google.com"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		VariantCategory:   `{"name": "Camera:Model"}`,
		SoftwareDeps: []string{caps.BuiltinOrVividCamera, "chrome"},
		Fixture:      "chromeCameraPerf",
		Data: append(
			getusermedia.DataFiles(),
			"web_api.html",
			"perfetto/camera_config.pbtxt",
			"perfetto/camera_query.sql"),
	})
}

func collectMetrics(ctx context.Context, pv *perf.Values, sess *tracing.Session, paths metricsPath) error {
	if err := sess.Stop(ctx); err != nil {
		return errors.Wrap(err, "failed to stop session")
	}

	// Collect important metrics and upload to CrosBolt.
	metrics, err := sess.RunQuery(ctx, paths.query)
	if err != nil {
		return errors.Wrap(err, "failed to process the trace data")
	}

	names := metrics[0]
	if names[0] != "open_device" || names[1] != "configure_streams" {
		return errors.Wrap(err, "unexpected query column names")
	}

	values := metrics[1]
	if len(names) != len(values) {
		return errors.Wrap(err, "mismatched amount of columns")
	}

	for i := 0; i < len(names); i++ {
		value, err := strconv.ParseFloat(values[i], 64)
		if err != nil {
			return errors.Wrapf(err, "value is not float64: %v", values[i])
		}

		pv.Set(perf.Metric{
			Name:      names[i],
			Unit:      "nanosecond",
			Direction: perf.SmallerIsBetter,
		}, value)
	}
	return pv.Save(paths.outputDir)
}

// GetUserMediaPerf is the full version of GetUserMedia. It renders the camera's
// media stream in VGA and 720p for 20 seconds. If there is no error while
// exercising the camera, it uploads statistics of black/frozen frames. This
// test will fail when an error occurs or too many frames are broken.
//
// This test uses the real webcam unless it is running under QEMU. Under QEMU,
// it uses "vivid" instead, which is the virtual video test driver and can be
// used as an external USB camera.
func GetUserMediaPerf(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Ensure camera service running to avoid bad state from previous tests.
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

	traceConfigPath := s.DataPath("perfetto/camera_config.pbtxt")
	traceDataPath := filepath.Join(s.OutDir(), "trace.pb")
	sess, err := tracing.StartSession(ctx, traceConfigPath, tracing.WithTraceDataPath(traceDataPath))
	if err != nil {
		s.Fatal("Failed to start tracing: ", err)
	}
	defer sess.Finalize(cleanupCtx)

	p := perf.NewValues()
	paths := metricsPath{
		trace:     traceDataPath,
		query:     s.DataPath("perfetto/camera_query.sql"),
		outputDir: s.OutDir()}
	defer func(cleanupCtx context.Context) {
		if s.HasError() {
			return
		}
		if err := collectMetrics(cleanupCtx, p, sess, paths); err != nil {
			s.Fatal("Failed to collect metrics: ", err)
		}
	}(cleanupCtx)

	s.Log("Collecting Perfetto trace File at: ", traceDataPath)

	ci := s.FixtValue().(chrome.HasChrome).Chrome()

	// Run tests for 20 seconds per resolution.
	results, err := getusermedia.RunGetUserMedia(ctx, s.DataFileSystem(), ci, 20*time.Second, nil, getusermedia.NoVerboseLogging)
	if err != nil {
		s.Fatal("Failed to call getUserMedia(): ", err)
	}
	// Set and upload frame statistics below.
	results.SetPerf(p)
}
