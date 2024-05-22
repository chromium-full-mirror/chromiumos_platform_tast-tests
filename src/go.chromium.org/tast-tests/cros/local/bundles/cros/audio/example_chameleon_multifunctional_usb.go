// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/audio/withchameleon"
	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/crastestclient"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleChameleonMultifunctionalUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "An example of Chameleon acting as an USB mic with Multifunctional Gadget API",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"crosep-intertech@google.com",
		},
		// BugComponent of CrOS Platform EngProd Interactive Technology
		BugComponent:    "b:1280385",
		Attr:            []string{"group:audio_e2e_experimental", "audio_e2e_experimental_usb"},
		SoftwareDeps:    []string{"chrome"},
		Fixture:         fixture.ChameleonAudioTestbed,
		VariantCategory: `{"name": "Audio_Board"}`,
	})
}

// ExampleChameleonMultifunctionalUSB an example of using Chameleon as an USB mic that has Audio and HID capabilities
func ExampleChameleonMultifunctionalUSB(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	chameleond := s.FixtValue().(audio.ChameleonAudioTestbedFixture).Chameleond
	audioPortType := chameleon.PortTypeUSBMFGAudioOut
	audioFormat := audio.DefaultChameleonUtilGeneratedAudioFormat

	// Preparing files
	s.Log("Start generating golden audio file")
	goldenFile, token, err := audio.GenerateTestRawDataForChameleon(ctx, chameleond, audioFormat)
	if err != nil {
		s.Fatal("Failed to GenerateTestRawDataChameleon: ", err)
	}

	defer func(ctx context.Context) {
		s.Log("Clean up golden audio file in chameleon")
		if err := chameleond.DeleteFileInChameleon(ctx, token); err != nil {
			s.Logf("Failed to delete file in chameleon %s: %v", token, err)
		}
	}(cleanupCtx)

	recordingFile, err := os.CreateTemp("", "30SEC_recorded*.raw")
	if err != nil {
		s.Fatal("Failed to create raw recording file: ", err)
	}

	s.Logf("Golden File %s", goldenFile.Name())
	s.Logf("Recording File %s", recordingFile.Name())
	defer func(ctx context.Context) {
		s.Log("Clean up golden and recording file on DUT")
		if err := os.Remove(goldenFile.Name()); err != nil {
			s.Logf("Failed to delete golden file in dut %s: %v", goldenFile.Name(), err)
		}
		if err := os.Remove(recordingFile.Name()); err != nil {
			s.Logf("Failed to delete recording file in dut %s: %v", recordingFile.Name(), err)
		}
	}(cleanupCtx)

	// Playing and recording audio
	s.Log("Start playing from chameleon")
	playbackDuration := 10 * time.Second
	chameleonAudioFormat := &chameleon.AudioDataFormat{
		FileType:     chameleon.AudioFileTypeRaw,
		SampleFormat: chameleon.AudioSampleFormatS16LE,
		Channel:      audioFormat.Channels,
		Rate:         audioFormat.Rate,
	}
	if err := withchameleon.PlayFileByPortType(ctx, chameleond, token, audioPortType, chameleonAudioFormat); err != nil {
		s.Fatalf("Failed to PlayFileByPortType %s with token %s: %v", audioPortType, token, err)
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

	if err := crastestclient.CaptureFileCommand(
		ctx, recordingFile.Name(),
		int(recordDuration.Seconds()),
		audioFormat.Channels,
		audioFormat.Rate).Run(testexec.DumpLogOnError); err != nil {
		s.Fatal("Failed to capture: ", err)
	}

	// Check recorded frequency of audio file
	err = audio.CheckRecordedFrequency(ctx, recordingFile, audioFormat)
	if err != nil {
		s.Fatal("Failed to check recorded frequency: ", err)
	}

}
