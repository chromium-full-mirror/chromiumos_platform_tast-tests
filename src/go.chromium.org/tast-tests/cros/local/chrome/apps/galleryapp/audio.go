// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package galleryapp

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// TogglePlayPause returns a function that plays or pauses audio or video
// via clicking the 'Toggle play pause' button.
func (g *Gallery) TogglePlayPause(play bool) uiauto.Action {
	return func(ctx context.Context) error {
		togglePlayPauseFinder := nodewith.Name("Toggle play pause").Ancestor(RootFinder)
		togglePlayPauseButton, err := g.ui.FindAnyExists(ctx,
			togglePlayPauseFinder.Role(role.Button),
			togglePlayPauseFinder.Role(role.ToggleButton))
		if err != nil {
			return err
		}

		playing, err := g.IsPlaying(ctx)
		if err != nil {
			return err
		}
		if (play && playing) || (!play && !playing) {
			return nil
		}
		if err := uiauto.NamedAction("toggle play pause",
			g.ui.LeftClick(togglePlayPauseButton),
		)(ctx); err != nil {
			return err
		}

		if play {
			return g.WaitUntilPlaying(ctx)
		}
		return g.WaitUntilPaused(ctx)
	}
}

// StepBackwardTenSeconds returns a function that clicks 'Step backward 10
// seconds' button.
func (g *Gallery) StepBackwardTenSeconds() uiauto.Action {
	stepBackwardButton := nodewith.Name("Step backward 10 seconds").Role(role.Button).Ancestor(RootFinder)
	return uiauto.NamedAction("step backward 10 seconds",
		g.ui.LeftClick(stepBackwardButton),
	)
}

// StepForwardTenSeconds returns a function that clicks 'Step forward 10
// seconds' button.
func (g *Gallery) StepForwardTenSeconds() uiauto.Action {
	stepForwardButton := nodewith.Name("Step forward 10 seconds").Role(role.Button).Ancestor(RootFinder)
	return uiauto.NamedAction("step forward 10 seconds",
		g.ui.LeftClick(stepForwardButton),
	)
}

// SkipPrevious returns a function that clicks 'Skip previous' button.
func (g *Gallery) SkipPrevious() uiauto.Action {
	skipPreviousButton := nodewith.Name("Skip previous").Role(role.Button).Ancestor(RootFinder)
	return uiauto.NamedAction("skip previous",
		g.ui.LeftClick(skipPreviousButton),
	)
}

// SkipNext returns a function that clicks 'Skip next' button.
func (g *Gallery) SkipNext() uiauto.Action {
	skipNextButton := nodewith.Name("Skip next").Role(role.Button).Ancestor(RootFinder)
	return uiauto.NamedAction("skip next",
		g.ui.LeftClick(skipNextButton),
	)
}

// GoToTrackStart returns a function that clicks 'Skip previous' button
// to go to the start of the track.
func (g *Gallery) GoToTrackStart() uiauto.Action {
	zeroTimeElapsed := nodewith.Name("00:00").Role(role.StaticText).Ancestor(RootFinder)
	return uiauto.IfSuccessThen(g.ui.Gone(zeroTimeElapsed),
		uiauto.NamedCombine("go to track start",
			g.SkipPrevious(),
			g.ui.WaitUntilExists(zeroTimeElapsed),
		))
}

// Speed ​​is the option for playback speed.
type Speed string

const (
	// SpeedOption1 is an option to set playback speed to 0.5.
	SpeedOption1 Speed = "0.5"
	// SpeedOption2 is an option to set playback speed to 1.0.
	SpeedOption2 Speed = "Normal"
	// SpeedOption3 is an option to set playback speed to 1.2.
	SpeedOption3 Speed = "1.2"
	// SpeedOption4 is an option to set playback speed to 1.5.
	SpeedOption4 Speed = "1.5"
	// SpeedOption5 is an option to set playback speed to 2.
	SpeedOption5 Speed = "2"
)

// SetPlaybackSpeed returns a function that sets audio playback speed to the
// specified speed.
func (g *Gallery) SetPlaybackSpeed(s Speed) uiauto.Action {
	return func(ctx context.Context) error {
		playbackSpeedFinder := nodewith.NameStartingWith("Playback speed").Ancestor(RootFinder)
		playbackSpeedMenu := nodewith.Role(role.Menu).Ancestor(RootFinder)
		playbackSpeedButton, err := g.ui.FindAnyExists(ctx,
			playbackSpeedFinder.Role(role.PopUpButton),
			playbackSpeedFinder.Role(role.ToggleButton))
		if err != nil {
			return err
		}
		if err := g.ui.LeftClickUntil(playbackSpeedButton,
			g.ui.WithTimeout(5*time.Second).WaitUntilExists(playbackSpeedMenu))(ctx); err != nil {
			return err
		}
		itemFinder := nodewith.Name(string(s)).Ancestor(RootFinder)
		playbackSpeedItem, err := g.ui.FindAnyExists(ctx,
			itemFinder.Role(role.MenuItemRadio),
			itemFinder.Role(role.MenuItem))
		if err != nil {
			return err
		}
		return uiauto.NamedAction(fmt.Sprintf("set playback speed to %s", s),
			g.ui.LeftClick(playbackSpeedItem),
		)(ctx)
	}
}
