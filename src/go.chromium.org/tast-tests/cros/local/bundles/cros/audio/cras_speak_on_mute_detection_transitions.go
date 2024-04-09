// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/data"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/device"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CrasSpeakOnMuteDetectionTransitions,
		Desc:         "Test CRAS detection of speaking while on mute with system state changes",
		Contacts:     []string{"chromeos-audio-bugs@google.com", "aaronyu@google.com"},
		BugComponent: "b:776546", // ChromeOS > Platform > Technologies > Audio > Test > Tast
		Attr: []string{
			"group:mainline",
			"group:cbx", "cbx_feature_enabled", "cbx_stable",
		},
		Fixture: fixture.AloopLoaded{
			Channels: 2,
			Parent:   fixture.UIStopped{}.Instance(),
		}.Instance(),
		Data:    []string{data.TheQuickBrownFoxWav},
		Timeout: 3 * time.Minute,
		Params: []testing.Param{
			{
				Name: "mute",
				Val: crasSpeakOnMuteDetectionTransitionsParam{
					states: [2]somdState{
						{
							systemMuted:    false,
							featureEnabled: true,
						},
						{
							systemMuted:    true,
							featureEnabled: true,
						},
					},
				},
			},
			{
				Name: "unmute",
				Val: crasSpeakOnMuteDetectionTransitionsParam{
					states: [2]somdState{
						{
							systemMuted:    true,
							featureEnabled: true,
						},
						{
							systemMuted:    false,
							featureEnabled: true,
						},
					},
				},
			},
			{
				Name: "enable_feature_while_muted",
				Val: crasSpeakOnMuteDetectionTransitionsParam{
					states: [2]somdState{
						{
							systemMuted:    true,
							featureEnabled: false,
						},
						{
							systemMuted:    true,
							featureEnabled: true,
						},
					},
				},
			},
			{
				Name: "disable_feature_while_muted",
				Val: crasSpeakOnMuteDetectionTransitionsParam{
					states: [2]somdState{
						{
							systemMuted:    true,
							featureEnabled: true,
						},
						{
							systemMuted:    true,
							featureEnabled: false,
						},
					},
				},
			},
			{
				Name: "enable_feature_unmuted",
				Val: crasSpeakOnMuteDetectionTransitionsParam{
					states: [2]somdState{
						{
							systemMuted:    false,
							featureEnabled: false,
						},
						{
							systemMuted:    false,
							featureEnabled: true,
						},
					},
				},
			},
			{
				Name: "disable_feature_unmuted",
				Val: crasSpeakOnMuteDetectionTransitionsParam{
					states: [2]somdState{
						{
							systemMuted:    false,
							featureEnabled: true,
						},
						{
							systemMuted:    false,
							featureEnabled: false,
						},
					},
				},
			},
		},
	})
}

type somdState struct {
	systemMuted    bool
	featureEnabled bool
}

type crasSpeakOnMuteDetectionTransitionsParam struct {
	states [2]somdState
}

func CrasSpeakOnMuteDetectionTransitions(ctx context.Context, s *testing.State) {
	cras, err := audio.NewCras(ctx)
	if err != nil {
		s.Fatal("Cannot connect to CRAS: ", err)
	}

	if err := audio.SelectIODevices(ctx, cras, "ALSA_LOOPBACK", "ALSA_LOOPBACK"); err != nil {
		s.Fatal("Cannot select ALSA loopback: ", err)
	}

	param := s.Param().(crasSpeakOnMuteDetectionTransitionsParam)

	speechWav := filepath.Join(s.OutDir(), "speech.wav")
	// "repeat 2" gives us triple the duration, which is around 10 seconds.
	speechWavDuration := data.TheQuickBrownFoxWavDuration * 3
	if err := testexec.CommandContext(ctx, "sox", s.DataPath(data.TheQuickBrownFoxWav), "--channels=2", "--rate=48000", speechWav, "repeat", "2").Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Cannot prepare speech.wav with sox: ", err)
	}
	playbackCaptureCtx, cancel := context.WithTimeout(ctx, 2*speechWavDuration) // Use 2x expected duration.
	defer cancel()

	// Prepare initial state.
	s.Logf("Setting mute=%t", param.states[0].systemMuted)
	if err := cras.SetInputMute(ctx, param.states[0].systemMuted); err != nil {
		s.Fatal("Cannot set mute: ", err)
	}
	s.Logf("Setting SpeakOnMuteDetection=%t", param.states[0].featureEnabled)
	if err := cras.SetSpeakOnMuteDetection(ctx, param.states[0].featureEnabled); err != nil {
		s.Fatal("Cannot set speak on mute detection: ", err)
	}

	// Run capture.
	captureDone := make(chan struct{})
	go func() {
		defer close(captureDone)
		if err := testexec.CommandContext(
			playbackCaptureCtx,
			"cras_test_client", "-C", "/dev/null",
			"--block_size=480",
			fmt.Sprintf("--duration=%v", int(speechWavDuration.Seconds())),
		).Run(testexec.DumpLogOnError); err != nil {
			s.Error("Cannot run capture: ", err)
		}
	}()

	// Transition state at T/2.
	go func() {
		// GoBigSleepLint: Transition state at the exact time.
		if err := testing.Sleep(ctx, speechWavDuration/2); err != nil {
			s.Error("Cannot sleep: ", err)
			return
		}
		if param.states[0].systemMuted != param.states[1].systemMuted {
			s.Logf("Setting mute=%t", param.states[1].systemMuted)
			if err := cras.SetInputMute(ctx, param.states[1].systemMuted); err != nil {
				s.Fatal("Cannot set mute: ", err)
			}
		}
		if param.states[0].featureEnabled != param.states[1].featureEnabled {
			s.Logf("Setting SpeakOnMuteDetection=%t", param.states[1].featureEnabled)
			if err := cras.SetSpeakOnMuteDetection(ctx, param.states[1].featureEnabled); err != nil {
				s.Fatal("Cannot set speak on mute detection: ", err)
			}
		}
	}()

	// Run playback.
	if err := audio.PlayWavToPCM(playbackCaptureCtx, speechWav, device.AloopPlaybackPCM); err != nil {
		s.Error("Cannot run playback: ", err)
	}

	s.Log("Waiting for capture to complete")
	<-captureDone
}
