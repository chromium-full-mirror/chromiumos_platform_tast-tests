// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/audio"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerAudioPlaybackPerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Measures the battery drain during audio playback with different performance flags",
		Contacts: []string{
			"chromeos-audio-bugs@google.com", // Media team
			"judyhsiao@chromium.org",         // Author
			"cychiang@chromium.org",          // Media team
			"paulhsia@chromium.org",          // Media team
		},
		// ChromeOS > Platform > Virtualization > ARC++ & ARCVM > ARC Audio
		BugComponent: "b:879188",
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBootedWithDisableExternalStorage",
		Attr:         []string{"group:crosbolt", "crosbolt_nightly"},
		Params: []testing.Param{
			{
				Name: "default",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModeNone,
					BatteryDischargeMode: setup.ForceBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
			},
			{
				Name: "default_vm",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModeNone,
					BatteryDischargeMode: setup.ForceBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
			},
			{
				Name: "low_latency",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModeLowLatency,
					BatteryDischargeMode: setup.ForceBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
			},
			{
				Name: "low_latency_vm",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModeLowLatency,
					BatteryDischargeMode: setup.ForceBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
			},
			{
				Name: "power_saving",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModePowerSaving,
					BatteryDischargeMode: setup.ForceBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
			},
			{
				Name: "power_saving_vm",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModePowerSaving,
					BatteryDischargeMode: setup.ForceBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.ForceDischarge()),
			},
			{
				Name: "default_nobatterymetrics",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModeNone,
					BatteryDischargeMode: setup.NoBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
			},
			{
				Name: "default_vm_nobatterymetrics",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModeNone,
					BatteryDischargeMode: setup.NoBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
			},
			{
				Name: "low_latency_nobatterymetrics",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModeLowLatency,
					BatteryDischargeMode: setup.NoBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
			},
			{
				Name: "low_latency_vm_nobatterymetrics",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModeLowLatency,
					BatteryDischargeMode: setup.NoBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
			},
			{
				Name: "power_saving_nobatterymetrics",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModePowerSaving,
					BatteryDischargeMode: setup.NoBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
			},
			{
				Name: "power_saving_vm_nobatterymetrics",
				Val: audio.TestParameters{
					PerformanceMode:      audio.PerformanceModePowerSaving,
					BatteryDischargeMode: setup.NoBatteryDischarge,
				},
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(hwdep.NoForceDischarge()),
			},
		},
		Timeout: 10 * time.Minute,
	})
}

// PowerAudioPlaybackPerf measures the battery drain during audio playback with different performance flags.
func PowerAudioPlaybackPerf(ctx context.Context, s *testing.State) {
	const (
		testActivity           = "org.chromium.arc.testapp.arcaudiotest.PlaybackPerformanceActivity"
		audioWarmupDuration    = 10 * time.Second
		measureDuration        = 60 * time.Second
		keyPerformanceMode     = "perf_mode"
		keyDuration            = "duration"
		playbackDurationSecond = audioWarmupDuration + measureDuration + 10*time.Second // Add 10 seconds buffer.
	)

	param := s.Param().(audio.TestParameters)
	s.Logf("Measuing power consumption of audio playback with flag: %#x", param.PerformanceMode)

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

	sup, cleanup := setup.New(fmt.Sprintf("audio playback with flag: %#x.", param.PerformanceMode))
	defer func(ctx context.Context) {
		if err := cleanup(ctx); err != nil {
			s.Error("Cleanup failed: ", err)
		}
	}(cleanupCtx)

	discharge := param.BatteryDischargeMode == setup.ForceBatteryDischarge
	sup.Add(setup.PowerTest(ctx, tconn,
		setup.PowerTestOptions{Wifi: setup.DisableWifiInterfaces, NightLight: setup.DisableNightLight},
		setup.NewBatteryDischarge(discharge, true /*ignoreErr*/, setup.DefaultDischargeThreshold),
	))

	// Install testing app.
	a := s.FixtValue().(*arc.PreData).ARC
	sup.Add(setup.InstallApp(ctx, a, arc.APKPath(audio.Apk), audio.Pkg))

	// Wait until CPU is cooled down.
	if _, err := cpu.WaitUntilCoolDown(ctx, cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)); err != nil {
		s.Fatal("CPU failed to cool down: ", err)
	}

	powerMetrics, err := perf.NewTimeline(ctx, metrics.TestMetrics(), perf.Interval(measureDuration))
	if err != nil {
		s.Fatal("Failed to build metrics: ", err)
	}

	if err := powerMetrics.Start(ctx); err != nil {
		s.Fatal("Failed to start metrics: ", err)
	}

	// Start testing activity.
	// TODO(b/203214749): Maybe need to make another field of ActivityStartOptions that can support uint64 types and pass them as int extras
	sup.Add(
		setup.StartActivity(ctx, tconn, a, audio.Pkg, testActivity,
			arc.WithExtraIntUint64(keyPerformanceMode, uint64(param.PerformanceMode)),
			arc.WithExtraIntUint64(keyDuration, uint64(playbackDurationSecond/time.Second))),
	)
	if err := sup.Check(ctx); err != nil {
		s.Fatal("Setup failed: ", err)
	}

	s.Log("Warmup: Waiting a bit before starting the measurement")
	// GoBigSleepLint: It's used as a measurement step which is part of the performance testing logic.
	if err := testing.Sleep(ctx, audioWarmupDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Keep audio playback and record power usage.
	s.Log("Starting measurement")
	if err := powerMetrics.StartRecording(ctx); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	// GoBigSleepLint: It's used as a measurement step which is part of the performance testing logic.
	if err := testing.Sleep(ctx, measureDuration); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	p, err := powerMetrics.StopRecording(ctx)
	if err != nil {
		s.Fatal("Error while recording power metrics: ", err)
	}

	if err := p.Save(s.OutDir()); err != nil {
		s.Error("Failed saving perf data: ", err)
	}
}
