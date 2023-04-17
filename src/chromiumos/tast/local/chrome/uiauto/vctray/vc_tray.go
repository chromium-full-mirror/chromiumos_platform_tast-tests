// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package vctray contains the ui automation libraries of Video Conference tray.
package vctray

import (
	"context"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/display"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/testing"
)

// Node finders in the tray bar.
var (
	vcTraySection = nodewith.ClassName("VideoConferenceTray")
	expandButton  = nodewith.Name("Camera and audio controls").Role(role.ToggleButton).Ancestor(vcTraySection)
)

// Node finders in the expanded panel which is implemented in TrayBubbleView.
var (
	panelSection             = nodewith.HasClass("TrayBubbleView").Role(role.Window)
	portraitRelightingButton = nodewith.Name("Adjust Lighting").Role(role.Button).Ancestor(panelSection)
	liveCaptionButton        = nodewith.Name("Live Caption").Role(role.Button).Ancestor(panelSection)

	bgBlurOffButton   = nodewith.Name("Off").Role(role.Button).Ancestor(panelSection)
	bgBlurLightButton = nodewith.Name("Light").Role(role.Button).Ancestor(panelSection)
	bgBlurFullButton  = nodewith.Name("Full").Role(role.Button).Ancestor(panelSection)

	showAppsButton = nodewith.Name("Show apps").Role(role.Button).Ancestor(panelSection)
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

// SwitchPortraitRelighting switches on/off the Portrait Relighting option.
func (vcTray VCTray) SwitchPortraitRelighting() action.Action {
	return vcTray.ui.DoDefault(portraitRelightingButton)
}

// SwitchLiveCaption switches on/off the Live Caption option.
func (vcTray VCTray) SwitchLiveCaption() action.Action {
	return vcTray.ui.DoDefault(liveCaptionButton)
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
// It expands vcTray and set both background blur and portrait relighting then collapse the vcTray.
func (vcTray VCTray) SetCameraEffects(backgroundBlur BackgroundBlurLevel, portraitRelighting bool) action.Action {
	return uiauto.Combine("configure camera effects via mcpanel",
		vcTray.ExpandPanel,
		vcTray.SetBackgroundBlur(backgroundBlur),
		// TODO(b/267709319): Add on/off params to switch functions
		// once the status can be checked in accessibility. It is currently blocked by b/266476993.
		// vcTray.SwitchPortraitRelighting(),
		vcTray.CollapsePanel,
	)
}
