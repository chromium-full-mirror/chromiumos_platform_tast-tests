// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"fmt"
	"math"
	"math/cmplx"
	"path/filepath"
	"strconv"
	"time"

	"gonum.org/v1/gonum/dsp/fourier"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type echoRefDeviceVal struct {
	frequency int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         EchoRefDevice,
		Desc:         "The manual test script for smart amp AVL. It verifies that the echo reference device records the speaker playback correctly",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "judyhsiao@chromium.org", "yuhsuan@chromium.org"},
		BugComponent: "b:776546",
		Attr:         []string{"group:audio_audiobox"},
		HardwareDeps: hwdep.D(hwdep.Speaker(), hwdep.SmartAmp()),
		Timeout:      20 * time.Minute,
		Params: []testing.Param{
			{
				Name: "frequency_200",
				Val: echoRefDeviceVal{
					frequency: 200,
				},
			},
			{
				Name: "frequency_400",
				Val: echoRefDeviceVal{
					frequency: 400,
				},
			},
			{
				Name: "frequency_1000",
				Val: echoRefDeviceVal{
					frequency: 1000,
				},
			},
			{
				Name: "frequency_2000",
				Val: echoRefDeviceVal{
					frequency: 2000,
				},
			},
		},
	})
}

func EchoRefDevice(ctx context.Context, s *testing.State) {
	const (
		duration               = 1 // second
		frequencyDiffThreshold = 5 //Hz
	)

	// Use a shorter context to save time for cleanup.
	ctxCleanup := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Stop UI in advance for this test to avoid the node being selected by UI.
	if err := upstart.StopJob(ctx, "ui"); err != nil {
		s.Fatal("Failed to stop ui: ", err)
	}
	defer upstart.EnsureJobRunning(ctxCleanup, "ui")

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}

	if err := cras.SetActiveNodeByType(ctx, "ECHO_REFERENCE"); err != nil {
		s.Fatal("Failed to set echo reference mic active: ", err)
	}

	if err := cras.SetActiveNodeByType(ctx, "INTERNAL_SPEAKER"); err != nil {
		s.Fatal("Failed to set internal speaker active: ", err)
	}

	speaker, err := cras.GetNodeByType(ctx, "INTERNAL_SPEAKER")
	if err != nil {
		s.Fatal("Failed to get internal speaker node: ", err)
	}

	if err := cras.SetOutputNodeVolume(ctx, *speaker, 80); err != nil {
		s.Fatal("Failed to set speaker volume: ", err)
	}

	param := s.Param().(echoRefDeviceVal)

	if err := crastestclient.WaitForNoStream(ctx, 20*time.Second); err != nil {
		s.Fatal(err, "failed to wait for no stream")
	}

	s.Logf("Testing freqency: %d Hz", param.frequency)
	// Playback function by CRAS.
	playback := testexec.CommandContext(
		ctx, "sox",
		"-b", "16",
		"-r", "48000",
		"-c", "2",
		"-n",
		"-t",
		"alsa",
		"default",
		"synth", strconv.Itoa(duration*5),
		"sine", strconv.Itoa(param.frequency))
	playback.Start()

	if _, err := crastestclient.WaitForStreams(ctx, 2*time.Second); err != nil {
		s.Fatal(err, "failed to playback within timeout")
	}

	// GoBigSleepLint: Start the recording after playback has been started for 1 second.
	testing.Sleep(ctx, 1*time.Second)

	filename := fmt.Sprintf("echo_ref_device_%d.raw", param.frequency)
	recording := audio.TestRawData{
		Path:          filepath.Join(s.OutDir(), filename),
		BitsPerSample: 16,
		Channels:      1,
		Rate:          48000,
		Duration:      duration,
	}

	testing.ContextLog(ctx, "Capture output to ", recording.Path)
	if err := crastestclient.CaptureFileCommand(
		ctx, recording.Path,
		recording.Duration,
		recording.Channels,
		recording.Rate).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal(err, "failed to capture")
	}

	tone, err := audio.ReadS16LEPCM(recording.Path, recording.Channels)
	if err != nil {
		s.Fatal(err, "failed to read recording from file")
	}

	// Convert the PCM data to float64.
	size := len(tone[0])
	samples := make([]float64, size)
	for i, x := range tone[0] {
		samples[i] = float64(x)
	}

	fft := fourier.NewFFT(size)
	// Initialize an FFT and perform the analysis.
	coeff := fft.Coefficients(nil, samples)

	var maxFreq, magnitude float64
	for i, c := range coeff {
		m := cmplx.Abs(c)
		if m > magnitude {
			magnitude = m
			maxFreq = fft.Freq(i) * float64(size)
		}
	}

	if math.Abs(maxFreq-float64(param.frequency)) > frequencyDiffThreshold {
		s.Fatalf("Expected freq=%.2f Hz, got: %.2f Hz", float64(param.frequency), maxFreq)
	}
}
