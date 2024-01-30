// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/health/types"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type audioHardwareInfo struct {
	AudioCards []audioCard `json:"audio_cards"`
}

type audioCard struct {
	AlsaID        string           `json:"alsa_id"`
	BusDevice     *types.BusDevice `json:"bus_device"`
	HDAudioCodecs []hdAudioCodec   `json:"hd_audio_codecs"`
}

type hdAudioCodec struct {
	Name    string `json:"name"`
	Address uint8  `json:"address"`
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ProbeAudioHardwareInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Check that we can probe cros_healthd for audio hardware info",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"chungsheng@google.com",
		},
		BugComponent: "b:982097", // ChromeOS > Platform > Enablement > Health
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"diagnostics"},
		Fixture:      "crosHealthdRunning",
		Timeout:      1 * time.Minute,
	})
}

func validateAudioHardwareInfo(info audioHardwareInfo) error {
	for _, audioCard := range info.AudioCards {
		if audioCard.BusDevice != nil && audioCard.BusDevice.DeviceClass != "audio card" {
			return errors.Errorf("DeviceClass is not audio card: %s", audioCard.BusDevice.DeviceClass)
		}
	}

	return nil
}

func ProbeAudioHardwareInfo(ctx context.Context, s *testing.State) {
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryAudioHardware}
	var info audioHardwareInfo
	if err := croshealthd.RunAndParseJSONTelem(ctx, params, s.OutDir(), &info); err != nil {
		s.Fatal("Failed to get audio hardware telemetry info: ", err)
	}

	if err := validateAudioHardwareInfo(info); err != nil {
		s.Fatalf("Failed to validate, err [%v]", err)
	}
}
