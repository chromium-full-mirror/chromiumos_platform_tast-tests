// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vctray contains the ui automation libraries of Video Conference tray.
package vctray

import (
	"context"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"

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
	panelSection = nodewith.HasClass("RootView").Role(role.Dialog).First().Ancestor(
		nodewith.HasClass("SettingBubbleContainer").Role(role.Window).First(),
	)
	adjustLightingButton = nodewith.NameStartingWith("Toggle Improve lighting").Role(role.ToggleButton).Ancestor(panelSection)
	liveCaptionButton    = nodewith.NameStartingWith("Toggle Live Caption").Role(role.ToggleButton).Ancestor(panelSection)

	bgBlurOffButton   = nodewith.NameContaining("Off").Role(role.Button).Ancestor(panelSection)
	bgBlurLightButton = nodewith.NameContaining("Light").Role(role.Button).Ancestor(panelSection)
	bgBlurFullButton  = nodewith.NameContaining("Full").Role(role.Button).Ancestor(panelSection)

	showAppsButton = nodewith.NameContaining("Used by").Role(role.Button).Ancestor(panelSection)
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
	isPanelExpanded, err := vcTray.ui.IsNodeFound(ctx, panelSection)
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
	return vcTray.ui.DoDefaultUntil(expandButton, vcTray.ui.WithTimeout(3*time.Second).WaitUntilExists(panelSection))(ctx)
}

// CollapsePanel clicks the down-arrow button in VC tray section to collapse the panel.
// It skips action if the panel is not expanded.
func (vcTray VCTray) CollapsePanel(ctx context.Context) error {
	isPanelExpanded, err := vcTray.ui.IsNodeFound(ctx, panelSection)
	if err != nil {
		return err
	} else if !isPanelExpanded {
		return nil
	}

	return vcTray.ui.DoDefaultUntil(expandButton, vcTray.ui.WithTimeout(3*time.Second).WaitUntilGone(panelSection))(ctx)
}

// BackgroundBlurLevel represents the type of background blur option.
type BackgroundBlurLevel int

// Available options of background blur settings.
const (
	BackgroundBlurOff BackgroundBlurLevel = iota
	BackgroundBlurLight
	BackgroundBlurFull
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
	default:
		return func(context.Context) error {
			return errors.Errorf("background blur level %q is not supported", blurLevel)
		}
	}
}

// adjustLightingEnabled returns whether adjustLighting is enabled.
// It assumes the vcTray panel is expanded already.
func (vcTray VCTray) adjustLightingEnabled(ctx context.Context) (bool, error) {
	nodeInfo, err := vcTray.ui.Info(ctx, adjustLightingButton)
	if err != nil {
		return false, errors.Wrap(err, "failed to get node info")
	}
	// Current status can be identified by the node name.
	// Off: "Adjust Lighting is off"; On: "Adjust Lighting is on".
	return strings.HasSuffix(nodeInfo.Name, "on"), nil
}

// SetAdjustLighting toggles on/off the "Adjust Lighting" option.
func (vcTray VCTray) SetAdjustLighting(expectedOn bool) action.Action {
	return func(ctx context.Context) error {
		currentlyOn, err := vcTray.adjustLightingEnabled(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get current status")
		}
		if (currentlyOn && expectedOn) || (!currentlyOn && !expectedOn) {
			return nil
		}

		return vcTray.ui.DoDefaultUntil(
			adjustLightingButton,
			func(ctx context.Context) error {
				return testing.Poll(ctx, func(ctx context.Context) error {
					currentlyOn, err := vcTray.adjustLightingEnabled(ctx)
					if err != nil {
						return errors.Wrap(err, "failed to get current status")
					}
					if (currentlyOn && !expectedOn) || (!currentlyOn && expectedOn) {
						return errors.New("failed to change adjustlighting")
					}
					return nil
				}, &testing.PollOptions{Timeout: 3 * time.Second})
			},
		)(ctx)
	}
}

// liveCaptionEnabled returns whether live caption is enabled.
// It assumes the vcTray panel is expanded already.
func (vcTray VCTray) liveCaptionEnabled(ctx context.Context) (bool, error) {
	nodeInfo, err := vcTray.ui.Info(ctx, liveCaptionButton)
	if err != nil {
		return false, errors.Wrap(err, "failed to get node info")
	}
	// Current status can be identified by the node name.
	// Off: "Live Caption is off"; On: "Live Caption is on".
	return strings.HasSuffix(nodeInfo.Name, "on"), nil
}

// SetLiveCaption toggles on/off the "Live Caption" option.
func (vcTray VCTray) SetLiveCaption(expectedOn bool) action.Action {
	return func(ctx context.Context) error {
		currentlyOn, err := vcTray.liveCaptionEnabled(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get current status")
		}
		if (currentlyOn && expectedOn) || (!currentlyOn && !expectedOn) {
			return nil
		}

		return vcTray.ui.DoDefaultUntil(
			liveCaptionButton,
			func(ctx context.Context) error {
				return testing.Poll(ctx, func(ctx context.Context) error {
					currentlyOn, err := vcTray.liveCaptionEnabled(ctx)
					if err != nil {
						return errors.Wrap(err, "failed to get current status")
					}
					if (currentlyOn && !expectedOn) || (!currentlyOn && expectedOn) {
						return errors.New("failed to change live caption")
					}
					return nil
				}, &testing.PollOptions{Timeout: 3 * time.Second})
			},
		)(ctx)
	}
}

// ReturnToApp returns an action returning to the VC app.
func (vcTray VCTray) ReturnToApp(appName string) action.Action {
	appFinder := nodewith.NameContaining(appName).Role(role.Button).Ancestor(panelSection)

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
