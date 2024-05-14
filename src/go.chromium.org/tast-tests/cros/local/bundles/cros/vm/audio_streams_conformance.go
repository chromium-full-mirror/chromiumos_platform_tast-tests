// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vm

import (
	"context"
	"encoding/json"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio/audionode"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type streamSource string

const (
	noop streamSource = "noop"
	cras              = "cras"
)

// audioStreamTestParameters contains all the data needed to run a single test iteration.
type audioStreamTestParameters struct {
	StreamSource               streamSource // The StreamSource to test.
	RateCriteria               float64
	RateErrCriteria            float64
	ColdStartLatencyCriteriaMs int
}

type duration struct {
	/// Seconds.
	Secs int `json:"secs"`
	/// Nano seconds.
	Nanos float64 `json:"nanos"`
}

type estimatedRate struct {
	/// linear coefficients of LINEST(frames,timestamps).
	Rate float64 `json:"rate"`
	/// STEYX(frames, timestamps).
	Error float64 `json:"error"`
}

type audioStreamsConformanceReport struct {
	ColdStartLatency duration      `json:"cold_start_latency"`
	RecordCount      int           `json:"record_count"`
	EstimatedRate    estimatedRate `json:"rate"`
	/// How many times that consumed frames are different from buffer_frames.
	MismatchedFrameCount int `json:"mismatched_frame_count"`
}

// Relax the ColdStartLatencyCriteriaMs for the following devices to prevent test flakiness.
var relaxedCriteriaModels = []string{
	// TODO(b/269065601)
	"atlas", "nocturne",

	// b/269065601#comment8: Devices with RT1015p amp have an additional 300ms delay on the power on sequence, so we relax the criteria for them.
	"beetley", "blipper", "galith360", "galnat", "galnat360", "galtic", "galtic360", "sasukette", "storo", "storo360",
}
var skippedModels = []string{
	// TODO(b/312097873): remove "brya" when b/309904720 is fixed.
	"brya",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         AudioStreamsConformance,
		Desc:         "Test AudioStream implementation correctness",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "judyhsiao@chromium.org", "paulhsia@google.com"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		HardwareDeps: hwdep.D(hwdep.SkipOnModel(skippedModels...)),
		SoftwareDeps: []string{"vm_host"},
		Fixture:      "uiStopped",
		BugComponent: "b:1332660",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild", "group:mainline", "informational"},
		Timeout:      4 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "noop",
				ExtraAttr: []string{"group:sw_gates_virt", "sw_gates_virt_enabled"},
				Val: audioStreamTestParameters{
					StreamSource:               noop,
					RateCriteria:               0.0005,
					RateErrCriteria:            10,
					ColdStartLatencyCriteriaMs: 5,
				},
			},
			{
				Name:              "cras",
				ExtraHardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.SkipOnModel(relaxedCriteriaModels...)),
				ExtraAttr:         []string{"group:sw_gates_virt", "sw_gates_virt_enabled"},
				Val: audioStreamTestParameters{
					StreamSource:               cras,
					RateCriteria:               0.001,
					RateErrCriteria:            10,
					ColdStartLatencyCriteriaMs: 500,
				},
			},
			{
				// Compare to cras subtest, it prolongs ColdStartLatencyCriteriaMs from 500 to 700 ms.
				Name:              "cras_relaxed",
				ExtraHardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.Model(relaxedCriteriaModels...)),
				Val: audioStreamTestParameters{
					StreamSource:               cras,
					RateCriteria:               0.001,
					RateErrCriteria:            10,
					ColdStartLatencyCriteriaMs: 700,
				},
			},
		},
	})
}

const (
	cleanupTime                    = 60 * time.Second
	audioStreamsConformanceTimeout = 20 * time.Second
	sampleRate                     = 48000.0
)

// AudioStreamsConformance is used as one of the standard SW gates (go/pe-sw-gates).
// Please ask to crosvm-core@ if you want to modify or delete this test.
func AudioStreamsConformance(ctx context.Context, s *testing.State) {
	param := s.Param().(audioStreamTestParameters)

	// Run audio_streams_conformance_test.
	runCtx, cancel := context.WithTimeout(ctx, audioStreamsConformanceTimeout)
	defer cancel()

	if param.StreamSource != noop {
		const expectedAudioNode = "INTERNAL_SPEAKER"
		_, err := audionode.SetAudioNode(ctx, expectedAudioNode)
		if err != nil {
			s.Fatal("Failed to set the Audio node: ", err)
		}
	}

	dump, err := testexec.CommandContext(
		runCtx, "/usr/local/bin/audio_streams_conformance_test",
		"-P", string(param.StreamSource),
		"-b", "480",
		"--iterations", "400",
		"--json",
	).Output(testexec.DumpLogOnError)

	if err != nil {
		s.Fatal("Failed to run audio_streams_conformance_test: ", err)
	}

	var stats audioStreamsConformanceReport
	err = json.Unmarshal(dump, &stats)
	if err != nil {
		s.Error("Failed to parse result file: ", err)
	}

	if stats.EstimatedRate.Rate < sampleRate*(1-param.RateCriteria) || stats.EstimatedRate.Rate > sampleRate*(1+param.RateCriteria) {
		s.Fatalf("Expect sample rate: %f (+- %.2f%%), got: %f", sampleRate, param.RateCriteria*100, stats.EstimatedRate.Rate)
	}

	if stats.EstimatedRate.Error > sampleRate*param.RateErrCriteria {
		s.Fatalf("Expect rate error: < %f, got: %f", sampleRate*param.RateErrCriteria, stats.EstimatedRate.Error)
	}

	if stats.MismatchedFrameCount > 0 {
		s.Fatalf("Expect MismatchedFrameCount: 0, got: %d", stats.MismatchedFrameCount)
	}

	coldStartLatencyMS := stats.ColdStartLatency.Secs*100 + int(stats.ColdStartLatency.Nanos/1000000)

	if param.StreamSource == cras {
		p := perf.NewValues()
		minLatency := perf.Metric{
			Name:      "coldStartLatency",
			Unit:      "ms",
			Direction: perf.SmallerIsBetter}
		p.Set(minLatency, float64(coldStartLatencyMS))
		if err := p.Save(s.OutDir()); err != nil {
			s.Error("Failed saving perf data: ", err)
		}
	}

	if coldStartLatencyMS > param.ColdStartLatencyCriteriaMs {
		s.Fatalf("Expect ColdStartLatency less than: %d ms, got: %d ms", param.ColdStartLatencyCriteriaMs, coldStartLatencyMS)
	}
}
