// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/jsontypes"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type audioInfo struct {
	InputDeviceName  string           `json:"input_device_name"`
	InputGain        jsontypes.Uint32 `json:"input_gain"`
	InputMute        bool             `json:"input_mute"`
	OutputDeviceName string           `json:"output_device_name"`
	OutputMute       bool             `json:"output_mute"`
	OutputVolume     jsontypes.Uint64 `json:"output_volume"`
	SevereUnderruns  jsontypes.Uint32 `json:"severe_underruns"`
	Underruns        jsontypes.Uint32 `json:"underruns"`
	OutputNodes      *[]audioNodeInfo `json:"output_nodes"`
	InputNodes       *[]audioNodeInfo `json:"input_nodes"`
}

type audioNodeInfo struct {
	ID            jsontypes.Uint64 `json:"id"`
	Name          string           `json:"name"`
	DeviceName    string           `json:"device_name"`
	Active        bool             `json:"active"`
	NodeVolume    uint8            `json:"node_volume"`
	InputNodeGain uint8            `json:"input_node_gain"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeAudioInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for audio info",
		Contacts:     []string{"cros-tdm-tpe-eng@google.com"},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"chrome", "diagnostics"},
		Fixture:      "crosHealthdRunning",
	})
}

func validateAudioData(audio *audioInfo) error {
	// Check "input_device_name" is not empty.
	if audio.InputDeviceName == "" {
		return errors.New("Failed. input_device_name field is empty")
	}

	// Check "input_gain" is integer and between [0, 100].
	if audio.InputGain > 100 {
		return errors.Errorf("Failed. input_gain is not in a legal range [0, 100]: %d", audio.InputGain)
	}

	// Check "output_device_name" is not empty.
	if audio.OutputDeviceName == "" {
		return errors.New("Failed. output_device_name field is empty")
	}

	// Check "output_volume" is integer and between [0, 100].
	if audio.OutputVolume > 100 {
		return errors.Errorf("Failed. output_volume is not in a legal range [0, 100]: %d", audio.OutputVolume)
	}

	return nil
}

func ProbeAudioInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryAudio}
	var audio audioInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &audio); err != nil {
		s.Fatal("Failed to get audio telemetry info: ", err)
	}

	if err := validateAudioData(&audio); err != nil {
		s.Fatalf("Failed to validate audio data, err [%v]", err)
	}
}
