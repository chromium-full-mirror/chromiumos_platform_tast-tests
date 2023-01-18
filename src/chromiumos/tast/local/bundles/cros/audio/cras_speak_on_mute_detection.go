// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"path/filepath"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/testexec"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/audio"
	"chromiumos/tast/local/audio/crastestclient"
	"chromiumos/tast/local/bundles/cros/audio/internal"
	"chromiumos/tast/local/dbusutil"
	"chromiumos/tast/testing"
)

const (
	theQuickBrownFoxWav = "the-quick-brown-fox.wav"

	// The ALSA device to playback directly to simulate speech from the user's mouth.
	aloopPlaybackPCM = "hw:Loopback,0"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasSpeakOnMuteDetection,
		Desc:         "Test CRAS detection of speaking while on mute",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:875484", // ChromeOS > Platform > Technologies > Audio > Test > Tast
		Attr:         []string{"group:mainline", "informational"},
		Fixture:      fixture.StereoAloopLoadedWithoutUI,
		Data:         []string{theQuickBrownFoxWav},
		Timeout:      3 * time.Minute,
	})
}

func CrasSpeakOnMuteDetection(ctx context.Context, s *testing.State) {
	cleanupCtx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		if s.HasError() {
			if err := crastestclient.DumpAudioDiagnostics(ctx, s.OutDir()); err != nil {
				s.Error("Failed to dump audio diagnostics: ", err)
			}
		}
	}(cleanupCtx)

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}
	if err := cras.SetInputMute(ctx, true); err != nil {
		s.Fatal("Cannot mute input: ", err)
	}
	if err := cras.SetSpeakOnMuteDetection(ctx, true); err != nil {
		s.Fatal("Cannot enable speak-on-mute detection: ", err)
	}
	if err := internal.SelectIODevices(ctx, cras, "ALSA_LOOPBACK", "ALSA_LOOPBACK"); err != nil {
		s.Fatal("Cannot select IO devices: ", err)
	}

	signalWatcher, err := dbusutil.NewSignalWatcherForSystemBus(ctx, dbusutil.MatchSpec{
		Type:      "signal",
		Interface: "org.chromium.cras.Control",
		Member:    "SpeakOnMuteDetected",
	})
	if err != nil {
		s.Fatal("Cannot set up a D-Bus signal watcher for speak-on-mute")
	}
	defer signalWatcher.Close(cleanupCtx)

	detected := false
	go func() {
		for range signalWatcher.Signals {
			s.Log("Detected speech")
			detected = true
		}
	}()

	speechWav := filepath.Join(s.OutDir(), "speech.wav")
	if err := testexec.CommandContext(ctx, "sox", s.DataPath(theQuickBrownFoxWav), "--channels=2", "--rate=48000", speechWav, "repeat", "1").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Cannot prepare speech.wav with sox: ", err)
	}

	playbackDone := make(chan struct{})
	go func() {
		defer close(playbackDone)
		s.Log("Playing speech directly to ", aloopPlaybackPCM)
		if err := testexec.CommandContext(ctx, "aplay", "-D"+aloopPlaybackPCM, speechWav).Run(testexec.DumpLogOnError); err != nil {
			s.Fatal("Cannot run aplay: ", err)
		}
	}()

	delayCapture := time.Second
	s.Logf("Delay capture for %v to let playback start first", delayCapture)
	if err := testing.Sleep(ctx, delayCapture); err != nil {
		s.Fatal("Cannot sleep: ", err)
	}

	s.Log("Capturing from default device")
	captureRaw := filepath.Join(s.OutDir(), "capture.raw")
	captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	if err := testexec.CommandContext(captureCtx, "cras_test_client", "-C", captureRaw, "--rate=48000", "-c", "2", "--block_size=480", "--duration=5").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Cannot capture with cras_test_client: ", err)
	}
	captureWav := filepath.Join(s.OutDir(), "capture.wav")
	if err := audio.ConvertRawToWav(ctx, captureRaw, captureWav, 48000, 2); err != nil {
		s.Errorf("Cannot convert %s to %s: %v", captureRaw, captureWav, err)
	}

	if rms, err := audio.GetRmsAmplitude(ctx, audio.TestRawData{
		Path:          captureRaw,
		BitsPerSample: 16,
		Channels:      2,
		Rate:          48000,
	}); err != nil {
		s.Error("Cannot get RMS from capture.raw")
	} else {
		if rms != 0 {
			s.Error("Captured audio is not muted; rms: ", rms)
		}
	}

	s.Log("Waiting for playback to complete")
	<-playbackDone

	if !detected {
		s.Fatal("Did not detect speech")
	}
}
