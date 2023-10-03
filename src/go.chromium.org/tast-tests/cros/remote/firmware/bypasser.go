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

type bypasser interface {
	// TriggerRecToDev moves the DUT from the recovery screen to
	// developer mode.
	TriggerRecToDev(ctx context.Context) error
	// TriggerDevToNormal moves the DUT from the developer warning screen
	// to normal mode.
	TriggerDevToNormal(ctx context.Context) error
	// BypassDevBootUSB bypasses the developer mode firmware logic to boot
	// from the USB.
	BypassDevBootUSB(ctx context.Context) error
	// BypassDevMode bypasses the developer mode firmware logic to boot
	// internal image.
	BypassDevMode(ctx context.Context) error
	// BypassAltfwMode bypasses the altfw-mode firmware logic to boot
	// the altfw image.
	BypassAltfwMode(ctx context.Context) error
}

type baseBypasser struct {
	helper    *Helper
	navigator menuNavigator
}

type keyboardBypasser struct {
	baseBypasser
}

func (k *keyboardBypasser) TriggerRecToDev(ctx context.Context) error {
	/*
		1. Press Ctrl-D to move to the confirm screen.
		2. Wait until the confirm screen appears.
		3. Push some button depending on the DUT's config:
		   toggle the rec button, press power, or press enter.
	*/
	h := k.helper
	testing.ContextLog(ctx, "Pressing CTRL-D")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab); err != nil {
		return err
	}
	testing.ContextLogf(ctx, "Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
	// GoBigSleepLint: Simulate a specific speed of key press.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return err
	}
	if h.Config.RecButtonDevSwitch {
		testing.ContextLog(ctx, "Toggling physical rec switch")
		if err := h.Servo.ToggleOnOff(ctx, servo.RecMode); err != nil {
			return err
		}
	} else if h.Config.PowerButtonDevSwitch {
		testing.ContextLog(ctx, "Pressing power key")
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
			return err
		}
	} else {
		if err := k.navigator.SelectOption(ctx); err != nil {
			return errors.Wrap(err, "while booting to dev mode")
		}
	}
	return nil
}

func (k *keyboardBypasser) TriggerDevToNormal(ctx context.Context) error {
	h := k.helper
	/*
		1. Press Ctrl+S.
		2. Sleep for [KeypressDelay] seconds.
		3. Press enter.
	*/
	testing.ContextLog(ctx, "Pressing Ctrl-S")
	if err := h.Servo.KeypressWithDuration(ctx, servo.CtrlS, servo.DurTab); err != nil {
		return errors.Wrap(err, "pressing Ctrl-S on firmware screen while disabling dev mode")
	}
	testing.ContextLogf(ctx, "Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
	// GoBigSleepLint: Simulate a specific speed of key press.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
	}
	if err := k.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "while disabling dev mode")
	}
	return nil
}

func (k *keyboardBypasser) BypassDevBootUSB(ctx context.Context) error {
	h := k.helper
	testing.ContextLog(ctx, "Pressing Ctrl-U")
	return h.Servo.KeypressWithDuration(ctx, servo.CtrlU, servo.DurTab)
}

func (k *keyboardBypasser) BypassDevMode(ctx context.Context) error {
	h := k.helper
	testing.ContextLog(ctx, "Pressing Ctrl-D")
	return h.Servo.KeypressWithDuration(ctx, servo.CtrlD, servo.DurTab)
}

func (k *keyboardBypasser) BypassAltfwMode(ctx context.Context) error {
	h := k.helper
	testing.ContextLog(ctx, "Pressing Ctrl-L")
	return h.Servo.ECPressCtrlKey(ctx, "l", h.Model)
}

type legacyKeyboardBypasser struct {
	keyboardBypasser
}

func (lk *legacyKeyboardBypasser) TriggerRecToDev(ctx context.Context) error {
	return lk.keyboardBypasser.TriggerRecToDev(ctx)
}

func (lk *legacyKeyboardBypasser) TriggerDevToNormal(ctx context.Context) error {
	h := lk.helper
	testing.ContextLog(ctx, "Pressing SPACE")
	if err := h.Servo.PressKey(ctx, " ", servo.DurTab); err != nil {
		return errors.Wrap(err, "pressing SPACE on firmware screen while disabling dev mode")
	}
	testing.ContextLogf(ctx, "Sleeping %s (KeypressDelay)", h.Config.KeypressDelay)
	// GoBigSleepLint: Simulate a specific speed of key press.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
	}
	if err := lk.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "while disabling dev mode")
	}
	return nil
}

func (lk *legacyKeyboardBypasser) BypassDevBootUSB(ctx context.Context) error {
	return lk.keyboardBypasser.BypassDevBootUSB(ctx)
}

func (lk *legacyKeyboardBypasser) BypassDevMode(ctx context.Context) error {
	return lk.keyboardBypasser.BypassDevMode(ctx)
}

func (lk *legacyKeyboardBypasser) BypassAltfwMode(ctx context.Context) error {
	testing.ContextLog(ctx, "legacy")
	return lk.keyboardBypasser.BypassAltfwMode(ctx)
}

type legacyDetachableBypasser struct {
	baseBypasser
}

func (ld *legacyDetachableBypasser) TriggerRecToDev(ctx context.Context) error {
	/*
		1. Hold both VolumeUp and VolumeDown for 200ms to trigger
		   TO_DEV screen.
		2. Wait [KeypressDelay] seconds to confirm keypress.
		3. Hold VolumeUp for 100ms to change menu selection
		   to 'Confirm enabling developer mode'.
		4. Wait [KeypressDelay] seconds to confirm keypress.
		5. Press PowerKey to select menu item.
		6. Wait [KeypressDelay] seconds to confirm keypress.
		7. Wait [FirmwareScreen] seconds to transition screens.
	*/
	h := ld.helper
	if err := h.Servo.SetInt(ctx, servo.VolumeUpDownHold, 200); err != nil {
		return errors.Wrap(err, "triggering TO_DEV screen")
	}
	// GoBigSleepLint: Simulate a specific speed of key press.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "sleeping for %s (KeypressDelay) to confirm triggering TO_DEV screen", h.Config.KeypressDelay)
	}
	if err := ld.navigator.MoveUp(ctx); err != nil {
		return errors.Wrap(err, "changing menu selection to 'Confirm enabling developer mode' on TO_DEV screen")
	}
	// GoBigSleepLint: Simulate a specific speed of key press.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "sleeping for %s (KeypressDelay) to confirm changing menu selection on TO_DEV screen", h.Config.KeypressDelay)
	}
	if err := ld.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "selecting menu item 'Confirm enabling developer mode' on TO_DEV screen")
	}
	return nil
}

func (ld *legacyDetachableBypasser) TriggerDevToNormal(ctx context.Context) error {
	/*
		1. Hold volume_up for 200ms to highlight the previous menu item,
		   'Enable Root Verification'.
		2. Sleep for [KeypressDelay] seconds to confirm keypress.
		3. Press power to select Enable Root Verification.
		4. Sleep for [KeypressDelay] seconds to confirm keypress.
		5. Wait until the TO_NORM screen appears.
		6. Press power to select Confirm Enabling Verified Boot.
	*/
	h := ld.helper
	if err := h.Servo.SetInt(ctx, servo.VolumeUpHold, 200); err != nil {
		return errors.Wrap(err, "changing menu selection to 'Enable Root Verification'")
	}
	// GoBigSleepLint: Simulate a specific speed of key press.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
	}
	if err := ld.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "selecting 'Enable Root Verification'")
	}
	// GoBigSleepLint: Simulate a specific speed of key press.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while disabling dev mode", h.Config.KeypressDelay)
	}
	if err := ld.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "selecting menu option 'Confirm Enabling Verified Boot'")
	}
	return nil
}

func (ld *legacyDetachableBypasser) BypassDevBootUSB(ctx context.Context) error {
	h := ld.helper
	testing.ContextLog(ctx, "Pressing and holding volume up button for 3 seconds")
	return h.Servo.SetInt(ctx, servo.VolumeUpHold, 3000)
}

func (ld *legacyDetachableBypasser) BypassDevMode(ctx context.Context) error {
	h := ld.helper
	testing.ContextLog(ctx, "Pressing and holding volume down button for 3 seconds")
	return h.Servo.SetInt(ctx, servo.VolumeDownHold, 3000)
}

func (ld *legacyDetachableBypasser) BypassAltfwMode(ctx context.Context) error {
	h := ld.helper
	// TODO(b:296600641): Is this the right method?
	testing.ContextLog(ctx, "Pressing and holding volume down button for 3 seconds")
	return h.Servo.SetInt(ctx, servo.VolumeDownHold, 3000)
}

// NewBypasser creates a new bypasser. It relies on a firmware Helper to
// identify the required class for the bypass logic, as well as other
// dependent objects.
func NewBypasser(ctx context.Context, h *Helper) (bypasser, error) {
	if err := h.RequireConfig(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to require config when creating new bypasser")
	}
	if err := h.RequireServo(ctx); err != nil {
		return nil, errors.Wrap(err, "requiring servo")
	}
	menuNavigator, err := NewMenuNavigator(ctx, h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create a new menu navigator")
	}
	newBaseBypasser := &baseBypasser{helper: h, navigator: menuNavigator}
	newKeyboardBypasser := &keyboardBypasser{baseBypasser: *newBaseBypasser}
	newLegacyKeyboardBypasser := &legacyKeyboardBypasser{keyboardBypasser: *newKeyboardBypasser}
	newLegacyDetachableBypasser := &legacyDetachableBypasser{baseBypasser: *newBaseBypasser}

	mapBypasserToModeSwitcherType := map[ModeSwitcherType]bypasser{
		KeyboardDevSwitcher:      newLegacyKeyboardBypasser,
		MenuSwitcher:             newKeyboardBypasser,
		TabletDetachableSwitcher: newLegacyDetachableBypasser,
	}
	if _, ok := mapBypasserToModeSwitcherType[h.Config.ModeSwitcherType]; !ok {
		return nil, errors.Errorf("unsupported mode switcher type: %s", h.Config.ModeSwitcherType)
	}
	return mapBypasserToModeSwitcherType[h.Config.ModeSwitcherType], nil
}
