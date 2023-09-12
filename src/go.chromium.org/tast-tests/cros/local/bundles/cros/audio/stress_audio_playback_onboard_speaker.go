// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type audioStressTestParams struct {
	stressDuration time.Duration
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         StressAudioPlaybackOnboardSpeaker,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies audio playback over onboard speaker for long duration",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:776546",
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Speaker()),
		Fixture:      "chromeLoggedIn",
		Params: []testing.Param{{
			Name: "bronze",
			Val: audioStressTestParams{
				stressDuration: 6 * time.Hour,
			},
			ExtraAttr: []string{"group:intel-reliability-bronze"},
			Timeout:   6*time.Hour + 10*time.Minute,
		}, {
			Name: "silver",
			Val: audioStressTestParams{
				stressDuration: 9 * time.Hour,
			},
			ExtraAttr: []string{"group:intel-reliability-silver"},
			Timeout:   9*time.Hour + 10*time.Minute,
		}, {
			Name: "gold",
			Val: audioStressTestParams{
				stressDuration: 12 * time.Hour,
			},
			ExtraAttr: []string{"group:intel-reliability-gold"},
			Timeout:   12*time.Hour + 10*time.Minute,
		}},
	})
}

// StressAudioPlaybackOnboardSpeaker plays audio file over onboard speaker for long duration.
func StressAudioPlaybackOnboardSpeaker(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	cr := s.FixtValue().(*chrome.Chrome)
	testOpt := s.Param().(audioStressTestParams)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	defer func(ctx context.Context) {
		if err := closeAudioPlayer(ctx, kb); err != nil {
			s.Error("Failed to close audio player at cleanup: ", err)
		}
	}(cleanupCtx)

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to create Cras object: ", err)
	}

	const expectedAudioNode = "INTERNAL_SPEAKER"

	// Get current audio output device info.
	deviceName, deviceType, err := cras.SelectedOutputDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get the selected audio device: ", err)
	}

	if deviceType != expectedAudioNode {
		if err := cras.SetActiveNodeByType(ctx, expectedAudioNode); err != nil {
			s.Fatalf("Failed to select active device %s: %v", expectedAudioNode, err)
		}
		deviceName, deviceType, err = cras.SelectedOutputDevice(ctx)
		if err != nil {
			s.Fatal("Failed to get the selected audio device: ", err)
		}
		if deviceType != expectedAudioNode {
			s.Fatalf("Failed to set the audio node type: got %q; want %q; error: %v", deviceType, expectedAudioNode, err)
		}
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve users Downloads path: ", err)
	}

	// Generate sine raw input file that lasts 1 hour.
	const rawFileName = "AudioFile.raw"
	rawFilePath := filepath.Join(downloadsPath, rawFileName)
	rawFile := audio.TestRawData{
		Path:          rawFilePath,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Frequencies:   []int{440, 440},
		Volume:        0.05,
		Duration:      3600,
	}

	if err := audio.GenerateTestRawData(ctx, rawFile); err != nil {
		s.Fatal("Failed to generate audio test data: ", err)
	}
	defer os.Remove(rawFile.Path)

	const wavFileName = "AudioFile.wav"
	wavFile := filepath.Join(downloadsPath, wavFileName)
	if err := audio.ConvertRawToWav(ctx, rawFile.Path, wavFile, 48000, 2); err != nil {
		s.Fatal("Failed to convert raw to wav: ", err)
	}
	defer os.Remove(wavFile)

	// Converting total stress test duration as hour iteration value.
	iterHour := int(testOpt.stressDuration / time.Hour)
	for i := 0; i < iterHour; i++ {
		files, err := filesapp.Launch(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to launch the Files App: ", err)
		}

		if err := files.OpenDownloads()(ctx); err != nil {
			s.Fatal("Failed to open Downloads folder in files app: ", err)
		}
		if err := files.OpenFile(wavFileName)(ctx); err != nil {
			s.Fatalf("Failed to open the audio file %q: %v", wavFileName, err)
		}

		// Total duration is taken as hour intervals.
		// Generating an audio file of 1 hour duration and checking whether audio is
		// routing through internal-speaker with 2 minutes of sleep till completion of each hour.
		s.Logf("Checking audio routing, test remaining time of %d/%d hour", i+1, iterHour)
		endTime := time.Now().Add(time.Hour)
		for {
			timeNow := time.Now()
			if timeNow.After(endTime) {
				break
			}
			devName, err := crastestclient.FirstRunningDevice(ctx, audio.OutputStream)
			if err != nil {
				s.Fatal("Failed to detect running output device")
			}
			if deviceName != devName {
				s.Fatalf("Failed: unexpected audio node: got %q; want %q", devName, deviceName)
			}
			// Sleep for 2 minutes if remainingTime is greater or equal to 2 minutes.
			// Otherwise sleep for remainingTime.
			sleepingDuration := endTime.Sub(time.Now())
			testing.ContextLog(ctx, "Checking audio routing, test remaining time: ", sleepingDuration)
			if sleepingDuration >= 2*time.Minute {
				sleepingDuration = 2 * time.Minute
			}

			// GoBigSleepLint: Sleep for 2 minutes or remaining time.
			if err := testing.Sleep(ctx, sleepingDuration); err != nil {
				s.Fatal("Failed to sleep: ", err)
			}
		}

		// Close audio player window.
		if err := closeAudioPlayer(ctx, kb); err != nil {
			s.Fatal("Failed to close audio player: ", err)
		}

		files.Close(ctx)
	}
}

// closeAudioPlayer performs closing of audio player window.
func closeAudioPlayer(ctx context.Context, kb *input.KeyboardEventWriter) error {
	if err := kb.Accel(ctx, "Ctrl+W"); err != nil {
		return errors.Wrap(err, "failed to press 'Ctrl+W' to close window")
	}
	return nil
}
