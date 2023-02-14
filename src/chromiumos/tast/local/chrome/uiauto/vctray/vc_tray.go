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
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
)

// Node finders in the tray bar.
var (
	vcTraySection = nodewith.ClassName("VideoConferenceTray")
	expandButton  = nodewith.Name("Camera and audio controls").Role(role.ToggleButton).Ancestor(vcTraySection)
)

// Node finders in the expanded panel which is implemented in TrayBubbleView.
var (
	panelSection             = nodewith.HasClass("TrayBubbleView").Role(role.Window)
	portraitRelightingButton = nodewith.Name("Portrait Relighting").Role(role.Button).Ancestor(panelSection)
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

// ExpandPanel clicks the up-arrow button in VC tray section to expand the panel.
// It skips action if the panel is already expanded.
func (vcTray VCTray) ExpandPanel(ctx context.Context) error {
	isPanelExpanded, err := vcTray.ui.IsNodeFound(ctx, panelSection)
	if err != nil {
		return err
	} else if isPanelExpanded {
		return nil
	}

	// ShowHotseat makes sure hotseat is shown in tablet mode.
	if err := ash.ShowHotseat(ctx, vcTray.tconn); err != nil {
		return errors.Wrap(err, "failed to show hotseat")
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

// TODO(b/267709319): Add on/off params to switch functions
// once the status can be checked in accessibility.

// TODO(b/267709155): Add returnToApp support.

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
