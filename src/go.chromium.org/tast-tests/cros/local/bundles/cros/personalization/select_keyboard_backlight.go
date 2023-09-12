// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SelectKeyboardBacklight,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test selecting keyboard backlight color in personalization hub app",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"thuongphan@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-6b2d1b1d-3977-431c-a8b3-b1b9c41f6d44",
		}},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model(personalization.RgbSupportedModels...)),
		Timeout:      3 * time.Minute,
		Fixture:      "personalizationWithRgbKeyboard",
	})
}

func SelectKeyboardBacklight(ctx context.Context, s *testing.State) {
	const (
		backlightColor1 = "Blue"
		backlightColor2 = "Rainbow"
		backlightColor3 = "Wallpaper color"
	)
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	if err := uiauto.Combine("open Personalization Hub and verify Keyboard settings available",
		personalization.OpenPersonalizationHub(ui),
		ui.WaitUntilExists(nodewith.Role(role.StaticText).NameContaining("Keyboard backlight")))(ctx); err != nil {
		s.Fatal("Failed to show Keyboard settings: ", err)
	}

	// There should be 9 color options for keyboard backlight.
	colorOptionsFinder := nodewith.HasClass("selectable")
	colorOptions, err := ui.NodesInfo(ctx, colorOptionsFinder)
	if err != nil {
		s.Fatal("Failed to find color options: ", err)
	}
	if len(colorOptions) != 9 {
		s.Fatal("Should be 9 color options available")
	}

	// Set the keyboard backlight to three different colors and verify whether the backlight color is updated for each change.
	colorIndexOptions := []int{3, 8, 0}
	for _, colorIndex := range colorIndexOptions {
		if err := personalization.SetBacklightColor(ctx, ui, colorIndex); err != nil {
			s.Fatalf("Failed to set backlight color to color at index %v: %v", colorIndex, err)
		}
	}
}
