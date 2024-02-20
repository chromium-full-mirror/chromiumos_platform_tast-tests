// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	perfpkg "go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/device"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/internal"
	"go.chromium.org/tast-tests/cros/local/procutil"
	"go.chromium.org/tast-tests/cros/local/profiler"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// TestParameters contains all the data needed to run a single test iteration.
type testParameters struct {
	Playback       bool
	Capture        bool
	CaptureEffects string
}

const (
	effectsNone    = ""
	effectsCrasAec = "0x01"
	effectsDspAec  = "0x11"
)

var unstableModelsPlaybackCapture = []string{
	// TODO(b/261363619): Undo skip after fix.
	"beetley",
	// TODO(b/261361770): Undo skip after fix.
	"akali", "akali360", "bard", "pantheon", "sona", "syndra", "vayne",
	// TODO(b/198322358): Undo skip after fix.
	"ekko", "nautilus", "nautiluslte", "soraka", "eve",
	// TODO(b/312097873): remove "brya" when b/309904720 is fixed.
	"brya",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasPerf,
		Desc:         "Performance measurement of CRAS",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "yuhsuan@chromium.org", "cychiang@chromium.org", "paulhsia@chromium.org"},
		BugComponent: "b:776546",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		Fixture:      "rebootForAudioDSPFixture",
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name: "playback",
				Val: testParameters{
					Playback:       true,
					Capture:        false,
					CaptureEffects: effectsNone,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.SkipOnModel(unstableModelsPlaybackCapture...)),
			},
			{
				Name: "capture",
				Val: testParameters{
					Playback:       false,
					Capture:        true,
					CaptureEffects: effectsNone,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.SkipOnModel(unstableModelsPlaybackCapture...)),
			},
			{
				Name: "playback_capture",
				Val: testParameters{
					Playback:       true,
					Capture:        true,
					CaptureEffects: effectsNone,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Speaker(), hwdep.SkipOnModel(unstableModelsPlaybackCapture...)),
			},
			{
				Name: "playback_capture_cras_aec",
				Val: testParameters{
					Playback:       true,
					Capture:        true,
					CaptureEffects: effectsCrasAec,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Speaker(), hwdep.SkipOnModel(unstableModelsPlaybackCapture...)),
			},
			{
				Name: "playback_capture_dsp_aec",
				Val: testParameters{
					Playback:       true,
					Capture:        true,
					CaptureEffects: effectsDspAec,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.Speaker(), hwdep.SkipOnModel(unstableModelsPlaybackCapture...), hwdep.Model(internal.DSPAECModels...)),
			},
		},
	})
}

func crasPerfOneIteration(ctx context.Context, s *testing.State, pid int, pv *perfpkg.Values) {
	const (
		getDeviceTimeout = 3 * time.Second
		blocksize        = 480
		topInterval      = 1 * time.Second
		perfDuration     = 10 * time.Second              // Duration to run perf command.
		commandDuration  = perfDuration + 2*time.Second  // Duration to run audio command
		contextDuration  = perfDuration + 15*time.Second // Upper bound for one iteration.
	)

	param := s.Param().(testParameters)

	runCtx, cancel := context.WithTimeout(ctx, contextDuration)
	defer cancel()

	var out profiler.PerfStatCyclesPerSecondOutput
	var outSched profiler.PerfSchedOutput
	var outStat profiler.ProcStatOutput

	profs := []profiler.Profiler{
		profiler.Top(&profiler.TopOpts{
			Interval: topInterval,
		}),
		profiler.Perf(profiler.PerfStatCyclesPerSecondOpts(&out, pid)),
		profiler.Perf(profiler.PerfRecordOpts("", nil, profiler.PerfRecordCallgraph)),
		profiler.Perf(profiler.PerfSchedOpts(&outSched, "cras")),
		profiler.ProcStat(&outStat, pid),
	}

	s.Log("start audio")
	playbackCommand := crastestclient.PlaybackCommand(runCtx, int(commandDuration.Seconds()), blocksize)
	captureCommand := crastestclient.CaptureCommand(runCtx, int(commandDuration.Seconds()), blocksize)

	if param.CaptureEffects != effectsNone {
		captureCommand.Args = append(captureCommand.Args, "--effects="+param.CaptureEffects)
	}

	if param.Capture {
		captureCommand.Start()
	}

	if param.Capture && param.Playback {
		// GoBigSleepLint: Wait one second to simulate WebRTC creating an output
		//                 stream about 1 seconds after creating an input stream.
		if err := testing.Sleep(ctx, 1*time.Second); err != nil {
			s.Fatal("Timed out on sleep: ", err)
		}
	}

	if param.Playback {
		playbackCommand.Start()
	}

	// GoBigSleepLint: Wait one second for audio processing stream to be ready.
	//                 TODO(b/165995912) remove the sleep once we can query
	//                 stream state from CRAS.
	if err := testing.Sleep(ctx, 1*time.Second); err != nil {
		s.Fatal("Timed out on sleep: ", err)
	}

	runningProfs, err := profiler.Start(ctx, s.OutDir(), profs...)
	if err != nil {
		s.Fatal("Failure in starting the profiler: ", err)
	}

	checkRunningDevice := func(ctx context.Context) error {
		return device.CheckRunningDevice(ctx, bool(param.Playback), bool(param.Capture))
	}
	if err := testing.Poll(ctx, checkRunningDevice, &testing.PollOptions{Timeout: getDeviceTimeout}); err != nil {
		s.Fatal("Failed to detect running device: ", err)
	}

	defer func() {
		// The perf value is stored when ending the profiler.
		if err := runningProfs.End(ctx); err != nil {
			s.Error("Failure in ending the profiler: ", err)
		} else {
			// Append measurements to PerfValue.
			pv.Append(perfpkg.Metric{
				Name:      "cras_cycles_per_second",
				Unit:      "cycles",
				Direction: perfpkg.SmallerIsBetter,
				Multiple:  true,
			}, out.CyclesPerSecond[0].Value)
			// The 2nd element holds the value for E-Cores.
			if len(out.CyclesPerSecond) > 1 {
				pv.Append(perfpkg.Metric{
					Name:      "cras_cycles_per_second_ecore",
					Unit:      "cycles",
					Direction: perfpkg.SmallerIsBetter,
					Multiple:  true,
				}, out.CyclesPerSecond[1].Value)
			}
			pv.Append(perfpkg.Metric{
				Name:      "cras_max_latency_ms",
				Unit:      "milliseconds",
				Direction: perfpkg.SmallerIsBetter,
				Multiple:  true,
			}, outSched.MaxLatencyMs)
			s.Log("CPU utilization: ", outStat.CPUUtilization())
			pv.Append(perfpkg.Metric{
				Name:      "cras_cpu_utilization",
				Unit:      "cpus",
				Direction: perfpkg.SmallerIsBetter,
				Multiple:  true,
			}, outStat.CPUUtilization())

		}

		if param.Playback {
			if err := playbackCommand.Wait(); err != nil {
				s.Error("Playback did not finish in time: ", err)
			}
		}

		if param.Capture {
			if err := captureCommand.Wait(); err != nil {
				s.Error("Capture did not finish in time: ", err)
			}
		}

		s.Log("Finished one iteration")
	}()

	// GoBigSleepLint: Record for perfDuration seconds.
	//                 This is to make sure that audio is being used during whole
	//                 perf recording.
	if err := testing.Sleep(ctx, perfDuration); err != nil {
		s.Fatal("Timed out on sleep: ", err)
	}

}

// CrasPerf measures the performance of CRAS.
func CrasPerf(ctx context.Context, s *testing.State) {
	const (
		crasPath   = "/usr/bin/cras"
		iterations = 10
	)

	// TODO(b/194820340): aarch64 is disabled while perf HW counters don't
	// all work reliably.
	if u, err := sysutil.Uname(); err != nil {
		s.Fatal("Failed to get uname: ", err)
	} else if u.Machine == "aarch64" {
		s.Log("Can not run perf on aarch64 machine")
		return
	}

	// Use this perf value to hold CPU cycles per second spent in CRAS of each iteration.
	pv := perfpkg.NewValues()

	for i := 0; i < iterations; i++ {
		s.Log("Iteration: ", i)

		// Stop CRAS to make sure the audio device won't be occupied.
		if _, err := audio.RestartCras(ctx); err != nil {
			s.Fatal("Failed to restart CRAS: ", err)
		}

		// Any device being available means CRAS is ready.
		if err := audio.WaitForDevice(ctx, audio.OutputStream|audio.InputStream); err != nil {
			s.Fatal("Failed to wait for any output or input device: ", err)
		}

		proc, err := procutil.FindUnique(procutil.ByExe(crasPath))
		if err != nil {
			s.Fatal("Failed to find PID of cras: ", err)
		}

		s.Log("Get PID done: ", proc.Pid)
		crasPerfOneIteration(ctx, s, int(proc.Pid), pv)
	}

	if err := pv.Save(s.OutDir()); err != nil {
		s.Fatal("Cannot save perf data: ", err)
	}
}
