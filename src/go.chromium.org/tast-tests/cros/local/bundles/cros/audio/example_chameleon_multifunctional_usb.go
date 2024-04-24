// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/chameleon"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/audio"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/audio/withchameleon"
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

func playPortTypeUSBAudioOut(ctx context.Context, s *testing.State, chameleond chameleon.Chameleond) {
	cleanupCtx := ctx

	playFrom := withchameleon.ChameleonOutputPort{
		ChamPortType: chameleon.PortTypeUSBMFGAudioOut,
	}
	playTo := withchameleon.CrosInputPort{
		CrasNodeType: "USB",
	}

	deferFunc, err := withchameleon.Setup(ctx, chameleond, playFrom, playTo)
	defer deferFunc(cleanupCtx)
	if err != nil {
		s.Fatal("Failed to setup chameleon: ", err)
	}

	deferFunc, err = withchameleon.Orchestrate(ctx, chameleond, playFrom, playTo)
	defer deferFunc(cleanupCtx)
	if err != nil {
		s.Fatal("Failed to ochestrate chameleon: ", err)
	}

}

func recordPortTypeUSBAudioIn(ctx context.Context, s *testing.State, chameleond chameleon.Chameleond) {
	cleanupCtx := ctx

	playFrom := withchameleon.CrosOutputPort{
		CrasNodeType: "USB",
	}
	playTo := withchameleon.ChameleonInputPort{
		ChamPortType: chameleon.PortTypeUSBMFGAudioIn,
	}

	deferFunc, err := withchameleon.Setup(ctx, chameleond, playFrom, playTo)
	defer deferFunc(cleanupCtx)
	if err != nil {
		s.Fatal("Failed to setup chameleon: ", err)
	}

	deferFunc, err = withchameleon.Orchestrate(ctx, chameleond, playFrom, playTo)
	defer deferFunc(cleanupCtx)
	if err != nil {
		s.Fatal("Failed to ochestrate chameleon: ", err)
	}

}

// ExampleChameleonMultifunctionalUSB an example of using Chameleon as an USB mic that has Audio and HID capabilities
func ExampleChameleonMultifunctionalUSB(ctx context.Context, s *testing.State) {
	chameleond := s.FixtValue().(audio.ChameleonAudioTestbedFixture).Chameleond

	// cham:play -> DUT:record
	playPortTypeUSBAudioOut(ctx, s, chameleond)

	// cham:record -> DUT:play
	recordPortTypeUSBAudioIn(ctx, s, chameleond)
}
