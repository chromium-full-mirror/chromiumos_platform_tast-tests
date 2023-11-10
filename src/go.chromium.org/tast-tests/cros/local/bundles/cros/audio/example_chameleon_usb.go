// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/audio/withchameleon"
	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleChameleonUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "An example of Chameleon acting as an USB mic",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"crosep-intertech@google.com",
		},
		// BugComponent of CrOS Platform EngProd Interactive Technology
		BugComponent: "b:1280385",
		Attr:         []string{"group:audio_e2e_experimental", "audio_e2e_experimental_usb"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChameleonAudioTestbed,
	})
}

// ExampleChameleonUSB provides an example of using Chameleon as an USB mic.
func ExampleChameleonUSB(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	chameleond := s.FixtValue().(audio.ChameleonAudioTestbedFixture).Chameleond
	audioPortType := chameleon.PortTypeUSBAudioOut
	audioFormat := audio.DefaultChameleonUtilGeneratedAudioFormat
	filename := "audio_example.raw"

	s.Log("Copying audio file from DUT to Chameleon")
	_, token, err := audio.GenerateTestRawDataForChameleon(ctx, chameleond, audioFormat)
	if err != nil {
		s.Fatalf("Failed to copy sound file %s from DUT to chameleon: %v", filename, err)
	}

	defer func(ctx context.Context) {
		s.Log("Clean up golden audio file in chameleon")
		if err := chameleond.DeleteFileInChameleon(ctx, token); err != nil {
			s.Logf("Failed to delete file in chameleon %s: %v", token, err)
		}
	}(cleanupCtx)

	s.Log("Start playing from chameleon")
	playbackDuration := 10 * time.Second
	chameleonAudioFormat := &chameleon.AudioDataFormat{
		FileType:     chameleon.AudioFileTypeRaw,
		SampleFormat: chameleon.AudioSampleFormatS16LE,
		Channel:      audioFormat.Channels,
		Rate:         audioFormat.Rate,
	}
	if err := withchameleon.PlayFileByPortType(ctx, chameleond, token, audioPortType, chameleonAudioFormat); err != nil {
		s.Fatalf("Failed to playFileByPortType %s with token %s: %v", audioPortType, token, err)
	}
	defer func(ctx context.Context) {
		portID, err := chameleond.FetchSupportedPortIDByType(ctx, audioPortType, 0)
		if err != nil {
			s.Logf("Cannot get portid when stopping audio. %s: %v", audioPortType, err)
		}
		if err := chameleond.StopPlayingAudio(ctx, portID); err != nil {
			s.Logf("Cannot stop audio. %s: %v", audioPortType, err)
		}
		if err = chameleond.Unplug(ctx, portID); err != nil {
			s.Logf("Cannot unplug the port. %d: %v", portID, err)
		}
	}(cleanupCtx)

	recordDuration := withchameleon.CalculateRecordDuration(playbackDuration)

	// GoBigSleepLint: sleep while chameleon is playing
	testing.Sleep(ctx, recordDuration)

}
