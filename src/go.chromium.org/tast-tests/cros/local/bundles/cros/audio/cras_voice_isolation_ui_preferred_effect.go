// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/audio/fixture"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: CrasVoiceIsolationUIPreferredEffect,
		Desc: "Checks that sound devices for recording are recognized",
		Contacts: []string{
			"chromeos-audio-bugs@google.com",
			"aaronyu@google.com",
		},
		BugComponent:    "b:776546",
		Attr:            []string{"group:mainline", "informational"},
		VariantCategory: `{"name": "Audio_Model"}`,
		Params: []testing.Param{
			{
				Name: "noise_cancellation",
				Val:  "NOISE_CANCELLATION",
				Fixture: fixture.CrasSetUp{
					VoiceIsolationUIPreferredEffect: audio.VoiceIsolationEffectNoiseCancellation,
				}.Instance(),
			},
			{
				Name: "style_transfer",
				Val:  "STYLE_TRANSFER",
				Fixture: fixture.CrasSetUp{
					VoiceIsolationUIPreferredEffect: audio.VoiceIsolationEffectStyleTransfer,
				}.Instance(),
			},
			{
				Name: "beamforming",
				Val:  "BEAMFORMING",
				Fixture: fixture.CrasSetUp{
					VoiceIsolationUIPreferredEffect: audio.VoiceIsolationEffectBeamforming,
				}.Instance(),
				ExtraTestBedDeps: []string{
					tbdep.AudioBeamforming("intelligo"),
				},
			},
		},
	})
}

func CrasVoiceIsolationUIPreferredEffect(ctx context.Context, s *testing.State) {
	wantEffect := s.Param().(string)
	fixt := s.FixtValue().(fixture.CrasFixtValue)
	cras := fixt.Cras()

	gotEffect, err := cras.GetVoiceIsolationUIPreferredEffect(ctx)
	if err != nil {
		s.Fatal("GetVoiceIsolationUIPreferredEffect(): ", err)
	}
	if gotEffect != wantEffect {
		s.Fatalf("GetVoiceIsolationUIPreferredEffect() = %v != %v", gotEffect, wantEffect)
	}
}
