// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vctray contains the ui automation libraries of Video Conference tray.
package vctray

import (
	"context"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Node finders in the tray bar.
var (
	vcTraySection = nodewith.HasClass("VideoConferenceTray")
	expandButton  = nodewith.Name("Camera and audio controls").Role(role.ToggleButton).Ancestor(vcTraySection)
)

// Node finders in the expanded panel which is implemented in TrayBubbleView.
var (
	bubleView = nodewith.NameContaining("Video Call Controls").HasClass("BubbleView").Role(role.Window)

	adjustLightingButton    = nodewith.NameStartingWith("Toggle Improve lighting").Role(role.ToggleButton).Ancestor(bubleView)
	liveCaptionButton       = nodewith.NameStartingWith("Toggle Live Caption").Role(role.ToggleButton).Ancestor(bubleView)
	adjustCameraFraming     = nodewith.NameStartingWith("Toggle Camera framing").Role(role.ToggleButton).Ancestor(bubleView)
	noiseCancellationButton = nodewith.NameStartingWith("Toggle Noise cancellation").Role(role.ToggleButton).Ancestor(bubleView)

	buttonNameRegexp           = regexp.MustCompile(`.*Button.*`)
	bgBlurOffButton            = nodewith.NameContaining("Off").ClassNameRegex(buttonNameRegexp).Ancestor(bubleView)
	bgBlurLightButton          = nodewith.NameContaining("Light").ClassNameRegex(buttonNameRegexp).Ancestor(bubleView)
	bgBlurFullButton           = nodewith.NameContaining("Full").ClassNameRegex(buttonNameRegexp).Ancestor(bubleView)
	bgBlurImageButton          = nodewith.NameContaining("Image").ClassNameRegex(buttonNameRegexp).Ancestor(bubleView)
	createwWithAiButton        = nodewith.NameContaining("Create with AI").Role(role.Button).Ancestor(bubleView)
	firstBackgroundImageButton = nodewith.ClassName("RecentlyUsedImageButton").Role(role.ListItem).First()
	showAppsButton             = nodewith.NameContaining("Used by").Role(role.Button).Ancestor(bubleView)
)

// VCTray represents the type of video conference tray.
type VCTray struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

// New creates a new instance of VC tray.
// It returns error if the VC tray is not present.
func New(ctx context.Context, tconn *chrome.TestConn) *VCTray {
	return &VCTray{tconn, uiauto.New(tconn)}
}

// Exists returns true if vcTray is shown.
// It throws out error if failed to check existence of the vcTray.
func (vcTray VCTray) Exists(ctx context.Context) (bool, error) {
	return vcTray.ui.IsNodeFound(ctx, vcTraySection)
}

// WaitUntilExists waits until the vcTray appears.
func (vcTray VCTray) WaitUntilExists(ctx context.Context) error {
	return vcTray.ui.WaitUntilExists(vcTraySection)(ctx)
}

// WaitUntilGone waits until the vcTray disappears.
func (vcTray VCTray) WaitUntilGone(ctx context.Context) error {
	return vcTray.ui.WaitUntilGone(vcTraySection)(ctx)
}

// ExpandPanel clicks the up-arrow button in VC tray section to expand the panel.
// It skips action if the panel is already expanded.
func (vcTray VCTray) ExpandPanel(ctx context.Context) error {
	isPanelExpanded, err := vcTray.ui.IsNodeFound(ctx, bubleView)
	if err != nil {
		return err
	} else if isPanelExpanded {
		return nil
	}

	// Set shelf to never hide to force show shelf.
	dispInfo, err := display.GetPrimaryInfo(ctx, vcTray.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to get primary display info")
	}

	origShelfBehavior, err := ash.GetShelfBehavior(ctx, vcTray.tconn, dispInfo.ID)
	if err != nil {
		return errors.Wrap(err, "failed to get original shelf behavior")
	}

	if origShelfBehavior != ash.ShelfBehaviorNeverAutoHide {
		if err := ash.SetShelfBehavior(ctx, vcTray.tconn, dispInfo.ID, ash.ShelfBehaviorNeverAutoHide); err != nil {
			return errors.Wrap(err, `failed to set shelf behavior to "never hidden"`)
		}
		defer func(ctx context.Context) error {
			if err := ash.SetShelfBehavior(ctx, vcTray.tconn, dispInfo.ID, origShelfBehavior); err != nil {
				testing.ContextLog(ctx, "Failed to revert shelf behavior")
			}
			return nil
		}(ctx)
	}

	if err := ash.WaitForShelf(ctx, vcTray.tconn, 3*time.Second); err != nil {
		return errors.Wrap(err, "shelf is not visible")
	}
	return vcTray.ui.DoDefaultUntil(expandButton, vcTray.ui.WithTimeout(3*time.Second).WaitUntilExists(bubleView))(ctx)
}

// CollapsePanel clicks the down-arrow button in VC tray section to collapse the panel.
// It skips action if the panel is not expanded.
func (vcTray VCTray) CollapsePanel(ctx context.Context) error {
	isPanelExpanded, err := vcTray.ui.IsNodeFound(ctx, bubleView)
	if err != nil {
		return err
	} else if !isPanelExpanded {
		return nil
	}

	return vcTray.ui.DoDefaultUntil(expandButton, vcTray.ui.WithTimeout(3*time.Second).WaitUntilGone(bubleView))(ctx)
}

// BackgroundBlurLevel represents the type of background blur option.
type BackgroundBlurLevel int

// Available options of background blur settings.
const (
	BackgroundBlurOff BackgroundBlurLevel = iota
	BackgroundBlurLight
	BackgroundBlurFull
	BackgroundBlurImage
)

// SetBackgroundBlur selects desired background blur option.
func (vcTray VCTray) SetBackgroundBlur(blurLevel BackgroundBlurLevel) action.Action {
	switch blurLevel {
	case BackgroundBlurLight:
		return vcTray.ui.DoDefault(bgBlurLightButton)
	case BackgroundBlurFull:
		return vcTray.ui.DoDefault(bgBlurFullButton)
	case BackgroundBlurOff:
		return vcTray.ui.DoDefault(bgBlurOffButton)
	case BackgroundBlurImage:
		return uiauto.Combine("ApplyBackgroundReplaceFromUi",
			// TODO(b/340352012):remove the sleep after the bug is fixed.
			// GoBigSleepLint: wait for 1 second for the background images being loaded
			// in the background. Note that we can't wait for the
			// firstBackgroundImageButton because they are hidden after loaded.
			uiauto.Sleep(time.Second),
			vcTray.ui.DoDefault(bgBlurImageButton),
			vcTray.ui.DoDefault(firstBackgroundImageButton),
		)
	default:
		return func(context.Context) error {
			return errors.Errorf("background blur level %q is not supported", blurLevel)
		}
	}
}

// featureEnabled returns whether the specified feature is enabled.
// It assumes the vcTray panel is expanded already.
func (vcTray VCTray) featureEnabled(ctx context.Context, finder *nodewith.Finder) (bool, error) {
	nodeInfo, err := vcTray.ui.Info(ctx, finder)
	if err != nil {
		return false, errors.Wrap(err, "failed to get node info")
	}
	// Current status can be identified by the node name.
	// Off: "<Feature> is off"; On: "<Feature> is on".
	return strings.HasSuffix(nodeInfo.Name, "on"), nil
}

// SetFeature toggles on/off the specified option.
func (vcTray VCTray) SetFeature(finder *nodewith.Finder, expectedOn bool) action.Action {
	return func(ctx context.Context) error {
		currentlyOn, err := vcTray.featureEnabled(ctx, finder)
		if err != nil {
			return errors.Wrap(err, "failed to get current status")
		}
		if currentlyOn == expectedOn {
			return nil
		}

		return vcTray.ui.DoDefaultUntil(
			finder,
			func(ctx context.Context) error {
				return testing.Poll(ctx, func(ctx context.Context) error {
					currentlyOn, err := vcTray.featureEnabled(ctx, finder)
					if err != nil {
						return errors.Wrap(err, "failed to get current status")
					}
					if currentlyOn != expectedOn {
						return errors.New("failed to change feature status")
					}
					return nil
				}, &testing.PollOptions{Timeout: 10 * time.Second})
			},
		)(ctx)
	}
}

// SetAdjustLighting toggles on/off the "Adjust Lighting" option.
func (vcTray VCTray) SetAdjustLighting(expectedOn bool) action.Action {
	return vcTray.SetFeature(adjustLightingButton, expectedOn)
}

// SetNoiseCancellation toggles on/off the "Noise cancellation" option.
func (vcTray VCTray) SetNoiseCancellation(expectedOn bool) action.Action {
	return vcTray.SetFeature(noiseCancellationButton, expectedOn)
}

// SetCameraFraming toggles on/off the "Camera Framing" option.
func (vcTray VCTray) SetCameraFraming(expectedOn bool) action.Action {
	return vcTray.SetFeature(adjustCameraFraming, expectedOn)
}

// SetLiveCaption toggles on/off the "Live Caption" option.
func (vcTray VCTray) SetLiveCaption(expectedOn bool) action.Action {
	return vcTray.SetFeature(liveCaptionButton, expectedOn)
}

// ReturnToApp returns an action returning to the VC app.
func (vcTray VCTray) ReturnToApp(appName string) action.Action {
	appFinder := nodewith.NameContaining(appName).Role(role.Button).Ancestor(bubleView)

	return func(ctx context.Context) error {
		// Multiple VC apps are hidden inside the app list.
		// Click "Show apps" arrow button will expand the list.
		foundFinder, err := vcTray.ui.FindAnyExists(ctx, showAppsButton, appFinder)
		if err != nil {
			return errors.Wrap(err, "failed to find vc app/section")
		}

		if foundFinder == showAppsButton {
			if err := vcTray.ui.DoDefaultUntil(
				showAppsButton,
				vcTray.ui.WithTimeout(3*time.Second).WaitUntilExists(appFinder),
			)(ctx); err != nil {
				return errors.Wrap(err, "failed to expand app list")
			}
		}

		return vcTray.ui.DoDefault(appFinder)(ctx)
	}
}

// ReturnToAppForWindow returns an action returning to the VC app, also handles window state.
func (vcTray VCTray) ReturnToAppForWindow(appName string, tconn *chrome.TestConn) action.Action {
	return func(ctx context.Context) error {
		window, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, appName)
		if err != nil {
			errors.Wrap(err, "can't find window with appName")
		}

		initialState := window.State
		if initialState == ash.WindowStateMinimized {
			return errors.New("can't return to a minimized window")
		}
		// Minimized the window.
		if err := ash.SetWindowStateAndWait(ctx, tconn, window.ID, ash.WindowStateMinimized); err != nil {
			return errors.Wrap(err, "failed to minimized the window")
		}

		// Click on the app name from the vcTray.
		if err := uiauto.Combine("return to tab via vcpanel",
			vcTray.ExpandPanel,
			vcTray.ReturnToApp(appName),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to click on the return to app button")
		}

		// Wait until the animation to complete.
		if err := ash.WaitWindowFinishAnimating(ctx, tconn, window.ID); err != nil {
			return errors.Wrap(err, "failed to wait until the animation is over")
		}

		// Check the restored state.
		if err := ash.WaitForCondition(ctx, tconn, func(wd *ash.Window) bool {
			return window.ID == wd.ID && wd.State == initialState && !wd.IsAnimating
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			return errors.Wrap(err, "failed to maximize the window by return to app")
		}

		return nil
	}
}

// OpenVcBackgroundApp clicks on the Create with AI button and opens the VcBackgroundApp.
func (vcTray VCTray) OpenVcBackgroundApp() action.Action {
	actionsToPerform := []action.Action{vcTray.ExpandPanel}
	actionsToPerform = append(actionsToPerform, vcTray.ui.DoDefault(bgBlurImageButton))
	actionsToPerform = append(actionsToPerform, vcTray.ui.DoDefault(createwWithAiButton))
	return uiauto.Combine("OpenVcBackgroundApp",
		actionsToPerform...,
	)
}

// SetCameraEffects is a high level wrapper to setup camera effects from main screen.
// It expands vcTray and set both background blur and relighting then collapse the vcTray.
func (vcTray VCTray) SetCameraEffects(backgroundBlur BackgroundBlurLevel, adjustRelighting bool) action.Action {
	return vcTray.ChangeSettingsInPanel(
		vcTray.SetBackgroundBlur(backgroundBlur),
		vcTray.SetAdjustLighting(adjustRelighting),
	)
}

// ChangeSettingsInPanel changes one or more settings in vcTray panel.
// It automatically expand the panel and close it in the end.
// e.g. vcTray.ChangeSettingsInPanel(vcTray.SetbackgroundBlur(backgroundBlur)) literally does something like below:
// vcTray.ExpandPanel,
// vcTray.SetBackgroundBlur(backgroundBlur),
// vcTray.CollaposePanel,
func (vcTray VCTray) ChangeSettingsInPanel(actions ...action.Action) action.Action {
	actionsToPerform := []action.Action{vcTray.ExpandPanel}
	actionsToPerform = append(actionsToPerform, actions...)
	actionsToPerform = append(actionsToPerform, vcTray.CollapsePanel)
	return uiauto.NamedCombine("change settings in panel",
		actionsToPerform...,
	)
}
