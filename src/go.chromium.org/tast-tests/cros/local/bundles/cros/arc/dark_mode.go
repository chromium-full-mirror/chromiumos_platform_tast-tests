// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"strings"
	"time"

	arcpkg "go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DarkMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that ArcSystemUIService changes Settings.Secure",
		Contacts: []string{
			"arc-app-dev@google.com",
		},
		// ChromeOS > Software > ARC++ > Framework > Chrome Integration
		BugComponent: "b:537221",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "android_vm"},
		Fixture:      "arcBooted",
		Timeout:      chrome.GAIALoginTimeout + arcpkg.BootTimeout + 120*time.Second,
	})
}

func DarkMode(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arcpkg.PreData).Chrome
	arc := s.FixtValue().(*arcpkg.PreData).ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	// By default after log in, dark light mode is set as Auto in Personlization Hub.
	// Switch to Light Mode for the test. ToggleLightMode() won't fail even if Light
	// mode is already enabled.
	if err := uiauto.Combine("Enable light mode",
		personalization.OpenPersonalizationHub(ui),
		personalization.ToggleLightMode(ui),
	)(ctx); err != nil {
		s.Fatal("Failed to enable light mode: ", err)
	}

	if err := toggleDarkThemeFromQuickSettings(ctx, tconn, ui); err != nil {
		s.Fatal("Failed to turn on dark theme in Quick Settings: ", err)
	}

	cmd := arc.Command(ctx, "settings", "get", "secure", "ui_night_mode")
	output, err := cmd.Output()
	if err != nil {
		s.Fatal("Failed to get secure settings: ", err)
	}

	// The value of ui_night_mode enabled.
	const darkModeOn = "2"
	if strings.TrimSpace(string(output)) != darkModeOn {
		s.Fatalf("Night_mode wanted: 2 was %s", string(output))
	}
}

// toggleDarkThemeFromQuickSettings opens the Quick Settings and then toggles the Dark Theme.
func toggleDarkThemeFromQuickSettings(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to show quick settings")
	}
	defer quicksettings.Hide(ctx, tconn)

	if err := uiauto.Combine("Toggle Dark theme",
		ui.LeftClick(quicksettings.DisplaySettingsButton),
		ui.LeftClick(quicksettings.FeatureTileDarkTheme),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to toggle Dark theme")
	}
	return nil
}
