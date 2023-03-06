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
	"chromiumos/tast/local/power/setup"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CameraPerfExtraMetrics,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures extra camera metrics such as open/close time and snapshot time",
		Contacts:     []string{"chromeos-camera-eng@google.com", "wtlee@chromium.org", "arcvm-eng@google.com"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Fixture:      "arcBootedRestricted",
		Data:         []string{arcapp.CameraAppApk},
		Params: []testing.Param{{
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_nightly"},
			ExtraSoftwareDeps: []string{"android_container"},
			ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
			Val:               setup.ForceBatteryDischarge,
		}, {
			Name:              "vm",
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_nightly"},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
			Val:               setup.ForceBatteryDischarge,
		}, {
			Name:              "nobatterymetrics",
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_nightly"},
			ExtraSoftwareDeps: []string{"android_p"},
			ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
			Val:               setup.NoBatteryDischarge,
		}, {
			Name:              "vm_nobatterymetrics",
			ExtraAttr:         []string{"group:crosbolt", "crosbolt_nightly"},
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
			Val:               setup.NoBatteryDischarge,
		}},
		Timeout:      10 * time.Minute,
		BugComponent: "b:978428",
	})
}

func CameraPerfExtraMetrics(ctx context.Context, s *testing.State) {
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

	sup, cleanup := setup.New("camera perf extra metrics")

	defer func() {
		if err := cleanup(cleanupCtx); err != nil {
			s.Error("Cleanup failed: ", err)
		}
	}()

	batteryMode := s.Param().(setup.BatteryDischargeMode)
	sup.Add(setup.PowerTest(ctx, tconn,
		setup.PowerTestOptions{Wifi: setup.DisableWifiInterfaces, NightLight: setup.DisableNightLight},
		setup.NewBatteryDischargeFromMode(batteryMode),
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

	p := perf.NewValues()

	const (
		afterBootWarmupDuration = 30 * time.Second
		cameraResetCount        = 15
		// Snapshots can be really small if the room is dark, but JPEGs are never smaller than 100 bytes.
		minExpectedFileSize = 100
		snapshotCount       = 15
		snapshotWarmupCount = 5
	)

	s.Log("Warmup: Waiting for Android to settle down")
	if err := testing.Sleep(ctx, afterBootWarmupDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Measure camera open and close time.
	s.Log("Measure camera open/close time")
	openCameraMetric := perf.Metric{Name: "open_camera_time", Unit: "ms", Direction: perf.SmallerIsBetter, Multiple: true}
	closeCameraMetric := perf.Metric{Name: "close_camera_time", Unit: "ms", Direction: perf.SmallerIsBetter, Multiple: true}
	for i := 0; i < cameraResetCount; i++ {
		s.Logf("Iteration %d reset camera", i)

		if err := arcapp.ResetCamera(ctx, a); err != nil {
			s.Fatal("Could not reset the camera: ", err)
		}
	}

	// Prepare host-side access to Android's SDCard partition, which should store the generated photo file.
	cleanupFunc, err := arc.MountSDCardPartitionOnHostWithSSHFSIfVirtioBlkDataEnabled(ctx, a, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to make Android's SDCard partition available on host: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	// Measure taking a photo (snapshot)
	s.Logf("Measure snapshot time: %d warmup rounds, %d measurements", snapshotWarmupCount, snapshotCount)
	snapshotMetric := perf.Metric{Name: "snapshot_time", Unit: "ms", Direction: perf.SmallerIsBetter, Multiple: true}

	for i := 0; i < snapshotCount+snapshotWarmupCount; i++ {
		s.Logf("Iteration %d snapshot", i)

		if err := arcapp.TakePhoto(ctx, cr, a); err != nil {
			s.Error("Failed to take photo: ", err)
		}
	}

	metrics, err := arcapp.GetMetrics(ctx, a)
	if err != nil {
		s.Error("Failed to get metrics: ", err)
	}
	if len(metrics.OpeningCamera) < cameraResetCount || len(metrics.ClosingCamera) < cameraResetCount {
		s.Errorf("Too few opening/closing camera metrics are collected. Opening: %v, Closing: %v", len(metrics.OpeningCamera), len(metrics.ClosingCamera))
	} else {
		for i := 0; i < cameraResetCount; i++ {
			p.Append(openCameraMetric, float64(metrics.OpeningCamera[i]))
			p.Append(closeCameraMetric, float64(metrics.ClosingCamera[i]))
		}
	}

	if len(metrics.TakingPhoto) < snapshotCount+snapshotWarmupCount {
		s.Error("Too few taking photo metrics are collected. Taking Photo: ", len(metrics.TakingPhoto))
	} else {
		for i := snapshotWarmupCount; i < snapshotCount+snapshotWarmupCount; i++ {
			p.Append(snapshotMetric, float64(metrics.TakingPhoto[i]))
		}
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
