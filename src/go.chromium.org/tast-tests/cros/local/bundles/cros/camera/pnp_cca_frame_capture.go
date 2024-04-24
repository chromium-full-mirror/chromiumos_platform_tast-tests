// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/camera/pnp"
	"go.chromium.org/tast-tests/cros/local/camera/testutil"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	traceTimePNPCCAFrameCapture = 20 * time.Second
	initTimePNPCCAFrameCapture  = 1 * time.Minute
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PNPCCAFrameCapture,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Collect latency information of frame capturing functions during a CCA session",
		Contacts:     []string{"chromeos-camera-eng@google.com", "esker@chromium.org"},
		BugComponent: "b:167281", // ChromeOS > Platform > Technologies > Camera
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{caps.BuiltinCamera, "chrome", "camera_app"},
		Fixture:      pnp.StablePowerLacros,
		Timeout:      initTimePNPCCAFrameCapture + traceTimePNPCCAFrameCapture + power.RecorderTimeout,
	})
}

func PNPCCAFrameCapture(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if err := pnp.Cooldown(ctx); err != nil {
		s.Fatal("Failed to run pnp cooldown routine: ", err)
	}

	testing.ContextLog(ctx, "[Start Work Phase]")
	cr := s.FixtValue().(powersetup.PowerUIFixtureData).Cr

	// Start Perfetto.
	frameCaptureRecorder := pnp.FrameCaptureRecorder{}
	if err := frameCaptureRecorder.StartRecording(ctx, filepath.Join(s.OutDir(), "trace.pb")); err != nil {
		s.Fatal("Failed start recording: ", err)
	}
	defer func() {
		// GoBigSleepLint: Cros-camera device closing function adheres to the Android
		// Camera3 API, which mandates a 500 ms response time.
		if err := testing.Sleep(cleanupCtx, 500*time.Millisecond); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}

		if err := frameCaptureRecorder.StopRecording(cleanupCtx); err != nil {
			s.Fatal("Failed stop recording: ", err)
		}

		if s.HasError() {
			return
		}
		if err := frameCaptureRecorder.CollectMetrics(cleanupCtx, s.OutDir()); err != nil {
			s.Fatal("Failed collect metrics: ", err)
		}
	}()

	// Start CCA.
	tb, err := testutil.NewTestBridge(ctx, cr, testutil.UseRealCamera)
	if err != nil {
		s.Fatal("Failed to construct test bridge: ", err)
	}
	defer tb.TearDown(cleanupCtx)
	if err := cca.ClearSavedDir(ctx, cr); err != nil {
		s.Fatal("Failed to clear saved directory: ", err)
	}
	app, err := cca.New(ctx, cr, s.OutDir(), tb)
	if err != nil {
		s.Fatal("Failed to open CCA: ", err)
	}
	defer app.Close(cleanupCtx)
	if err := app.FullscreenWindow(ctx); err != nil {
		s.Fatal("Failed to enter full screen of CCA: ", err)
	}

	testing.ContextLog(ctx, "[Record Phase] Start recording trace for ", traceTimePNPCCAFrameCapture)
	// GoBigSleepLint: Collect performance metrics.
	if err := testing.Sleep(ctx, traceTimePNPCCAFrameCapture); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}
}
