// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/audio/wav"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasSidetone,
		Desc:         "Check sidetone functionality is working",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "normanbt@google.com"},
		BugComponent: "b:776546",
		Attr: []string{
			"group:mainline",
			"informational",
		},
		Fixture: fixture.AloopLoaded{
			Channels:    2,
			DevicePairs: 2,
			Parent:      fixture.UIStopped{}.Instance(),
		}.Instance(),
		Timeout:      3 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// CrasSidetone checks sidetone functionality. The audio path is as follows
//
//	                               aloop capture 0                      aloop capture 1
//	sine.wav \                   / (aloop)         \ (cras sidetone)  / (aloop)         \ capture.wav
//	           aloop playback 0                      aloop playback 1
func CrasSidetone(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, chrome.ResetTimeout)
	defer cancel()

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Failed to connect to CRAS: ", err)
	}

	if err := cras.SetActiveNodeByMatcher(ctx, audio.MatchNodeName{
		Name: "Loopback Playback 1",
	}); err != nil {
		s.Fatal("Failed to SetActiveNodeByMatcher: ", err)
	}

	if err := cras.SetActiveNodeByMatcher(ctx, audio.MatchNodeName{
		Name: "Loopback Capture",
	}); err != nil {
		s.Fatal("Failed to SetActiveNodeByMatcher: ", err)
	}

	if err := cras.SetSidetoneEnabled(ctx, true); err != nil {
		s.Fatal("Failed to SetSidetoneEnabled: ", err)
	}
	defer func() {
		cras.SetSidetoneEnabled(ctx, false)
	}()

	const wavDuration = 10 * time.Second

	playbackWavPath := filepath.Join(s.OutDir(), "sine.wav")
	wavData := audio.TestRawData{
		Path:          playbackWavPath,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
		Frequencies:   []int{440, 440},
		Volume:        0.8,
		Duration:      int(wavDuration.Seconds()),
	}
	if err := audio.GenerateTestWavData(ctx, wavData); err != nil {
		s.Fatal("Failed to generate sine wav file: ", err)
	}

	playbackCaptureCtx, cancel := context.WithTimeout(ctx, 2*wavDuration)
	defer cancel()

	playbackDone := make(chan struct{})
	go func() {
		defer close(playbackDone)
		if err := audio.PlayWavToPCM(playbackCaptureCtx, playbackWavPath, "hw:Loopback,0"); err != nil {
			s.Error("Cannot run playback: ", err)
		}
		s.Log("Playback complete")
	}()

	captureWavPath := filepath.Join(s.OutDir(), "capture.wav")
	if err := audio.CaptureWavFromPCM(ctx, captureWavPath, "hw:Loopback_1,1", int(wavDuration.Seconds())); err != nil {
		s.Fatal("Cannot run capture: ", err)
	}
	s.Log("Capture complete")

	var captureWav wav.File
	if captureWav, err = wav.ReadPCMFile(ctx, captureWavPath); err != nil {
		s.Fatalf("Cannot read %s, err: %v", captureWavPath, err)
	}
	audioData := captureWav.GetBodyAsInt16()
	for ch := 0; ch < 2; ch++ {
		if err := audio.CheckFrequency(ctx, audioData[ch], 48000, 440, 10, 3); err != nil {
			s.Error("CheckFrequency err: ", err)
		}
	}

	s.Log("Waiting for playback to complete")
	<-playbackDone
}
