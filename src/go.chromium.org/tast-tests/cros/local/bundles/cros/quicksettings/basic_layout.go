// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/pointer"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// basicLayoutTestParam is the parameter for BasicLayout tests.
type basicLayoutTestParam struct {
	// isTabletMode indicates whether the DUT should be in tablet mode.
	isTabletMode bool
	// hasBattery indicates if the DUT has builtin battery.
	hasBattery bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         BasicLayout,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that settings can be found on Quick Settings",
		Contacts: []string{
			"chromeos-sw-engprod@google.com",
			"cros-status-area-eng@google.com",
			"awendy@google.com",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInQsRevampEnabled",
		Params: []testing.Param{
			{
				ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
				Val: basicLayoutTestParam{
					isTabletMode: false,
					hasBattery:   true,
				},
			},
			{
				Name: "no_battery",
				Val: basicLayoutTestParam{
					isTabletMode: false,
					hasBattery:   false,
				},
			},
			{
				Name:              "tablet",
				ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
				Val: basicLayoutTestParam{
					isTabletMode: true,
					hasBattery:   true,
				},
			},
			{
				Name: "tablet_no_battery",
				Val: basicLayoutTestParam{
					isTabletMode: true,
					hasBattery:   false,
				},
			},
		},
	})
}

type basicLayoutTestResources struct {
	cr     *chrome.Chrome
	tconn  *chrome.TestConn
	ui     *uiauto.Context
	pc     pointer.Context
	btn    *nodewith.Finder
	outDir string
}

// BasicLayout verifies that the basic components in Quick Settings are existed.
func BasicLayout(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	param := s.Param().(basicLayoutTestParam)
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, param.isTabletMode)
	if err != nil {
		s.Fatal("Failed to enable the tablet mode: ", err)
	}
	defer cleanup(cleanupCtx)

	var pc pointer.Context
	if param.isTabletMode {
		if pc, err = pointer.NewTouch(ctx, tconn); err != nil {
			s.Fatal("Failed to create touch context: ", err)
		}
	} else {
		pc = pointer.NewMouse(tconn)
	}
	defer func(ctx context.Context) {
		if pc != nil {
			if err := pc.Close(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close pointer context")
			}
		}
	}(ctx)

	resources := &basicLayoutTestResources{
		cr:     cr,
		tconn:  tconn,
		ui:     uiauto.New(tconn),
		pc:     pc,
		btn:    nodewith.Role(role.Button),
		outDir: s.OutDir(),
	}

	s.Log("Enable accessibility and keyboard quick settings")
	if err := enableAccessAndKeyboard(ctx, resources); err != nil {
		s.Fatal("Failed to enable Accessibility and Keyboard: ", err)
	}

	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to open Quick Settings: ", err)
	}
	defer quicksettings.Hide(cleanupCtx, tconn)

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	defer faillog.SaveScreenshotOnError(cleanupCtx, cr, s.OutDir(), s.HasError)

	s.Log("Checking user in Quick Settings")
	if err := checkUser(ctx, resources); err != nil {
		s.Fatal("Failed to check user in Quick Settings: ", err)
	}

	s.Log("Checking buttons in Quick Settings")
	if err := checkButtons(ctx, resources); err != nil {
		s.Fatal("Failed to check buttons in Quick Settings: ", err)
	}

	s.Log("Checking panels in Quick Settings")
	if err := checkPanels(ctx, resources); err != nil {
		s.Fatal("Failed to check panels in Quick Settings: ", err)
	}

	s.Log("Checking sliders in Quick Settings")
	if err := checkSliders(ctx, resources); err != nil {
		s.Fatal("Failed to check sliders in Quick Settings: ", err)
	}

	// If DUT does not have battery (e.g., Chromeboxes and Chromebits),
	// skip battery icon verification.
	s.Log("Checking battery in Quick Settings")
	if param.hasBattery {
		if err := resources.ui.WaitUntilExists(quicksettings.BatteryView)(ctx); err != nil {
			s.Fatal("Failed to find Battery info: ", err)
		}
	}
}

func checkUser(ctx context.Context, res *basicLayoutTestResources) error {
	userEmail := res.cr.User()

	// The user's email should be in the power menu.
	return uiauto.Combine(fmt.Sprintf("find email: %s", userEmail),
		res.pc.Click(quicksettings.PowerMenuButton),
		res.ui.WaitUntilExists(nodewith.Role(role.MenuItem).Name(userEmail)),
		// Close the menu after checking.
		res.pc.Click(quicksettings.PowerMenuButton),
		res.ui.WaitUntilGone(nodewith.Role(role.MenuItem).Name(userEmail)),
	)(ctx)
}

func checkButtons(ctx context.Context, res *basicLayoutTestResources) error {
	for _, node := range []*nodewith.Finder{
		quicksettings.FeatureTileNetwork,
		quicksettings.FeatureTileScreenCapture,
		quicksettings.FeatureTileDoNotDisturb,
		quicksettings.FeatureTileBluetooth,
		quicksettings.FeatureTileCast,
		quicksettings.VolumeToggle,
		quicksettings.PowerMenuButton,
		quicksettings.SettingsButton,
	} {
		nodeInfo, err := res.ui.Info(ctx, node)
		if err != nil || nodeInfo == nil {
			return errors.Wrap(err, "failed to get node info")
		}
		testing.ContextLogf(ctx, "Button %q, %q found", nodeInfo.Name, nodeInfo.ClassName)
	}
	return nil
}

func checkPanels(ctx context.Context, res *basicLayoutTestResources) error {
	// Associate the tiles with names for better error reporting.
	tiles := map[string]*nodewith.Finder{
		"Network":       quicksettings.FeatureTileNetwork,
		"Bluetooth":     quicksettings.FeatureTileBluetooth,
		"Accessibility": quicksettings.FeatureTileAccessibility,
		"Keyboard":      quicksettings.FeatureTileKeyboard,
	}
	for name, tile := range tiles {
		if err := uiauto.Combine("check panel",
			searchPanelInQuickSettings(res, tile),
			res.pc.Click(tile),
			res.pc.Click(res.btn.Name("Previous menu")),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to check panel %q", name)
		}
		testing.ContextLogf(ctx, "Panel %q found", name)

		// Click on page one button if it exists.
		pageBtn := res.btn.NameRegex(regexp.MustCompile(`Page 1 of \d+`)).HasClass("PageIndicatorView")
		if err := uiauto.IfSuccessThen(
			res.ui.WithTimeout(3*time.Second).WaitUntilExists(pageBtn),
			res.pc.Click(pageBtn),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to back to first page")
		}
	}
	return nil
}

func searchPanelInQuickSettings(res *basicLayoutTestResources, targetBtn *nodewith.Finder) uiauto.Action {
	pageIndicator := res.btn.NameRegex(regexp.MustCompile(`Page \d+ of \d+`)).HasClass("PageIndicatorView")

	return uiauto.NamedAction("search panels in pages", func(ctx context.Context) error {
		for nth := 1; ; nth++ {
			if err := res.ui.WaitForLocation(targetBtn)(ctx); err != nil {
				return errors.Wrap(err, "failed to wait for node stable")
			}

			info, err := res.ui.Info(ctx, targetBtn)
			if err != nil {
				return errors.Wrap(err, "failed to get node info")
			}

			if !info.State[state.Offscreen] {
				return nil
			}

			infos, err := res.ui.NodesInfo(ctx, pageIndicator)
			if err != nil {
				return errors.Wrap(err, "failed to get page indicator info")
			} else if nth >= len(infos) {
				return errors.Wrapf(err, "index out of bound [%d, %d)", nth, len(infos))
			}

			if err := res.pc.Click(pageIndicator.Nth(nth))(ctx); err != nil {
				return errors.Wrap(err, "failed to click page indicator")
			}
			testing.ContextLog(ctx, "Click next page")
		}
	})
}

func checkSliders(ctx context.Context, res *basicLayoutTestResources) error {
	for _, sliders := range []struct {
		parentSection *nodewith.Finder
		targetNode    *nodewith.Finder
	}{
		{nil, quicksettings.VolumeSlider},
		{nil, quicksettings.BrightnessSlider},
		// TODO(b/225081940): Enable the audio settings checks when the issue is fixed.
		// Audio details might be missing on some DUT models (e.g., kaisa, blacktip)
		// {res.btn.Name("Audio settings"), quicksettings.MicGainSlider},
	} {
		if sliders.parentSection != nil {
			if err := res.pc.Click(sliders.parentSection)(ctx); err != nil {
				return errors.Wrap(err, "failed to click parent section of a slider")
			}
		}

		if err := res.ui.WaitUntilExists(sliders.targetNode)(ctx); err != nil {
			return errors.Wrap(err, "failed to find slider")
		}

		if sliders.parentSection != nil {
			if err := res.pc.Click(res.btn.Name("Previous menu"))(ctx); err != nil {
				return errors.Wrap(err, "failed to go back to previous")
			}
		}
	}

	return nil
}

// enableAccessAndKeyboard enables accessibility and keyboard quick settings.
func enableAccessAndKeyboard(ctx context.Context, res *basicLayoutTestResources) (retErr error) {
	setting, err := ossettings.LaunchAtPageURL(ctx, res.tconn, res.cr, "osAccessibility", func(context.Context) error { return nil })
	if err != nil {
		return errors.Wrap(err, "failed to open setting page")
	}
	defer setting.Close(ctx)
	defer faillog.DumpUITreeOnErrorToFile(ctx, res.outDir, func() bool { return retErr != nil }, res.tconn, "ui_dump_enable_access_and_keyboard")

	optionName := "Show accessibility options in Quick Settings"
	if err := uiauto.Combine("add input methods",
		res.ui.WaitUntilExists(nodewith.Name(optionName).Role(role.ToggleButton)),
		setting.SetToggleOption(res.cr, optionName, true),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to toggle accessibility settings")
	}

	if err := setting.NavigateToPageURL(ctx, res.cr, "osLanguages", res.pc.Click(nodewith.NameStartingWith("Inputs"))); err != nil {
		return errors.Wrap(err, "failed to enter inputs settings page")
	}

	return uiauto.Combine("add input methods",
		res.pc.Click(nodewith.Role(role.Button).Name("Add input methods")),
		res.pc.Click(nodewith.Role(role.CheckBox).First()),
		res.pc.Click(nodewith.Role(role.Button).Name("Add")),
		res.ui.WaitUntilExists(nodewith.HasClass("list-item").First()),
		setting.SetToggleOption(res.cr, "Show input options in the shelf", false),
	)(ctx)
}
