// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"chromiumos/tast/common/audio/withchameleon"
	"chromiumos/tast/common/chameleon"
	"chromiumos/tast/common/fixture"
	"chromiumos/tast/local/audio"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExampleChameleonUSB,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "An example of Chameleon acting as an USB mic",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"yjle@google.com",
		},
		// BugComponent of CrOS Platform EngProd Interactive Technology
		BugComponent: "b:1280385",
		// TODO: (Optional) create a group for tast tests that use chameleon
		// Attr:         []string{"group:audio_chameleon_example"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.ChameleonAudioTestbed,
	})
}

// ExampleChameleonUSB provides an example of using Chameleon as an USB mic.
func ExampleChameleonUSB(ctx context.Context, s *testing.State) {
	chameleond := s.FixtValue().(audio.ChameleonAudioTestbedFixture).Chameleond
	audioPortType := chameleon.PortTypeUSBAudioOut
	filename := "audio_example.raw"

	s.Log("Copying audio file from DUT to Chameleon")
	token, err := chameleond.CopyFileToChameleon(ctx, generateBinaryInput(ctx, s), filename)
	if err != nil {
		s.Fatalf("Failed to copy sound file %s from DUT to chameleon: %v", filename, err)
	}

	s.Log("Start playing from chameleon")
	playbackDuration := 10 * time.Second
	if err = withchameleon.PlayFileByPortType(ctx, chameleond, token, audioPortType, playbackDuration); err != nil {
		s.Fatal("Failed to playFileByPortType: ", token, audioPortType, err)
	}

}

func generateBinaryInput(ctx context.Context, s *testing.State) []byte {
	const (
		audioRate    = 48000
		audioChannel = 2
		duration     = 30
	)

	rawTempFile, err := ioutil.TempFile("", "30SEC_*.raw")
	if err != nil {
		s.Error("Failed to create raw temp file: ", err)
	}
	if err := rawTempFile.Close(); err != nil {
		s.Error("Failed to close raw temp file: ", err)
	}
	rawFile := audio.TestRawData{
		Path:          rawTempFile.Name(),
		BitsPerSample: 16,
		Channels:      audioChannel,
		Rate:          audioRate,
		Frequencies:   []int{440, 440},
		Volume:        0.05,
		Duration:      duration,
	}
	if err := audio.GenerateTestRawData(ctx, rawFile); err != nil {
		s.Fatal("Failed to generate audio test data: ", err)
	}

	bytes, err := os.ReadFile(rawFile.Path)
	if err != nil {
		s.Fatal("Failed to read audio test data and convert to binary form")
	}

	return bytes
}
