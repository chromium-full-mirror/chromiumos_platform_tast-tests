// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"context"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/memory/kernelmeter"
	"go.chromium.org/tast-tests/cros/local/resourced"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func readSwappiness(ctx context.Context) (int, error) {
	fileBytes, err := os.ReadFile("/proc/sys/vm/swappiness")

	if err != nil {
		return 0, errors.Wrap(err, "failed to read swappiness")
	}
	swappinessVal, errConv := strconv.Atoi(strings.TrimSuffix(string(fileBytes), "\n"))
	if errConv != nil {
		return 0, errors.Wrap(errConv, "failed to parse swappiness to int")
	}
	return swappinessVal, nil
}

func readTHP(ctx context.Context, thpFile string) (string, error) {
	thp, err := os.ReadFile(thpFile)
	if err != nil {
		return "", err
	}
	// The thp is of format like `[always] madvise never`, with the value
	// inside [] as the mode that's currently used.
	re := regexp.MustCompile(`.*\[(.+)\].*`)
	match := re.FindStringSubmatch(string(thp))
	return match[1], nil
}

// validateSwappiness checks swappiness is tuned correctly:
//  1. for VM game, tuned to 30;
//  2. for others, not tuned.
func validateSwappiness(ctx context.Context, newGameMode uint8) error {
	const VMSwappiness = 30
	const DefaultSwappiness = 60
	// GoBigSleepLint: add a sleep to avoid possible flakiness that can
	// be caused by the async modification of swappiness.
	testing.Sleep(ctx, 500*time.Millisecond)
	swappinessVal, err := readSwappiness(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to read swappiness")
	}
	if newGameMode == resourced.GameModeVM {
		// For VM Game, swappiness should be 30.
		if swappinessVal != VMSwappiness {
			return errors.Errorf("swappiness value should be 30, but get %d", swappinessVal)
		}
	} else {
		// For other cases, swappiness should be default 60.
		if swappinessVal != DefaultSwappiness {
			return errors.Errorf("swappiness value should be 60, but get %d", swappinessVal)
		}
	}
	testing.ContextLog(ctx, "Swappiness validation succeed")
	return nil
}

// validateTHP checks if transparent huage page is tuned correctly:
//  1. for VM game, tuned to always mode;
//  2. for others, not tuned.
func validateTHP(ctx context.Context, newGameMode uint8) error {
	const VMTHP = "always"
	const DefaultTHP = "madvise"

	const thpFile = "/sys/kernel/mm/transparent_hugepage/enabled"
	if _, err := os.Stat(thpFile); err != nil {
		testing.ContextLog(ctx, "THP is not enabled, skip the validation of THP tuning")
		return nil
	}
	thp, err := readTHP(ctx, thpFile)
	if err != nil {
		return errors.Wrap(err, "failed to read THP mode")
	}
	memInfo, err := kernelmeter.MemInfo()
	if err != nil {
		return errors.Wrap(err, "cannot obtain memory info")
	}
	// THP tuning is enabled for boards with total memory > 8GiB.
	if memInfo.Total <= kernelmeter.NewMemSizeMiB(9*1024) {
		testing.ContextLog(ctx, "THP tuning is not enabled on this device, skip the validation of THP tuning")
		return nil
	}

	if newGameMode == resourced.GameModeVM {
		// For VM Game, THP should be always mode.
		if thp != VMTHP {
			return errors.Errorf("THP mode should be always, but got %s", thp)
		}
	} else {
		// For other cases, THP should be default madvise mode.
		if thp != DefaultTHP {
			return errors.Errorf("THP mode should be madvise, but got %s", thp)
		}
	}
	testing.ContextLog(ctx, "THP validation succeed")
	return nil
}

// CheckSetGameMode tests SetGameMode functionality and also the tunings that come along with the change of game mode if specified.
// We will check the tuning of swappiness along with the change of game mode if checkSwappinessTuning is true;
// and we will check the tuning transparent huge pages along with the change of game mode if checkTHPTuning is true.
func CheckSetGameMode(ctx context.Context, rm *resourced.Client, checkSwappinessTuning, checkTHPTuning bool) (resErr error) {
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
	var newGameMode uint8
	if origGameMode == 0 {
		newGameMode = 1
	}
	if err = rm.SetGameMode(ctx, newGameMode); err != nil {
		return errors.Wrap(err, "failed to set game mode state")
	}
	testing.ContextLog(ctx, "Set game mode: ", newGameMode)

	if checkSwappinessTuning {
		if err := validateSwappiness(ctx, newGameMode); err != nil {
			return errors.Wrap(err, "validate swappiness failed")
		}
	}

	if checkTHPTuning {
		if err := validateTHP(ctx, newGameMode); err != nil {
			return errors.Wrap(err, "validate THP failed")
		}
	}

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

// CheckSetGameModeWithTimeout tests SetGamSetGameModeWithTimeout functionality and also the tunings that come along with the change of game mode if specified.
// We will check the tuning of swappiness along with the change of game mode if checkSwappinessTuning is true;
// and we will check the tuning of transparent huge pages along with the change of game mode if checkTHPTuning is true.
func CheckSetGameModeWithTimeout(ctx context.Context, rm *resourced.Client, checkSwappinessTuning, checkTHPTuning bool) (resErr error) {
	var newGameMode uint8 = resourced.GameModeVM
	if err := rm.SetGameModeWithTimeout(ctx, newGameMode, 1); err != nil {
		return errors.Wrap(err, "failed to set game mode state")
	}
	testing.ContextLog(ctx, "Set game mode with 1 second timeout: ", newGameMode)
	if checkSwappinessTuning {
		if err := validateSwappiness(ctx, newGameMode); err != nil {
			return errors.Wrap(err, "validate swappiness failed")
		}
	}

	if checkTHPTuning {
		if err := validateTHP(ctx, newGameMode); err != nil {
			return errors.Wrap(err, "validate THP failed")
		}
	}

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

	// Check swappiness is reset after timeout.
	if checkSwappinessTuning {
		if err := validateSwappiness(ctx, resourced.GameModeOff); err != nil {
			return errors.Wrap(err, "Reset swapiness failed")
		}
	}

	if checkTHPTuning {
		if err := validateTHP(ctx, resourced.GameModeOff); err != nil {
			return errors.Wrap(err, "Reset THP failed")
		}
	}
	return nil
}
