// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MicGainSlider,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the Quick Settings mic gain slider can be adjusted",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"amehfooz@chromium.org",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-39d4666a-39dc-40ed-a918-75acf57335a0",
		}},
		Fixture: "chromeLoggedIn",
		// kakadu audio is currently broken: https://crbug.com/1153016
		// atlas is flaky: b/189732223
		HardwareDeps: hwdep.D(hwdep.Microphone(), hwdep.SkipOnModel("kakadu", "atlas")),
	})
}

// MicGainSlider tests that the mic gain slider can be adjusted up and down.
func MicGainSlider(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// Set up the keyboard, which is used to increment/decrement the slider.
	// TODO(crbug/1123231): use better slider automation controls if possible, instead of keyboard controls.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show Quick Settings: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	initial, err := quicksettings.SliderValue(ctx, tconn, quicksettings.SliderTypeMicGain)
	if err != nil {
		s.Fatal("Failed initial value check for mic gain slider: ", err)
	}
	s.Log("Initial mic gain slider value: ", initial)

	decrease, err := quicksettings.DecreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeMicGain)
	if err != nil {
		s.Fatal("Failed to decrease mic gain slider: ", err)
	}
	s.Log("Decreased mic gain slider value: ", decrease)

	increase, err := quicksettings.IncreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeMicGain)
	if err != nil {
		s.Fatal("Failed to increase mic gain slider: ", err)
	}
	s.Log("Increased mic gain slider value: ", increase)
}
