// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"math"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleAudioLatencyToolkit,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "An example of audio latency toolkit and latency measurements",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"crosep-intertech@google.com",
		},
		BugComponent:    "b:1280385",
		Attr:            []string{"group:audio_e2e_experimental", "audio_e2e_experimental_latency_toolkit"},
		SoftwareDeps:    []string{"chrome"},
		Fixture:         fixture.AudioLatencyToolkit,
		VariantCategory: `{"name": "Audio_SoC_Codec_Amp"}`,
	})
}

func getLatencyStats(latencies []int) (mean, sd float64, err error) {
	if len(latencies) == 0 {
		return 0, 0, errors.New("no latency available")
	}

	totalLatency := 0

	for _, latency := range latencies {
		totalLatency += latency
	}

	mean = float64(totalLatency) / float64(len(latencies))

	totalVariance := 0.0
	for _, latency := range latencies {
		totalVariance += math.Pow(float64(latency)-mean, 2)
	}

	sd = math.Sqrt(totalVariance / float64(len(latencies)))

	return mean, sd, nil
}

// ExampleAudioLatencyToolkit provides an example of using latency toolkit to
// measure cras latency.
func ExampleAudioLatencyToolkit(ctx context.Context, s *testing.State) {
	var measuredLatencies, reportedLatencies []int

	for i := 0; i < 5; i++ {
		measuredLatency, reportedLatency, err := audio.RunCrasLatencyTest(ctx)

		if err != nil {
			s.Fatal("Failed to get latency from latency toolkit: ", err)
		}

		s.Logf("Measured Cras Speaker Latency (us) %d", measuredLatency)
		s.Logf("Reported Cras Speaker Latency (us) %d", reportedLatency)

		measuredLatencies = append(measuredLatencies, measuredLatency)
		reportedLatencies = append(reportedLatencies, reportedLatency)

		// Prevent the previous measurement beep affect current measurement
		// GoBigSleepLint if beep happen too frequently we cannot correctly
		// measure the latency, as older beep maybe recorded as current beep
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Fail to sleep in between latency tests")
		}
	}

	meanMeasuredLatency, sdMeasuredLatency, err := getLatencyStats(measuredLatencies)
	if err != nil {
		s.Fatal("Failed to get statistics of measured latency: ", err)
	}
	meanReportedLatency, sdReportedLatency, err := getLatencyStats(reportedLatencies)
	if err != nil {
		s.Fatal("Failed to get statistics of reported latency: ", err)
	}

	s.Logf("Mean Measured Cras Speaker Latency (us) %f", meanMeasuredLatency)
	s.Logf("Standard Deviation Measured Cras Speaker Latency (us) %f", sdMeasuredLatency)
	s.Logf("Mean Reported Cras Speaker Latency (us) %f", meanReportedLatency)
	s.Logf("Standard Deviation Reported Cras Speaker Latency (us) %f", sdReportedLatency)

}
