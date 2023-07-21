// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type menuNavigator interface {
	MoveUp(ctx context.Context) error
	MoveDown(ctx context.Context) error
	SelectOption(ctx context.Context) error
}

type keyboardMenuNavigator struct {
	helper *Helper
}

func (kbMenuNav *keyboardMenuNavigator) MoveUp(ctx context.Context) error {
	h := kbMenuNav.helper
	testing.ContextLogf(ctx, "Moving up with %s", servo.ArrowUp)
	if err := h.Servo.KeypressWithDuration(ctx, servo.ArrowUp, servo.DurTab); err != nil {
		return errors.Wrapf(err, "failed to move up with %s", servo.ArrowUp)
	}
	return nil
}

func (kbMenuNav *keyboardMenuNavigator) MoveDown(ctx context.Context) error {
	h := kbMenuNav.helper
	testing.ContextLogf(ctx, "Moving down with %s", servo.ArrowDown)
	if err := h.Servo.KeypressWithDuration(ctx, servo.ArrowDown, servo.DurTab); err != nil {
		return errors.Wrapf(err, "failed to move down with %s", servo.ArrowDown)
	}
	return nil
}

func (kbMenuNav *keyboardMenuNavigator) SelectOption(ctx context.Context) error {
	h := kbMenuNav.helper
	testing.ContextLogf(ctx, "Hitting %s to select firmware menu option", servo.Enter)
	if err := h.Servo.KeypressWithDuration(ctx, servo.Enter, servo.DurTab); err != nil {
		return errors.Wrapf(err, "failed to select menu option with %s", servo.Enter)
	}
	return nil
}

type detachableMenuNavigator struct {
	helper *Helper
}

func (detMenuNav *detachableMenuNavigator) MoveUp(ctx context.Context) error {
	h := detMenuNav.helper
	testing.ContextLogf(ctx, "Moving up with %s", servo.VolumeUpHold)
	if err := h.Servo.SetInt(ctx, servo.VolumeUpHold, 100); err != nil {
		return errors.Wrapf(err, "failed to move up with %s", servo.VolumeUpHold)
	}
	return nil
}

func (detMenuNav *detachableMenuNavigator) MoveDown(ctx context.Context) error {
	h := detMenuNav.helper
	testing.ContextLogf(ctx, "Moving down with %s", servo.VolumeDownHold)
	if err := h.Servo.SetInt(ctx, servo.VolumeDownHold, 100); err != nil {
		return errors.Wrapf(err, "failed to move down with %s", servo.VolumeDownHold)
	}
	return nil
}

func (detMenuNav *detachableMenuNavigator) SelectOption(ctx context.Context) error {
	h := detMenuNav.helper
	testing.ContextLogf(ctx, "Hitting %s to select firmware menu option", servo.PowerKey)
	if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
		return errors.Wrapf(err, "failed to select menu option with %s", servo.PowerKey)
	}
	return nil
}

// MoveTo accepts 'fromIdx' and 'toIdx', which correspond with the locations of
// firmware menu options, and traverses the menu up or down.
func MoveTo(ctx context.Context, h *Helper, navigate menuNavigator, fromIdx, toIdx int) error {
	method := navigate.MoveDown
	inc := 1
	if fromIdx > toIdx {
		method = navigate.MoveUp
		inc = -1
	}

	for fromIdx != toIdx {
		if err := method(ctx); err != nil {
			return err
		}
		// GoBigSleepLint: Simulate a specific speed of key presses.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		fromIdx += inc
	}
	return nil
}

// NewMenuNavigator creates a new menuNavigator. It relies on a firmware Helper to
// identify the required class for navigation, as well as other dependent objects.
func NewMenuNavigator(ctx context.Context, h *Helper) (menuNavigator, error) {
	if err := h.RequireConfig(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to require config when creating menu navigator")
	}
	if h.Config.IsDetachable {
		return &detachableMenuNavigator{helper: h}, nil
	}
	return &keyboardMenuNavigator{helper: h}, nil
}
