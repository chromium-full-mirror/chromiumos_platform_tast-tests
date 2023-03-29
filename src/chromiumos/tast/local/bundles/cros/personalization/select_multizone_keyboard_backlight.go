// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/personalization"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
	"go.chromium.org/tast/core/ctxutil"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SelectMultizoneKeyboardBacklight,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test selecting keyboard backlight zone colors in personalization hub app",
		Contacts: []string{
			"assistive-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"thuongphan@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model(personalization.RgbSupportedModels...)),
		Timeout:      3 * time.Minute,
		Fixture:      "personalizationWithMultizoneRgbKeyboard",
	})
}

func SelectMultizoneKeyboardBacklight(ctx context.Context, s *testing.State) {
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

	colorIndexes := [...]int{2, 5, 0, 3, 7}
	keyboardBacklightFinder := nodewith.Role(role.StaticText).NameContaining("Keyboard backlight")
	customizeButtonFinder := nodewith.Role(role.StaticText).NameContaining("Customize")
	customizedCheckmarkFinder := nodewith.HasClass("customized-checkmark")
	zonesFinder := nodewith.HasClass("zone-tab")
	doneButtonFinder := nodewith.Role(role.StaticText).NameContaining("Done")

	if err := uiauto.Combine("open Personalization Hub and verify Keyboard settings available",
		personalization.OpenPersonalizationHub(ui),
		ui.WaitUntilExists(keyboardBacklightFinder))(ctx); err != nil {
		s.Fatal("Failed to show Keyboard settings: ", err)
	}

	if err := uiauto.Combine("click on Customize button to open zone customization dialog",
		ui.WaitUntilExists(customizeButtonFinder),
		ui.DoDefault(customizeButtonFinder),
		ui.WaitUntilExists(zonesFinder.First()))(ctx); err != nil {
		s.Fatal("Failed to open zone customization dialog: ", err)
	}

	// Depends on the device model, the number of zones can be 4 or 5 zones.
	zones, err := ui.NodesInfo(ctx, zonesFinder)
	if err != nil {
		s.Fatal("Failed to find zone tab slider: ", err)
	}
	if len(zones) < 2 {
		s.Fatalf("Should be more than %v zone available: %v", len(zones), err)
	}

	for i, zone := range zones {
		if err := setZoneColor(ctx, ui, zone.Name, colorIndexes[i]); err != nil {
			s.Fatalf("Failed to set zone color for %v to %v color: %v", zone.Name, colorIndexes[i], err)
		}
	}

	if err := uiauto.Combine("click on Done button to close zone customization dialog",
		ui.WaitUntilExists(doneButtonFinder),
		ui.DoDefault(doneButtonFinder),
		ui.WaitUntilExists(keyboardBacklightFinder))(ctx); err != nil {
		s.Fatal("Failed to close zone customization dialog: ", err)
	}

	if err := uiauto.Combine("verify checkmark set for customize button",
		ui.WaitUntilExists(customizeButtonFinder),
		ui.WaitUntilExists(customizedCheckmarkFinder))(ctx); err != nil {
		s.Fatal("Failed to verify zone colors are customized: ", err)
	}
}

// setZoneColor selects a zone and sets a color for that zone.
// Each zone has 8 color options to choose from. Clicking on a color will set that color to
// the currently selected zone.
func setZoneColor(ctx context.Context, ui *uiauto.Context, zoneName string, colorIndex int) error {
	zoneTab := nodewith.HasClass("zone-tab").Name(zoneName)
	colorOptionsFinder := nodewith.HasClass("selectable")

	// Click on a zone.
	if err := uiauto.Combine("select a zone",
		ui.DoDefault(zoneTab),
		ui.WaitUntilExists(colorOptionsFinder.First()),
	)(ctx); err != nil {
		return errors.Wrapf(err, "failed to select %s", zoneName)
	}

	// there should be 8 color options for the selected zone.
	colorOptions, err := ui.NodesInfo(ctx, colorOptionsFinder)
	if err != nil {
		return errors.Wrap(err, "failed to find color options")
	}
	if len(colorOptions) != 8 {
		return errors.Wrap(err, "should be 8 color options available")
	}

	// select a color for the selected zone.
	if err := personalization.SetBacklightColor(ctx, ui, colorIndex); err != nil {
		return errors.Wrapf(err, "failed to select color at index %v for %v", colorIndex, zoneName)
	}

	return nil
}
