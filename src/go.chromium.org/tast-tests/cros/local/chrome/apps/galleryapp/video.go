// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package galleryapp

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// ToggleFullscreen returns a function that sets the video to full screen
// or non-full screen via clicking "Toggle fullscreen" button.
func (g *Gallery) ToggleFullscreen(isFullscreen bool) uiauto.Action {
	return func(ctx context.Context) error {
		w, err := ash.FindWindow(ctx, g.tconn, func(w *ash.Window) bool {
			return strings.Contains(w.Title, apps.Gallery.Name)
		})
		if err != nil {
			return errors.Wrap(err, "failed to find gallery window")
		}
		isExpectedState := func(w *ash.Window) bool {
			return (isFullscreen && w.State == ash.WindowStateFullscreen) ||
				(!isFullscreen && w.State != ash.WindowStateFullscreen)
		}
		if isExpectedState(w) {
			testing.ContextLog(ctx, "The window is already in the expected window state")
			return nil
		}
		waitExpectedWindowState := func(ctx context.Context) error {
			if err := ash.WaitForCondition(ctx, g.tconn, func(w *ash.Window) bool {
				return strings.Contains(w.Title, apps.Gallery.Name) && isExpectedState(w)
			}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
				return errors.Wrap(err, "failed to wait for window state")
			}
			return nil
		}
		toggleFullscreenButton := nodewith.Name("Toggle fullscreen").Role(role.Button).Ancestor(RootFinder)
		return uiauto.NamedCombine(fmt.Sprintf("toggle full screen to %v", isFullscreen),
			g.showSeekSlider(),
			g.ui.DoDefaultUntil(toggleFullscreenButton,
				waitExpectedWindowState),
		)(ctx)
	}
}

// SetPIP returns a function that sets playback option to picture in picture
// mode or not.
func (g *Gallery) SetPIP(isPIP bool) uiauto.Action {
	ui := g.ui
	pipFinder := nodewith.Name("Picture in picture")
	pipWindow := pipFinder.ClassName("Widget").Role(role.Window)
	pipCheckBox := pipFinder.Role(role.MenuItemCheckBox)

	if isPIP {
		return uiauto.IfSuccessThen(ui.Gone(pipWindow),
			uiauto.NamedCombine("enter picture in picture mode",
				g.OpenPlaybackOptions(),
				ui.DoDefaultUntil(pipCheckBox,
					ui.WaitUntilCheckedState(pipCheckBox, isPIP)),
				ui.WaitUntilExists(pipWindow),
			))
	}

	return uiauto.IfSuccessThen(ui.Exists(pipWindow),
		uiauto.NamedCombine("exit picture in picture mode",
			g.OpenPlaybackOptions(),
			ui.DoDefaultUntil(pipCheckBox,
				ui.WaitUntilCheckedState(pipCheckBox, isPIP)),
			ui.WaitUntilGone(pipWindow),
		))
}

// OpenPlaybackOptions returns a function that open playback option.
func (g *Gallery) OpenPlaybackOptions() uiauto.Action {
	ui := g.ui
	playbackOptionsFinder := nodewith.Name("Playback options").Ancestor(RootFinder)
	playbackOptionsMenu := nodewith.Role(role.Menu).Ancestor(RootFinder)
	openPlaybackOptionsMenu := func(ctx context.Context) error {
		playbackOptionsButton, err := ui.FindAnyExists(ctx,
			playbackOptionsFinder.Role(role.PopUpButton),
			playbackOptionsFinder.Role(role.ToggleButton))
		if err != nil {
			return err
		}
		return ui.DoDefaultUntil(playbackOptionsButton,
			ui.WithTimeout(5*time.Second).WaitUntilExists(playbackOptionsMenu))(ctx)
	}

	return uiauto.IfFailThen(ui.Exists(playbackOptionsMenu),
		uiauto.Retry(3, uiauto.NamedCombine("open playback options",
			g.showSeekSlider(),
			openPlaybackOptionsMenu)))
}

// NavigateNext returns a function that clicks 'Navigate next' button.
func (g *Gallery) NavigateNext() uiauto.Action {
	navigateNextButton := nodewith.Name("Navigate next").Role(role.Button).Ancestor(RootFinder)
	return uiauto.NamedAction("navigate next",
		g.ui.LeftClick(navigateNextButton),
	)
}

// NavigateBack returns a function that clicks 'Navigate back' button.
func (g *Gallery) NavigateBack() uiauto.Action {
	navigateBackButton := nodewith.Name("Navigate back").Role(role.Button).Ancestor(RootFinder)
	return uiauto.NamedAction("navigate back",
		g.ui.LeftClick(navigateBackButton),
	)
}

// showSeekSlider returns a function that moves mouse to show seek slider.
func (g *Gallery) showSeekSlider() uiauto.Action {
	ui := g.ui
	rootWebArea := nodewith.Role(role.RootWebArea).Ancestor(RootFinder).First()
	seekSlider := nodewith.Name("Seek").Role(role.Slider).Ancestor(RootFinder)

	return uiauto.IfFailThen(ui.Exists(seekSlider),
		uiauto.Retry(3, uiauto.NamedCombine("show progress bar",
			mouse.Move(g.tconn, coords.NewPoint(0, 0), 0),
			ui.MouseMoveTo(rootWebArea, 200*time.Millisecond),
			ui.WaitUntilExists(seekSlider),
		)))
}
