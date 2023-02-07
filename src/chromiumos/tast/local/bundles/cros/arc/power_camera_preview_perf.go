// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"chromiumos/tast/common/media/caps"
	"chromiumos/tast/common/perf"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/camera/arcapp"
	"chromiumos/tast/local/cpu"
	"chromiumos/tast/local/power"
	"chromiumos/tast/local/power/setup"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

type powerCameraPreviewPerfArgs struct {
	targetFPS     string
	dischargeMode setup.BatteryDischargeMode
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerCameraPreviewPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the battery drain and camera statistics (e.g., dropped frames) during camera preview at 30/60 FPS",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org", "arcvm-eng@google.com"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Fixture:      "arcBootedWithDisableSyncFlags",
		Data:         []string{arcapp.CameraAppApk},
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Params: []testing.Param{
			{
				Name:              "30fps",
				ExtraSoftwareDeps: []string{"android_p"},
				ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
				Val: powerCameraPreviewPerfArgs{
					targetFPS:     "30",
					dischargeMode: setup.ForceBatteryDischarge,
				},
			},
			{
				Name:              "30fps_vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
				Val: powerCameraPreviewPerfArgs{
					targetFPS:     "30",
					dischargeMode: setup.ForceBatteryDischarge,
				},
			},
			{
				Name:              "30fps_nobatterymetrics",
				ExtraSoftwareDeps: []string{"android_p"},
				ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
				Val: powerCameraPreviewPerfArgs{
					targetFPS:     "30",
					dischargeMode: setup.NoBatteryDischarge,
				},
			},
			{
				Name:              "30fps_vm_nobatterymetrics",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
				Val: powerCameraPreviewPerfArgs{
					targetFPS:     "30",
					dischargeMode: setup.NoBatteryDischarge,
				},
			},
		},
		Timeout:      10 * time.Minute,
		BugComponent: "b:978428",
	})
}

func PowerCameraPreviewPerf(ctx context.Context, s *testing.State) {
	// Give cleanup actions a minute to run, even if we fail by exceeding our
	// deadline.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	cr := s.FixtValue().(*arc.PreData).Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	sup, cleanup := setup.New("camera preview power and fps")

	defer func() {
		if err := cleanup(cleanupCtx); err != nil {
			s.Error("Cleanup failed: ", err)
		}
	}()

	args := s.Param().(powerCameraPreviewPerfArgs)
	sup.Add(setup.PowerTest(ctx, tconn,
		setup.PowerTestOptions{Wifi: setup.DisableWifiInterfaces, NightLight: setup.DisableNightLight},
		setup.NewBatteryDischargeFromMode(args.dischargeMode),
	))

	// Install camera testing app.
	a := s.FixtValue().(*arc.PreData).ARC
	if err := a.Install(ctx, s.DataPath(arcapp.CameraAppApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	// Wait until CPU is cooled down.
	if _, err := cpu.WaitUntilCoolDown(ctx, cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)); err != nil {
		s.Error("CPU failed to cool down: ", err)
	}

	// Start camera testing app.
	cleanupAppFunc, err := arcapp.LaunchARCCameraApp(ctx, a, tconn)
	if err != nil {
		s.Fatal("Failed to launch ARC camera app: ", err)
	}
	defer cleanupAppFunc(cleanupCtx, tconn)

	if err := sup.Check(ctx); err != nil {
		s.Fatal("Setup failed: ", err)
	}

	const (
		// TODO(springerm): Make iteration count an optional command line parameter.
		iterationCount            = 30
		iterationDuration         = 2 * time.Second
		afterBootWarmupDuration   = 30 * time.Second
		cameraResetWarmupDuration = 30 * time.Second
	)

	s.Log("Warmup: Waiting for Android to settle down")
	if err := testing.Sleep(ctx, afterBootWarmupDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	s.Logf("Set target FPS: %v FPS", args.targetFPS)
	if err = arcapp.SetFPS(ctx, a, args.targetFPS); err != nil {
		s.Fatal("Failed to set fps: ", err)
	}

	// Create metrics. We report separately for each target FPS.
	frameDropRatioMetric := perf.Metric{Name: "frame_drop_ratio", Unit: "ratio", Direction: perf.SmallerIsBetter}

	powerMetrics, err := perf.NewTimeline(ctx, power.TestMetrics(), perf.Interval(iterationDuration))
	if err != nil {
		s.Fatal("Failed to build metrics: ", err)
	}

	if err := powerMetrics.Start(ctx); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}

	s.Log("Warmup: Waiting a bit before starting the measurement")
	if err := testing.Sleep(ctx, cameraResetWarmupDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	s.Log("Starting measurement")

	if err = arcapp.ResetCamera(ctx, a); err != nil {
		s.Fatal("Could not reset camera: ", err)
	}

	// Keep camera running and record power usage.
	if err := powerMetrics.StartRecording(ctx); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	if err := testing.Sleep(ctx, iterationCount*iterationDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	p, err := powerMetrics.StopRecording(ctx)
	if err != nil {
		s.Fatal("Error while recording power metrics: ", err)
	}

	frameDropRatio, err := arcapp.GetFrameDropRatio(ctx, a)
	if err != nil {
		s.Fatal("Failed to get frame drop ratio: ", err)
	}
	p.Set(frameDropRatioMetric, frameDropRatio)

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
