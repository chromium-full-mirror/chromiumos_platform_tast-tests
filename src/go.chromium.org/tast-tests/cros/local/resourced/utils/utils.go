// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// CheckSetGameMode tests SetGameMode functionality.
func CheckSetGameMode(ctx context.Context, rm *resourced.Client) (resErr error) {
	// Get the original game mode.
	origGameMode, err := rm.GameMode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query game mode state")
	}
	testing.ContextLog(ctx, "Original game mode: ", origGameMode)

	defer func() {
		// Restore game mode.
		if err = rm.SetGameMode(ctx, origGameMode); err != nil {
			if resErr == nil {
				resErr = errors.Wrap(err, "failed to reset game mode state")
			} else {
				testing.ContextLog(ctx, "Failed to reset game mode state: ", err)
			}
		}
	}()

	// Set game mode to different value.
	newGameMode := resourced.GameModeOff
	if origGameMode == resourced.GameModeOff {
		newGameMode = resourced.GameModeARCVM
	}
	if err = rm.SetGameMode(ctx, newGameMode); err != nil {
		return errors.Wrap(err, "failed to set game mode state")
	}
	testing.ContextLog(ctx, "Set game mode: ", newGameMode)

	// Check game mode is set to the new value.
	gameMode, err := rm.GameMode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query game mode state")
	}
	if newGameMode != gameMode {
		return errors.Errorf("set game mode to: %d, but got game mode: %d", newGameMode, gameMode)
	}
	return nil
}

// CheckSetGameModeWithTimeout tests SetGameModeWithTimeout functionality.
func CheckSetGameModeWithTimeout(ctx context.Context, rm *resourced.Client) (resErr error) {
	newGameMode := resourced.GameModeARCVM
	if err := rm.SetGameModeWithTimeout(ctx, newGameMode, 1); err != nil {
		return errors.Wrap(err, "failed to set game mode state")
	}
	testing.ContextLog(ctx, "Set game mode with 1 second timeout: ", newGameMode)

	// Check game mode is set to the new value.
	gameMode, err := rm.GameMode(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to query game mode state")
	}
	if newGameMode != gameMode {
		return errors.Errorf("set game mode to: %d, but got game mode: %d", newGameMode, gameMode)
	}

	// Check game mode is reset after timeout.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		gameMode, err := rm.GameMode(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to query game mode state")
		}
		if gameMode != resourced.GameModeOff {
			return errors.New("game mode is not reset")
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Second, Interval: 100 * time.Millisecond}); err != nil {
		return errors.Wrap(err, "failed to wait for game mode reset")
	}
	return nil
}
