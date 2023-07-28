// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/camera/arcapp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power"
	powersetup "go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	targetFPS            = "30"
	iterationCount       = 30
	iterationDuration    = 2 * time.Second
	cameraWarmupDuration = 30 * time.Second
	prefixPreview        = "preview_"
	prefixRecording      = "recording_"

	cameraResetCount = 15
	// Snapshots can be really small if the room is dark, but JPEGs are never smaller than 100 bytes.
	minExpectedFileSize = 100
	snapshotCount       = 15
	snapshotWarmupCount = 5
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCCameraAppPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the performance when interacting with the ARC camera app",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com", "arcvm-eng@google.com"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Fixture:      "arcWithWorkingCameraForPerf",
		Data:         []string{arcapp.CameraAppApk},
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Timeout:      20 * time.Minute,
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App
	})
}

func ARCCameraAppPerf(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Install camera testing app.
	a := s.FixtValue().(*arc.PreData).ARC
	if err := a.Install(ctx, s.DataPath(arcapp.CameraAppApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	// Wait until CPU is cooled down.
	if _, err := cpu.WaitUntilCoolDown(ctx, cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)); err != nil {
		// If CPU fails to cool down, skip the performance test instead of failing it.
		s.Log("[Skip] CPU failed to cool down: ", err)
		return
	}

	// Start camera testing app.
	cleanupAppFunc, err := arcapp.LaunchARCCameraApp(ctx, a, tconn)
	if err != nil {
		s.Fatal("Failed to launch ARC camera app: ", err)
	}
	defer cleanupAppFunc(cleanupCtx, tconn)

	cleanupFunc, err := arcapp.PrepareSDCardAccess(ctx, cr)
	if err != nil {
		s.Fatal("Failed to prepare SD card access: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	p := perf.NewValues()

	subTestTimeout := 5 * time.Minute
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *chrome.Chrome, *chrome.TestConn, *arc.ARC, *perf.Values) error
	}{{
		"testOpenCloseSnapshot",
		testOpenCloseSnapshot,
	}, {
		"testPreview",
		testPreview,
	}, {
		"testRecording",
		testRecording,
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			if err := tst.testFunc(ctx, cr, tconn, a, p); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tst.name, err)
			}
		})
		cancel()
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}

func testOpenCloseSnapshot(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, a *arc.ARC, p *perf.Values) error {
	testing.ContextLog(ctx, "Starting measurement")
	if err := arcapp.ResetMetrics(ctx, a); err != nil {
		return errors.Wrap(err, "could not reset metrics")
	}

	// Measure camera open and close time.
	testing.ContextLog(ctx, "Measure camera open/close time")
	openCameraMetric := perf.Metric{Name: "open_camera_time", Unit: "ms", Direction: perf.SmallerIsBetter, Multiple: true}
	closeCameraMetric := perf.Metric{Name: "close_camera_time", Unit: "ms", Direction: perf.SmallerIsBetter, Multiple: true}
	for i := 0; i < cameraResetCount; i++ {
		testing.ContextLogf(ctx, "Iteration %d reset camera", i)

		if err := arcapp.ResetCamera(ctx, a); err != nil {
			return errors.Wrap(err, "could not reset the camera")
		}
	}

	// Measure taking a photo (snapshot)
	testing.ContextLogf(ctx, "Measure snapshot time: %d warmup rounds, %d measurements", snapshotWarmupCount, snapshotCount)
	snapshotMetric := perf.Metric{Name: "snapshot_time", Unit: "ms", Direction: perf.SmallerIsBetter, Multiple: true}

	for i := 0; i < snapshotWarmupCount+snapshotCount; i++ {
		testing.ContextLogf(ctx, "Iteration %d snapshot", i)

		if err := arcapp.TakePhoto(ctx, cr, a); err != nil {
			return errors.Wrap(err, "failed to take photo")
		}
	}

	metrics, err := arcapp.GetMetrics(ctx, a)
	if err != nil {
		return errors.Wrap(err, "failed to get metrics")
	}
	if len(metrics.OpeningCamera) < cameraResetCount || len(metrics.ClosingCamera) < cameraResetCount {
		return errors.Wrapf(err, "too few opening/closing camera metrics are collected. Opening: %v, Closing: %v", len(metrics.OpeningCamera), len(metrics.ClosingCamera))
	}
	for i := 0; i < cameraResetCount; i++ {
		p.Append(openCameraMetric, float64(metrics.OpeningCamera[i]))
		p.Append(closeCameraMetric, float64(metrics.ClosingCamera[i]))
	}

	if len(metrics.TakingPhoto) < snapshotWarmupCount+snapshotCount {
		return errors.Wrapf(err, "too few taking photo metrics are collected. Taking Photo: %v", len(metrics.TakingPhoto))
	}
	for i := snapshotWarmupCount; i < snapshotWarmupCount+snapshotCount; i++ {
		p.Append(snapshotMetric, float64(metrics.TakingPhoto[i]))
	}

	return nil
}

func testPreview(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, a *arc.ARC, p *perf.Values) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	powerSetupObj, powerCleanupFunc := powersetup.New("camera preview power and fps")
	defer func() {
		if err := powerCleanupFunc(cleanupCtx); err != nil {
			testing.ContextLog(ctx, "cleanup failed: ", err)
		}
	}()

	powerSetupObj.Add(powersetup.PowerTest(ctx, tconn,
		powersetup.PowerTestOptions{Wifi: powersetup.DisableWifiInterfaces, NightLight: powersetup.DisableNightLight},
		powersetup.NewBatteryDischarge(false /*discharge*/, true /*ignoreErr*/, powersetup.DefaultDischargeThreshold),
	))
	if err := powerSetupObj.Check(ctx); err != nil {
		return errors.Wrap(err, "power setup check failed")
	}

	testing.ContextLogf(ctx, "Set target FPS: %v FPS", targetFPS)
	if err := arcapp.SetFPS(ctx, a, targetFPS); err != nil {
		return errors.Wrap(err, "failed to set fps")
	}

	powerMetrics, err := perf.NewTimeline(ctx, power.TestMetrics(), perf.Prefix(prefixPreview), perf.Interval(iterationDuration))
	if err != nil {
		return errors.Wrap(err, "failed to build metrics")
	}

	if err := powerMetrics.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start metrics")
	}

	if err = arcapp.ResetCamera(ctx, a); err != nil {
		return errors.Wrap(err, "could not reset camera")
	}

	testing.ContextLog(ctx, "Warmup: Waiting a bit before starting the measurement")
	// GoBigSleepLint: It's used as a warm-up step which is part of the performance testing logic.
	if err := testing.Sleep(ctx, cameraWarmupDuration); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	testing.ContextLog(ctx, "Starting measurement")
	if err = arcapp.ResetMetrics(ctx, a); err != nil {
		return errors.Wrap(err, "could not reset metrics")
	}

	// Keep camera running and record power usage.
	if err := powerMetrics.StartRecording(ctx); err != nil {
		return errors.Wrap(err, "failed to start recording")
	}

	// Sleeps while measuring the power metrics.
	// GoBigSleepLint: It's used as a measurement step which is part of the performance testing logic.
	if err := testing.Sleep(ctx, iterationCount*iterationDuration); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	powerP, err := powerMetrics.StopRecording(ctx)
	if err != nil {
		return errors.Wrap(err, "error while recording power metrics")
	}
	p.Merge(powerP)

	frameDropRatio, err := arcapp.GetFrameDropRatio(ctx, a)
	if err != nil {
		return errors.Wrap(err, "failed to get frame drop ratio")
	}
	p.Set(perf.Metric{Name: prefixPreview + "frame_drop_ratio", Unit: "ratio", Direction: perf.SmallerIsBetter}, frameDropRatio)

	return nil
}

func testRecording(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, a *arc.ARC, p *perf.Values) error {
	// TODO(b/277865895): Remove skip logic once the grunt issue has been resolved.
	boardName, err := a.GetProp(ctx, "ro.product.name")
	if err != nil {
		return errors.Wrap(err, "failed to get the board name")
	}
	if strings.Contains(boardName, "grunt") {
		testing.ContextLog(ctx, "Skip recording test for grunt due to b/277865895")
		return nil
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	powerSetupObj, powerCleanupFunc := powersetup.New("camera recording power and fps")
	defer func() {
		if err := powerCleanupFunc(cleanupCtx); err != nil {
			testing.ContextLog(ctx, "cleanup failed: ", err)
		}
	}()

	powerSetupObj.Add(powersetup.PowerTest(ctx, tconn,
		powersetup.PowerTestOptions{Wifi: powersetup.DisableWifiInterfaces, NightLight: powersetup.DisableNightLight},
		powersetup.NewBatteryDischarge(false /*discharge*/, true /*ignoreErr*/, powersetup.DefaultDischargeThreshold),
	))
	if err := powerSetupObj.Check(ctx); err != nil {
		return errors.Wrap(err, "power setup check failed")
	}

	testing.ContextLogf(ctx, "Set target FPS: %v FPS", targetFPS)
	if err := arcapp.SetFPS(ctx, a, targetFPS); err != nil {
		return errors.Wrap(err, "failed to set fps")
	}

	powerMetrics, err := perf.NewTimeline(ctx, power.TestMetrics(), perf.Prefix(prefixRecording), perf.Interval(iterationDuration))
	if err != nil {
		return errors.Wrap(err, "failed to build metrics")
	}

	if err := powerMetrics.Start(ctx); err != nil {
		return errors.Wrap(err, "failed to start metrics")
	}

	if err := arcapp.StartRecording(ctx, cr, a); err != nil {
		return errors.Wrap(err, "failed to start recording")
	}

	testing.ContextLog(ctx, "Warmup: Waiting a bit before starting the measurement")
	// GoBigSleepLint: It's used as a warm-up step which is part of the performance testing logic.
	if err := testing.Sleep(ctx, cameraWarmupDuration); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	testing.ContextLog(ctx, "Starting measurement")
	if err = arcapp.ResetMetrics(ctx, a); err != nil {
		return errors.Wrap(err, "could not reset metrics")
	}

	// Keep camera running and record power usage.
	if err := powerMetrics.StartRecording(ctx); err != nil {
		return errors.Wrap(err, "failed to start recording")
	}

	// GoBigSleepLint: It's used as a measurement step which is part of the performance testing logic.
	if err := testing.Sleep(ctx, iterationCount*iterationDuration); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	powerP, err := powerMetrics.StopRecording(ctx)
	if err != nil {
		return errors.Wrap(err, "error while recording power metrics")
	}
	p.Merge(powerP)

	if err = arcapp.StopRecording(ctx, cr, a); err != nil {
		return errors.Wrap(err, "could not stop recording")
	}

	frameDropRatio, err := arcapp.GetFrameDropRatio(ctx, a)
	if err != nil {
		return errors.Wrap(err, "failed to get frame drop ratio")
	}
	p.Set(perf.Metric{Name: prefixRecording + "frame_drop_ratio", Unit: "ratio", Direction: perf.SmallerIsBetter}, frameDropRatio)

	return nil
}
