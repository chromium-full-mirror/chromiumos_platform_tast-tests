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

type menuBypasser interface {
	TriggerRecToDev(ctx context.Context) error
	BypassDevMode(ctx context.Context) error
	BypassDevBootUSB(ctx context.Context) error
	TriggerDevToNormal(ctx context.Context) error
	PowerOff(ctx context.Context, waitForFirmwareScreen bool) error
	BypassDevDefaultBoot(ctx context.Context) error
}

type baseMenuBypasser struct {
	helper    *Helper
	navigator menuNavigator
}

const recoveryMenuItemCount = 7

func newBaseMenuBypasser(ctx context.Context, h *Helper) (baseMenuBypasser, error) {
	if err := h.RequireConfig(ctx); err != nil {
		return baseMenuBypasser{}, errors.Wrap(err, "requiring new configs")
	}
	nvg, err := NewMenuNavigator(ctx, h)
	if err != nil {
		return baseMenuBypasser{}, errors.Wrap(err, "creating a new navigator")
	}
	return baseMenuBypasser{
		helper:    h,
		navigator: nvg,
	}, nil
}

// Bypasser with menu navigator for legacy menu UI.
// The "legacy menu UI" is an old menu-based UI, which has been replaced
// by the new one, called "menu UI".
type legacyMenuBypasser struct {
	baseMenuBypasser
}

// TriggerRecToDev triggers to-dev transition.
func (lmb *legacyMenuBypasser) TriggerRecToDev(ctx context.Context) error {
	h := lmb.helper
	bypasser, err := NewBypasser(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create a new bypasser")
	}
	return bypasser.TriggerRecToDev(ctx)
}

// BypassDevMode boots from internal disk in developer screen.
func (lmb *legacyMenuBypasser) BypassDevMode(ctx context.Context) error {
	// Menu items in developer warning screen:
	// 	0. Developer Options
	// 	1. Show Debug Info
	// 	2. Enable OS Verification
	// 	*3. Power Off
	// 	4. Language

	// Menu items in developer boot options screen:
	// 	0. Boot From Network
	// 	1. Boot Legacy BIOS
	// 	2. Boot From USB or SD Card
	// 	*3. Boot From Internal Disk
	// 	4. Cancel
	// 	5. Power Off
	h := lmb.helper
	if err := MoveTo(ctx, h, lmb.navigator, 3, 0); err != nil {
		return err
	}
	if err := lmb.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Developer Options\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := lmb.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Boot From Internal Disk\"")
	}
	return nil
}

// BypassDevBootUSB boots from external disk in developer screen.
func (lmb *legacyMenuBypasser) BypassDevBootUSB(ctx context.Context) error {
	// Menu items in developer warning screen:
	// 	0. Developer Options
	// 	1. Show Debug Info
	// 	2. Enable OS Verification
	// 	*3. Power Off
	// 	4. Language

	// Menu items in developer boot options screen:
	// 	0. Boot From Network
	// 	1. Boot Legacy BIOS
	// 	2. Boot From USB or SD Card
	// 	*3. Boot From Internal Disk
	// 	4. Cancel
	// 	5. Power Off
	// 	6. Language
	h := lmb.helper
	if err := MoveTo(ctx, h, lmb.navigator, 3, 0); err != nil {
		return err
	}
	if err := lmb.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Developer Options\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := MoveTo(ctx, h, lmb.navigator, 3, 2); err != nil {
		return err
	}
	if err := lmb.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Boot From USB or SD Card\"")
	}
	return nil
}

// TriggerDevToNormal triggers dev-to-norm transition.
func (lmb *legacyMenuBypasser) TriggerDevToNormal(ctx context.Context) error {
	// Menu items in developer warning screen:
	// 	0. Developer Options
	// 	1. Show Debug Info
	// 	2. Enable OS Verification
	// 	*3. Power Off
	// 	4. Language

	// Menu items in to-norm confirmation screen:
	// 	*0. Confirm Enabling OS Verification
	// 	1. Cancel
	// 	2. Power Off
	// 	3. Language
	// (*) is the default selection.
	h := lmb.helper
	if err := MoveTo(ctx, h, lmb.navigator, 3, 2); err != nil {
		return err
	}
	if err := lmb.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Enable OS Verification\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := lmb.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Confirm Enabling OS Verification\"")
	}
	return nil
}

// PowerOff powers off the device.
// This method should work in both developer and recovery screens.
func (lmb *legacyMenuBypasser) PowerOff(ctx context.Context, waitForFirmwareScreen bool) error {
	h := lmb.helper
	if waitForFirmwareScreen {
		testing.ContextLogf(ctx, "Waiting for %s (firmware screen)", h.Config.FirmwareScreen)
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			return errors.Wrapf(err, "failed to wait for %s", h.Config.FirmwareScreen)
		}
	}
	if err := lmb.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Power Off\"")
	}
	return nil
}

// BypassDevDefaultBoot selects the firmware menu option to boot from
// default target.
func (lmb *legacyMenuBypasser) BypassDevDefaultBoot(ctx context.Context) error {
	// Menu options seen in DEVELOPER WARNING screen:
	//	0. Developer Options
	//	1. Show Debug Info
	//	2. Enable Root Verification
	//	*3. Power Off
	//	4. Language
	//	(*) is the default selection.

	// Menu options seen in DEV screen:
	//	0. Boot legacy BIOS*      (default if dev_default_boot=legacy)
	//	1. Boot USB image*        (default if dev_default_boot=usb)
	//	2. Boot developer image*  (default if dev_default_boot=disk)
	//	3. Cancel
	//	4. Power off
	//	5. Language
	h := lmb.helper
	// Transition from DEVELOPER WARNING to DEV screen.
	if err := MoveTo(ctx, h, lmb.navigator, 3, 0); err != nil {
		return err
	}
	if err := lmb.navigator.SelectOption(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Simulate a specific speed of key press.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "sleeping for %s (KeypressDelay) while at developer screen", h.Config.KeypressDelay)
	}
	testing.ContextLog(ctx, "Selecting default option on developer screen")
	return lmb.navigator.SelectOption(ctx)
}

// menuOperator performs menu operations for menu UI.
// The "menu UI" aims to replace both "legacy clamshell UI" and "legacy
// menu UI".

// Menu items in recovery select screen:
// 	0. Language
// 	1. Recovery using phone (always hidden)
// 	2. Recovery using external disk
// 	3. Recovery using internet connection (shown if minios_enabled)
// 	4. Launch diagnostics (shown if minidiag_enabled)
// 	5. Advanced options
// 	6. Power off
type menuOperator struct {
	baseMenuBypasser
	miniDiagEnabled bool
	miniOSEnabled   bool
}

// NewMenuOperator creates a new menuOperator.
func NewMenuOperator(ctx context.Context, h *Helper) (menuOperator, error) {
	newBaseMenuBypasser, err := newBaseMenuBypasser(ctx, h)
	if err != nil {
		return menuOperator{}, errors.Wrap(err, "failed to create baseMenuBypasser")
	}
	if h.Config.ModeSwitcherType != MenuSwitcher {
		return menuOperator{}, errors.Errorf("menuOperator unsupported on %s", h.Config.ModeSwitcherType)
	}

	return menuOperator{
		baseMenuBypasser: newBaseMenuBypasser,
		miniDiagEnabled:  h.Config.MiniDiagEnabled,
		miniOSEnabled:    h.Config.MiniOSEnabled,
	}, nil
}

func (mo *menuOperator) confirmToDev(ctx context.Context) error {
	h := mo.helper
	if h.Config.RecButtonDevSwitch {
		return h.Servo.ToggleOnOff(ctx, servo.RecMode)
	} else if h.Config.PowerButtonDevSwitch {
		return h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab)
	}
	return mo.navigator.SelectOption(ctx)
}

// TriggerRecToDev enables dev mode from recovery screen.
func (mo *menuOperator) TriggerRecToDev(ctx context.Context) error {
	// Menu items in advanced options screen:
	// 	0. Language
	// 	*1. Enable developer mode
	// 	2. Back
	// 	3. Power off

	// Menu items in to-dev screen:
	// 	0. Language
	// 	*1. Confirm
	// 	2. Cancel
	// 	3. Power off
	// (*) is the default selection.
	h := mo.helper
	// The default selection is unknown, so navigate to the last item first.
	if err := MoveTo(ctx, h, mo.navigator, 0, recoveryMenuItemCount); err != nil {
		return err
	}
	if err := mo.navigator.MoveUp(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Advanced Options\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Enable Developer Mode\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.confirmToDev(ctx); err != nil {
		return errors.Wrap(err, "failed to confirm to dev")
	}
	return nil
}

// BypassDevMode boots from internal disk in developer screen.
func (mo *menuOperator) BypassDevMode(ctx context.Context) error {
	// Menu items in developer mode screen:
	// 	0. Language
	// 	1. Return to secure mode
	// 	2. Boot from internal disk
	// 	3. Boot from external disk
	// 	4. Advanced options
	// 	5. Power off
	h := mo.helper
	if err := MoveTo(ctx, h, mo.navigator, 5, 0); err != nil {
		return err
	}
	if err := MoveTo(ctx, h, mo.navigator, 0, 2); err != nil {
		return err
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Boot From Internal Disk\"")
	}
	return nil
}

// BypassDevBootUSB boots from external disk in developer screen.
func (mo *menuOperator) BypassDevBootUSB(ctx context.Context) error {
	// Menu items in developer mode screen:
	// 	0. Language
	// 	1. Return to secure mode
	// 	2. Boot from internal disk
	// 	3. Boot from external disk
	// 	4. Advanced options
	// 	5. Power off
	h := mo.helper
	if err := MoveTo(ctx, h, mo.navigator, 5, 0); err != nil {
		return err
	}
	if err := MoveTo(ctx, h, mo.navigator, 0, 3); err != nil {
		return err
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Boot From External Disk\"")
	}
	return nil
}

// TriggerDevToNormal triggers dev-to-norm transition.
func (mo *menuOperator) TriggerDevToNormal(ctx context.Context) error {
	// Menu items in developer mode screen:
	// 	0. Language
	// 	1. Return to secure mode
	// 	2. Boot from internal disk
	// 	3. Boot from external disk
	// 	4. Advanced options
	// 	5. Power off

	// Menu items in to-norm screen:
	// 	0. Language
	// 	*1. Confirm
	// 	2. Cancel
	// 	3. Power off
	// (*) is the default selection.
	h := mo.helper
	if err := MoveTo(ctx, h, mo.navigator, 5, 0); err != nil {
		return err
	}
	if err := mo.navigator.MoveDown(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Return to secure mode\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select `Confirm`")
	}
	return nil
}

// PowerOff powers off the device.
// This method should work in both developer and recovery screens.
func (mo *menuOperator) PowerOff(ctx context.Context, waitForFirmwareScreen bool) error {
	h := mo.helper
	if waitForFirmwareScreen {
		testing.ContextLogf(ctx, "Waiting for %s (firmware screen)", h.Config.FirmwareScreen)
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
			return errors.Wrapf(err, "failed to wait for %s", h.Config.FirmwareScreen)
		}
	}
	if err := MoveTo(ctx, h, mo.navigator, 0, 6); err != nil {
		return err
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Power off\"")
	}
	return nil
}

// TriggerRecToMinidiag triggers diagnostic boot from recovery screen.
func (mo *menuOperator) TriggerRecToMinidiag(ctx context.Context) error {
	h := mo.helper
	if !mo.miniDiagEnabled {
		return errors.New("miniDiag is not enabled for this board")
	}
	if err := MoveTo(ctx, h, mo.navigator, 0, recoveryMenuItemCount); err != nil {
		return err
	}
	if err := mo.navigator.MoveUp(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.MoveUp(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Launch diagnostics\"")
	}
	testing.ContextLogf(ctx, "Waiting for %s (firmware screen)", h.Config.FirmwareScreen)
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.FirmwareScreen); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.FirmwareScreen)
	}
	return nil
}

// NavigateMinidiagStorage navigates to storage screen.
func (mo *menuOperator) NavigateMinidiagStorage(ctx context.Context) error {
	// Menu items in storage screen:
	// 	0. Language
	// 	1. Page up (disabled)
	// 	2. Page down
	// 	3. Back
	// 	4. Power off
	h := mo.helper
	if !mo.miniDiagEnabled {
		return errors.New("miniDiag is not enabled for this board")
	}
	if err := MoveTo(ctx, h, mo.navigator, 5, 0); err != nil {
		return err
	}
	if err := mo.navigator.MoveDown(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Storage (Health) Info\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := MoveTo(ctx, h, mo.navigator, 0, 4); err != nil {
		return err
	}
	if err := mo.navigator.MoveUp(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to back to MiniDiag root screen")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	return nil
}

// NavigateMinidiagQuickMemoryCheck navigates to quick memory test screen.
func (mo *menuOperator) NavigateMinidiagQuickMemoryCheck(ctx context.Context) error {
	// Menu items in quick memory test screen:
	// 	0. Language
	// 	1. Page up (disabled)
	// 	2. Page down (disabled)
	// 	3. Back
	// 	4. Power off
	h := mo.helper
	if !mo.miniDiagEnabled {
		return errors.New("miniDiag is not enabled for this board")
	}
	if err := MoveTo(ctx, h, mo.navigator, 0, 5); err != nil {
		return err
	}
	if err := mo.navigator.MoveUp(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.MoveUp(ctx); err != nil {
		return err
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to select \"Quick memory test\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	if err := mo.navigator.SelectOption(ctx); err != nil {
		return errors.Wrap(err, "failed to back to \"MiniDiag root screen\"")
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
	}
	return nil
}

// TriggerRecToMinios triggers internet recovery from recovery screen.
func (mo *menuOperator) TriggerRecToMiniOS(ctx context.Context, olderVersion bool) error {
	// Menu items in advanced options screen, developer mode:
	// 	0. Language
	// 	*1. Debug info
	// 	2. Firmware log
	// 	3. Internet recovery (older version)
	// 	4. Back
	// 	5. Power off
	// (*) is the default selection.
	h := mo.helper
	if !mo.miniOSEnabled {
		return errors.New("this function should not be implemented")
	}
	if olderVersion {
		testing.ContextLog(ctx, "Boot to MiniOS (older version)")
		// The default selection is unknown, so navigate to the last item first.
		if err := MoveTo(ctx, h, mo.navigator, 0, recoveryMenuItemCount); err != nil {
			return err
		}
		if err := mo.navigator.MoveUp(ctx); err != nil {
			return err
		}
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
		}
		if err := mo.navigator.SelectOption(ctx); err != nil {
			return errors.Wrap(err, "failed to select \"Advanced options\"")
		}
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
		}
		if err := MoveTo(ctx, h, mo.navigator, 0, 5); err != nil {
			return err
		}
		if err := MoveTo(ctx, h, mo.navigator, 5, 3); err != nil {
			return err
		}
		if err := mo.navigator.SelectOption(ctx); err != nil {
			return errors.Wrap(err, "failed to select \"Internet recovery (older version)\"")
		}
	} else {
		testing.ContextLog(ctx, "Booting to MiniOS")
		if err := mo.navigator.MoveDown(ctx); err != nil {
			return err
		}
		// GoBigSleepLint: Sleep for model specific time.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrapf(err, "failed to wait for %s", h.Config.KeypressDelay)
		}
		if err := mo.navigator.SelectOption(ctx); err != nil {
			return errors.Wrap(err, "failed to select \"Recovery using internet connection\"")
		}
	}
	// GoBigSleepLint: Sleep for model specific time.
	if err := testing.Sleep(ctx, h.Config.MiniOSScreen); err != nil {
		return errors.Wrapf(err, "failed to wait for %s", h.Config.MiniOSScreen)
	}
	return nil
}

// BypassDevDefaultBoot selects the firmware menu option to boot from
// default target.
func (mo *menuOperator) BypassDevDefaultBoot(ctx context.Context) error {
	testing.ContextLog(ctx, "Selecting default option on developer screen")
	return mo.navigator.SelectOption(ctx)
}

// NewMenuBypasser creates a new menuBypasser. It relies on a firmware
// Helper to identify the required class for the menu bypass logic,
// as well as other dependent objects.
func NewMenuBypasser(ctx context.Context, h *Helper) (menuBypasser, error) {
	newBaseMenuBypasser, err := newBaseMenuBypasser(ctx, h)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create newBaseMenuBypasser")
	}
	if err := h.RequireServo(ctx); err != nil {
		return nil, errors.Wrap(err, "requiring servo")
	}
	switch h.Config.ModeSwitcherType {
	case MenuSwitcher:
		return &menuOperator{
			baseMenuBypasser: newBaseMenuBypasser,
		}, nil
	case TabletDetachableSwitcher:
		return &legacyMenuBypasser{
			baseMenuBypasser: newBaseMenuBypasser,
		}, nil
	default:
		return nil, errors.Errorf("unsupported switcher type: %v", h.Config.ModeSwitcherType)
	}
}
