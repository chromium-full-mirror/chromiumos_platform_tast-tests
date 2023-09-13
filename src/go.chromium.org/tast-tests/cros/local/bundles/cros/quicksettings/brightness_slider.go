// Copyright 2022 The ChromiumOS Authors
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
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BrightnessSlider,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the Quick Settings brightness slider can be adjusted by keyboard",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"sylvieliu@chromium.org",
			"cros-system-ui-eng@google.com",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

// BrightnessSlider tests that the brightness slider can be adjusted up and down.
func BrightnessSlider(ctx context.Context, s *testing.State) {
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

	initialBrightness, err := quicksettings.SliderValue(ctx, tconn, quicksettings.SliderTypeBrightness)
	if err != nil {
		s.Fatal("Failed initial value check for brightness slider: ", err)
	}

	decreaseBrightness, err := quicksettings.DecreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeBrightness)
	if err != nil {
		s.Fatalf("Failed to decrease brightness slider from %d: %v", initialBrightness, err)
	}
	s.Log("Decreased brightness slider value: ", decreaseBrightness)

	increaseBrightness, err := quicksettings.IncreaseSlider(ctx, tconn, kb, quicksettings.SliderTypeBrightness)
	if err != nil {
		s.Fatal("Failed to increase brightness slider: ", err)
	}
	s.Log("Increased brightness slider value: ", increaseBrightness)
}
