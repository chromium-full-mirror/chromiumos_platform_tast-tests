// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package graphics

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/typecutils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type displayCompositionTestParams struct {
	tabletMode  bool
	displayType string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtendedDisplayCompositionCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies graphics composition on extended display",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts: []string{
			"intel.chrome.automation.team@intel.com",
			"pathan.jilani@intel.com",
		},
		SoftwareDeps: []string{"chrome"},
		// To skip on duffy(Chromebox) with no internal display.
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Fixture:      "chromeGraphics",
		Params: []testing.Param{{
			Name: "hdmi_clamshell_mode",
			Val: displayCompositionTestParams{
				tabletMode:  false,
				displayType: usbutils.NativeHDMI,
			},
			ExtraAttr: []string{"group:intel-hdmi"},
		}, {
			Name: "hdmi_tablet_mode",
			Val: displayCompositionTestParams{
				tabletMode:  true,
				displayType: usbutils.NativeHDMI,
			},
			ExtraAttr: []string{"group:intel-hdmi"},
		}, {
			Name: "dp_clamshell_mode",
			Val: displayCompositionTestParams{
				tabletMode:  false,
				displayType: usbutils.NativeDP,
			},
			ExtraAttr: []string{"group:intel-dp"},
		}},
	})
}

func ExtendedDisplayCompositionCheck(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	testOpt := s.Param().(displayCompositionTestParams)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if testOpt.tabletMode {
		cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
		if err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer cleanup(cleanupCtx)
	}

	const (
		settingsDeviceText  = "Device"
		settingsDisplayText = "Display"
	)

	var (
		resolutionMenuParams  = nodewith.Name("Resolution").Role(role.ComboBoxSelect)
		refreshRateMenuParams = nodewith.Name("Refresh Rate Menu").Role(role.ComboBoxSelect)
		refreshRate60HzParam  = nodewith.Name("60 Hz").Role(role.ListBoxOption).First()
		builtinDisplayParams  = nodewith.Name("Mirror Built-in display").Role(role.ToggleButton)
	)
	numberOfDisplay := 1
	spec := usbutils.DisplaySpec{
		NumberOfDisplays: &numberOfDisplay,
		DisplayType:      testOpt.displayType,
	}
	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
		s.Fatal("Failed to check for connected external display: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		capps, err := ash.ChromeApps(ctx, tconn)
		if err != nil {
			return testing.PollBreak(err)
		}
		for _, app := range capps {
			if app.AppID == apps.Settings.ID {
				return nil
			}
		}
		return errors.New("Settings app not yet found in available Chrome apps")
	}, nil); err != nil {
		s.Fatal("Failed to find the settings app in the available Chrome apps: ", err)
	}

	cui := uiauto.New(tconn)
	leftClickUIElement := func(elementFinder *nodewith.Finder) error {
		if err := cui.WaitUntilExists(elementFinder)(ctx); err != nil {
			return errors.Wrap(err, "failed to find elementFinder")
		}
		if err := cui.LeftClick(elementFinder)(ctx); err != nil {
			return errors.Wrap(err, "failed to left click element")
		}
		return nil
	}

	// Launch the Settings app and wait for it to open.
	if err := apps.Launch(ctx, tconn, apps.Settings.ID); err != nil {
		s.Fatal("Failed to launch the Settings app: ", err)
	}
	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, 5*time.Second); err != nil {
		s.Fatal("Failed to appear settings app in the shelf: ", err)
	}

	if err := settingsPage(ctx, tconn, cui, role.Link, settingsDeviceText); err != nil {
		s.Fatalf("Failed to click on %q option in settings page: %v", settingsDeviceText, err)
	}

	if err := settingsPage(ctx, tconn, cui, role.Link, settingsDisplayText); err != nil {
		s.Fatalf("Failed to click on %q option in Device page: %v", settingsDisplayText, err)
	}

	if testOpt.tabletMode {
		if err := leftClickUIElement(builtinDisplayParams); err != nil {
			s.Fatal("Failed to find and click built-in display checkbox with error: ", err)
		}
	}

	var displayName string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		displayInfo, err := display.GetInfo(ctx, tconn)
		if err != nil {
			return errors.Wrap(err, "failed to get external display name")
		}
		if len(displayInfo) < 2 {
			return errors.New("failed please connect external 4K monitor to DUT")
		}
		displayName = displayInfo[1].Name
		return nil
	}, &testing.PollOptions{
		Timeout: 15 * time.Second,
	}); err != nil {
		s.Fatal("Failed to get external display info: ", err)
	}

	if err := settingsPage(ctx, tconn, cui, role.Tab, displayName); err != nil {
		s.Fatalf("Failed to click on 'External Display %v' option in Display page with error: %v", displayName, err)
	}

	if !testOpt.tabletMode {
		//Scroll to down the page
		tpw, err := input.Trackpad(ctx)
		if err != nil {
			s.Fatal("Failed to create a trackpad device: ", err)
		}
		defer tpw.Close(ctx)

		tw, err := tpw.NewMultiTouchWriter(2)
		if err != nil {
			s.Fatal("Failed to create a multi touch writer: ", err)
		}
		defer tw.Close()
		// Perform two finger scrolling on the trackpad
		doTrackpadScroll := func(ctx context.Context) error {
			x0 := tpw.Width() / 2
			y0 := tpw.Height() / 4
			x1 := tpw.Width() / 2
			y1 := tpw.Height() / 4 * 3
			d := tpw.Width() / 8 // x-axis distance between two fingers
			const t = time.Second
			return tw.DoubleSwipe(ctx, x0, y0, x1, y1, d, t)
		}
		if err := doTrackpadScroll(ctx); err != nil {
			s.Fatal("Failed to perform two finger scroll: ", err)
		}
	}

	// Check if the 4k resolution @3840 x 2160 and refresh rate @60HZ is getting listed in the drop down menu.
	if err := leftClickUIElement(resolutionMenuParams); err != nil {
		s.Fatal("Failed to find and click resolution menu with error: ", err)
	}

	info, err := display.GetInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}

	if err := typecutils.SetDisplayResolution(ctx, tconn, &info[1], 3840, 2160, cr); err != nil {
		s.Fatal("Failed to change resolution: ", err)
	}
	if err := cui.ScrollToVisible(refreshRateMenuParams)(ctx); err != nil {
		s.Fatal("Failed to scroll far enough down to refresh rate settings: ", err)
	}
	if err := leftClickUIElement(refreshRateMenuParams); err != nil {
		s.Fatal("Failed to find and click refresh rate menu with error: ", err)
	}

	if err := leftClickUIElement(refreshRate60HzParam); err != nil {
		s.Fatal("Failed to find and click '60 Hz' with error: ", err)
	}
}

// settingsPage performs UI element find and click, after opening settings page.
func settingsPage(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context, role role.Role, name string) error {
	confirm := nodewith.Name(name).Role(role)
	if err := ui.WaitForLocation(confirm)(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for element")
	}
	if err := ui.LeftClick(confirm)(ctx); err != nil {
		return errors.Wrap(err, "failed to left click element")
	}
	return nil
}
