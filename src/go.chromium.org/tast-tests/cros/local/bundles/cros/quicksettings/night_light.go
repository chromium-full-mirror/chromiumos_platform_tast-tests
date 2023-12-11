// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/settings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NightLight,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that the Quick Settings Night Light feature pod button is working correctly",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"leandre@chromium.org",
			"cros-status-area-eng@google.com",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-39d4666a-39dc-40ed-a918-75acf57335a0",
		}},
		Fixture: "chromeLoggedIn",
	})
}

// NightLight tests that Night Light feature pod button is working correctly.
func NightLight(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	// Get night light state at the beginning.
	state1, err := settings.NightLightEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get Night Light state: ", err)
	}

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to show Quick Settings: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Night light is a top-level button in quick settings.
	nightLightBtn := nodewith.Role(role.ToggleButton).NameStartingWith("Toggle Night Light")

	if err := ui.LeftClick(nightLightBtn)(ctx); err != nil {
		s.Fatal("Failed to click the Night Light button: ", err)
	}

	// Get night light state after the first toggle.
	state2, err := settings.NightLightEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get Night Light state: ", err)
	}

	if state1 == state2 {
		s.Fatal("Night Light state did not change after toggling the feature pod button")
	}

	if err := ui.LeftClick(nightLightBtn)(ctx); err != nil {
		s.Fatal("Failed to click the Night Light button: ", err)
	}

	// Get night light state after the second toggle.
	state3, err := settings.NightLightEnabled(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get Night Light state: ", err)
	}

	if state1 != state3 {
		s.Fatal("Night Light state did not change back to the beginning state after toggling twice")
	}
}
