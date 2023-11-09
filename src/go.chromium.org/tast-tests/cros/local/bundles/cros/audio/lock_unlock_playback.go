// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"math"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LockUnlockPlayback,
		Desc:         "Check playback works in lock and unlock state",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "normanbt@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline", "informational",
		},
		Fixture:      fixture.AloopLoaded{Channels: 2, Parent: "chromeLoggedIn"}.Instance(),
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// LockUnlockPlayback checks playback still works after lock and unlock.
func LockUnlockPlayback(ctx context.Context, s *testing.State) {
	const segmentLength = 5 * time.Second

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}
	if err := audio.SetupLoopback(ctx, cr); err != nil {
		s.Fatal("Failed to setup loopback device: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	wavFileName := "440hz_60s.wav"
	wavFilePath := filepath.Join(downloadsPath, wavFileName)
	wavData := audio.TestRawData{
		Path:          wavFilePath,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Frequencies:   []int{440, 440},
		Volume:        0.8,
		Duration:      60,
	}
	if err := audio.GenerateTestWavData(ctx, wavData); err != nil {
		s.Fatal("Failed to generate sine wav file: ", err)
	}
	if err := testexec.CommandContext(ctx, "cp", wavFilePath, s.OutDir()).Run(testexec.DumpLogOnError); err != nil {
		s.Fatalf("Failed to copy %s to %s, err: %v", wavFilePath, s.OutDir(), err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open keyboard")
	}

	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files app: ", err)
	}
	defer files.Close(ctx)

	if err := files.OpenDownloads()(ctx); err != nil {
		s.Fatal("Failed to open Downloads folder: ", err)
	}

	if err := files.OpenFile(wavFileName)(ctx); err != nil {
		s.Fatal("Failed to open the wav file: ", err)
	}

	captureData := audio.TestRawData{
		Path:          filepath.Join(s.OutDir(), "capture.wav"),
		BitsPerSample: 16,
		Rate:          48000,
		Channels:      2,
		Duration:      40,
	}
	captureDone := make(chan struct{})
	defer func() {
		s.Log("Waiting for capture to complete")
		<-captureDone
		s.Log("Capture complete")
	}()

	var captureTimestamp time.Time
	var finishTime time.Duration
	go func() {
		captureTimestamp = time.Now()
		s.Log("Start capture: ", time.Now())
		if err := audio.CaptureWavFromDefault(ctx, captureData); err != nil {
			s.Error("Cannot capture: ", err)
		}
		close(captureDone)
	}()

	// GoBigSleepLint: Make sure the test capture enough audio samples before lock
	// And add some time to make sure the segment fully contains audio.
	testing.Sleep(ctx, segmentLength+2*time.Second)
	lockTime := time.Now().Sub(captureTimestamp)
	if err := lockscreen.Lock(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}
	// GoBigSleepLint: Make sure the test capture enough audio samples after lock
	testing.Sleep(ctx, segmentLength)

	s.Log("Decreasing volume")
	_, err = quicksettings.DecreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeVolume)
	if err != nil {
		s.Error("Failed to change volume: ", err)
	}

	s.Log("Ready to unlock")
	// GoBigSleepLint: Make sure the test capture enough audio samples before unlock
	testing.Sleep(ctx, segmentLength)
	err = lockscreen.UnlockWithPassword(ctx, tconn, cr.Creds().User, cr.Creds().Pass, kb, 30*time.Second, 30*time.Second)
	unlockTime := time.Now().Sub(captureTimestamp)
	// GoBigSleepLint: Make sure the test capture enough audio samples after unlock
	testing.Sleep(ctx, segmentLength)
	finishTime = time.Now().Sub(captureTimestamp)

	s.Log("lock time: ", lockTime.Seconds())
	s.Log("unlock time: ", unlockTime.Seconds())
	s.Log("finish time: ", finishTime.Seconds())

	//
	// playback capture         lock        volume down     unlock        stop capture stop playback
	//    |         |   | a:<5s> | b:<5s> |     |    | c:<5s> | d:<5s> |      |            |
	// ---------------------------------------------------------------------------------------------
	//
	// We only care with segment a, b, c and d, which are 5 seconds before and after lock,
	// and 5 seconds before and after unlock
	trimmedFiles := []*struct {
		path      string
		startTime time.Duration
		amplitude float64
	}{
		{
			path:      filepath.Join(s.OutDir(), "beforeLock.wav"),
			startTime: lockTime - segmentLength,
		},
		{
			path:      filepath.Join(s.OutDir(), "afterLock.wav"),
			startTime: lockTime,
		},
		{
			path:      filepath.Join(s.OutDir(), "beforeUnlock.wav"),
			startTime: unlockTime - segmentLength,
		},
		{
			path:      filepath.Join(s.OutDir(), "afterUnlock.wav"),
			startTime: unlockTime,
		},
	}

	for _, file := range trimmedFiles {
		if err := audio.TrimFileFromAndTo(
			ctx, captureData.Path, file.path, file.startTime, segmentLength,
		); err != nil {
			s.Fatalf("Failed to trim file %s, err: %s", filepath.Base(file.path), err)
		}

		file.amplitude, err = audio.GetRmsAmplitudeFromWav(ctx, file.path)
		if err != nil {
			s.Fatalf("Failed to get rms amplitude for %s, err: %s", filepath.Base(file.path), err)
		}
	}

	similarityTolerance := 1e-5
	if math.Abs(trimmedFiles[0].amplitude-trimmedFiles[1].amplitude) >= similarityTolerance {
		s.Errorf("RMS Amplitude of %s and %s differ, got %v and %v",
			trimmedFiles[0].path, trimmedFiles[1].path,
			trimmedFiles[0].amplitude, trimmedFiles[1].amplitude)
	}
	if math.Abs(trimmedFiles[2].amplitude-trimmedFiles[3].amplitude) >= similarityTolerance {
		s.Errorf("RMS Amplitude of %s and %s differ, got %v and %v",
			trimmedFiles[2].path, trimmedFiles[3].path,
			trimmedFiles[2].amplitude, trimmedFiles[3].amplitude)
	}

	minimumDifference := 1e-3
	if trimmedFiles[1].amplitude-trimmedFiles[2].amplitude < minimumDifference {
		s.Errorf("RMS Amplitude of %s and %s should be different, got %v and %v",
			trimmedFiles[1].path, trimmedFiles[2].path,
			trimmedFiles[1].amplitude, trimmedFiles[2].amplitude)
	}
}
