// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/device"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/internal"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasSpeakOnMuteDetection,
		Desc:         "Test CRAS detection of speaking while on mute",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546", // ChromeOS > Platform > Technologies > Audio > Test > Tast
		Attr: []string{
			"group:mainline",
			"group:video_conference", "video_conference_per_build",
		},
		Fixture: fixture.AloopLoaded{
			Channels: 2,
			Parent:   fixture.UIStopped{}.Instance(),
		}.Instance(),
		Data:    []string{data.TheQuickBrownFoxWav},
		Timeout: 3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "rtc_stream_with_apm", // testplan#1
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: true,
					inputMuted:     true,
					clientStream: clientStreamConfig{
						rtc: true,
						apm: true,
					},
					speechSource:      mouth,
					expectedDetection: true,
				},
			},
			{
				Name: "rtc_stream_without_apm", // testplan#2
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: true,
					inputMuted:     true,
					clientStream: clientStreamConfig{
						rtc: true,
						apm: false,
					},
					speechSource:      mouth,
					expectedDetection: true,
				},
			},
			{
				Name: "non_rtc_stream_with_apm", // testplan#3
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: true,
					inputMuted:     true,
					clientStream: clientStreamConfig{
						rtc: false,
						apm: true,
					},
					speechSource:      mouth,
					expectedDetection: false,
				},
			},
			{
				Name: "non_rtc_stream_without_apm", // testplan#4
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: true,
					inputMuted:     true,
					clientStream: clientStreamConfig{
						rtc: false,
						apm: false,
					},
					speechSource:      mouth,
					expectedDetection: false,
				},
			},
			{
				Name: "rtc_stream_with_apm_at_speaker", // testplan#7
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: true,
					inputMuted:     true,
					clientStream: clientStreamConfig{
						rtc: true,
						apm: true,
					},
					speechSource:      speaker,
					expectedDetection: false,
				},
				ExtraAttr: []string{"informational"},
			},
			{
				Name: "rtc_stream_without_apm_at_speaker", // testplan#8
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: true,
					inputMuted:     true,
					clientStream: clientStreamConfig{
						rtc: true,
						apm: false,
					},
					speechSource:      speaker,
					expectedDetection: false,
				},
				ExtraAttr: []string{"informational"},
			},
			{
				Name: "rtc_stream_with_apm_unmuted", // testplan#9
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: true,
					inputMuted:     false,
					clientStream: clientStreamConfig{
						rtc: true,
						apm: true,
					},
					speechSource:      mouth,
					expectedDetection: false,
				},
				ExtraAttr: []string{"informational"},
			},
			{
				Name: "rtc_stream_with_apm_feature_disabled", // testplan#10
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: false,
					inputMuted:     true,
					clientStream: clientStreamConfig{
						rtc: true,
						apm: true,
					},
					speechSource:      mouth,
					expectedDetection: false,
				},
			},
			{
				Name: "rtc_stream_with_apm_unmuted_feature_disabled", // testplan#11
				Val: crasSpeakOnMuteDetectionParam{
					featureEnabled: false,
					inputMuted:     false,
					clientStream: clientStreamConfig{
						rtc: true,
						apm: true,
					},
					speechSource:      mouth,
					expectedDetection: false,
				},
			},
		},
	})
}

type speechSource int

const (
	mouth speechSource = iota
	speaker
)

type crasSpeakOnMuteDetectionParam struct {
	featureEnabled    bool
	inputMuted        bool
	clientStream      clientStreamConfig
	speechSource      speechSource
	expectedDetection bool
}

type clientStreamConfig struct {
	rtc bool
	apm bool
}

func (c *clientStreamConfig) crasTestClientArgs() []string {
	var args []string
	if c.rtc {
		args = append(args, "--block_size=480")
	} else {
		args = append(args, "--block_size=240")
	}

	if c.apm {
		args = append(args, "--effects=aec")
	}
	return args
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

	param := s.Param().(crasSpeakOnMuteDetectionParam)

	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}
	if err := cras.SetInputMute(ctx, param.inputMuted); err != nil {
		s.Fatal("Cannot mute input: ", err)
	}
	if err := cras.SetSpeakOnMuteDetection(ctx, param.featureEnabled); err != nil {
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
	speechWavDuration := data.TheQuickBrownFoxWavDuration * 2 // "repeat 1" gives us double the duration.
	if err := testexec.CommandContext(ctx, "sox", s.DataPath(data.TheQuickBrownFoxWav), "--channels=2", "--rate=48000", speechWav, "repeat", "1").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Cannot prepare speech.wav with sox: ", err)
	}

	playbackDone := make(chan struct{})
	go func() {
		defer close(playbackDone)

		// go/tast-writing#contexts-and-timeouts recommends the timeout to be double of the expected worst case performance.
		ctx, cancel := context.WithTimeout(ctx, 2*speechWavDuration)
		defer cancel()

		var err error
		switch param.speechSource {
		case mouth:
			// Play to PCM device to simulate speech from mouth.
			err = audio.PlayWavToPCM(ctx, speechWav, device.AloopPlaybackPCM)
		case speaker:
			// Play to default device which goes through to CRAS,
			// to simulate speech played by apps.
			err = audio.PlayWavToDefault(ctx, speechWav)
		}
		if err != nil {
			s.Fatal("Cannot simulate speech: ", err)
		}
	}()

	delayCapture := 2 * time.Second
	s.Logf("Delay capture for %v to let playback start first", delayCapture)
	// GoBigSleepLint: Sleep works the same as having a longer capture + crop.
	if err := testing.Sleep(ctx, delayCapture); err != nil {
		s.Fatal("Cannot sleep: ", err)
	}

	s.Log("Capturing from default device")
	captureRaw := filepath.Join(s.OutDir(), "capture.raw")
	captureCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()

	clientCommand := testexec.CommandContext(captureCtx, "cras_test_client", "-C", captureRaw, "--rate=48000", "-c", "2", "--duration=5")
	clientCommand.Args = append(clientCommand.Args, param.clientStream.crasTestClientArgs()...)
	if err := clientCommand.Run(testexec.DumpLogOnError); err != nil {
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
		captureMuted := rms == 0
		if captureMuted != param.inputMuted {
			s.Errorf("Captured audio muted want: %v; got: %v, rms: %v", param.inputMuted, captureMuted, rms)
		}
	}

	s.Log("Waiting for playback to complete")
	<-playbackDone

	if detected != param.expectedDetection {
		s.Fatalf("Detect speech: want: %v; got: %v", param.expectedDetection, detected)
	}
}
