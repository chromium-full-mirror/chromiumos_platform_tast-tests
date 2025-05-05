// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

// ModifiersStatus is a struct to present the all modifier keys status.
// True means enable, false means disable.
type ModifiersStatus struct {
	Shift bool
	Altgr bool
	Caps  bool
}

// LinuxKeyCode is a struct that maps Linux key code values to their string names.
// While some could be named after characters on the US-Qwerty layout,
// they refer to physical positions on standard hardware layouts, regardless of character mappings.
// When changing ime, we simulate key presses based on these codes.
// This allows us to correctly handle language-specific keyboard layouts.
// For example, with a French (France) InputMethod:
//   - Pressing the Q key, which key code is input_KEY_Q, it would insert the letter "A".
type LinuxKeyCode struct {
	LinuxKeyCode input.EventCode
	KeyName      string
}

// LinuxKeyCodes is a list LinuxKeyCode.
var LinuxKeyCodes = []LinuxKeyCode{
	{
		LinuxKeyCode: input.KEY_1,
		KeyName:      "KEY_1",
	},
	{
		LinuxKeyCode: input.KEY_2,
		KeyName:      "KEY_2",
	},
	{
		LinuxKeyCode: input.KEY_3,
		KeyName:      "KEY_3",
	},
	{
		LinuxKeyCode: input.KEY_4,
		KeyName:      "KEY_4",
	},
	{
		LinuxKeyCode: input.KEY_5,
		KeyName:      "KEY_5",
	},
	{
		LinuxKeyCode: input.KEY_6,
		KeyName:      "KEY_6",
	},
	{
		LinuxKeyCode: input.KEY_7,
		KeyName:      "KEY_7",
	},
	{
		LinuxKeyCode: input.KEY_8,
		KeyName:      "KEY_8",
	},
	{
		LinuxKeyCode: input.KEY_9,
		KeyName:      "KEY_9",
	},
	{
		LinuxKeyCode: input.KEY_0,
		KeyName:      "KEY_0",
	},
	{
		LinuxKeyCode: input.KEY_MINUS,
		KeyName:      "KEY_MINUS",
	},
	{
		LinuxKeyCode: input.KEY_EQUAL,
		KeyName:      "KEY_EQUAL",
	},
	{
		LinuxKeyCode: input.KEY_Q,
		KeyName:      "KEY_Q",
	},
	{
		LinuxKeyCode: input.KEY_W,
		KeyName:      "KEY_W",
	},
	{
		LinuxKeyCode: input.KEY_E,
		KeyName:      "KEY_E",
	},
	{
		LinuxKeyCode: input.KEY_R,
		KeyName:      "KEY_R",
	},
	{
		LinuxKeyCode: input.KEY_T,
		KeyName:      "KEY_T",
	},
	{
		LinuxKeyCode: input.KEY_Y,
		KeyName:      "KEY_Y",
	},
	{
		LinuxKeyCode: input.KEY_U,
		KeyName:      "KEY_U",
	},
	{
		LinuxKeyCode: input.KEY_I,
		KeyName:      "KEY_I",
	},
	{
		LinuxKeyCode: input.KEY_O,
		KeyName:      "KEY_O",
	},
	{
		LinuxKeyCode: input.KEY_P,
		KeyName:      "KEY_P",
	},
	{
		LinuxKeyCode: input.KEY_LEFTBRACE,
		KeyName:      "KEY_LEFTBRACE",
	},
	{
		LinuxKeyCode: input.KEY_RIGHTBRACE,
		KeyName:      "KEY_RIGHTBRACE",
	},
	{
		LinuxKeyCode: input.KEY_A,
		KeyName:      "KEY_A",
	},
	{
		LinuxKeyCode: input.KEY_S,
		KeyName:      "KEY_S",
	},
	{
		LinuxKeyCode: input.KEY_D,
		KeyName:      "KEY_D",
	},
	{
		LinuxKeyCode: input.KEY_F,
		KeyName:      "KEY_F",
	},
	{
		LinuxKeyCode: input.KEY_G,
		KeyName:      "KEY_G",
	},
	{
		LinuxKeyCode: input.KEY_H,
		KeyName:      "KEY_H",
	},
	{
		LinuxKeyCode: input.KEY_J,
		KeyName:      "KEY_J",
	},
	{
		LinuxKeyCode: input.KEY_K,
		KeyName:      "KEY_K",
	},
	{
		LinuxKeyCode: input.KEY_L,
		KeyName:      "KEY_L",
	},
	{
		LinuxKeyCode: input.KEY_SEMICOLON,
		KeyName:      "KEY_SEMICOLON",
	},
	{
		LinuxKeyCode: input.KEY_APOSTROPHE,
		KeyName:      "KEY_APOSTROPHE",
	},
	{
		LinuxKeyCode: input.KEY_GRAVE,
		KeyName:      "KEY_GRAVE",
	},
	{
		LinuxKeyCode: input.KEY_BACKSLASH,
		KeyName:      "KEY_BACKSLASH",
	},
	{
		LinuxKeyCode: input.KEY_Z,
		KeyName:      "KEY_Z",
	},
	{
		LinuxKeyCode: input.KEY_X,
		KeyName:      "KEY_X",
	},
	{
		LinuxKeyCode: input.KEY_C,
		KeyName:      "KEY_C",
	},
	{
		LinuxKeyCode: input.KEY_V,
		KeyName:      "KEY_V",
	},
	{
		LinuxKeyCode: input.KEY_B,
		KeyName:      "KEY_B",
	},
	{
		LinuxKeyCode: input.KEY_N,
		KeyName:      "KEY_N",
	},
	{
		LinuxKeyCode: input.KEY_M,
		KeyName:      "KEY_M",
	},
	{
		LinuxKeyCode: input.KEY_COMMA,
		KeyName:      "KEY_COMMA",
	},
	{
		LinuxKeyCode: input.KEY_DOT,
		KeyName:      "KEY_DOT",
	},
	{
		LinuxKeyCode: input.KEY_SLASH,
		KeyName:      "KEY_SLASH",
	},
	{
		LinuxKeyCode: input.KEY_102ND,
		KeyName:      "KEY_102ND", // intl backslash
	},
	{
		LinuxKeyCode: input.KEY_RO,
		KeyName:      "KEY_RO",
	},
	{
		LinuxKeyCode: input.KEY_YEN,
		KeyName:      "KEY_YEN",
	},
	{
		LinuxKeyCode: input.KEY_SPACE,
		KeyName:      "KEY_SPACE",
	},
}

// ModifiersStatusCombo is set of all modifier keys combonation.
var ModifiersStatusCombo = []ModifiersStatus{
	{
		Shift: false,
		Altgr: false,
		Caps:  false,
	},
	{
		Shift: false,
		Altgr: false,
		Caps:  true,
	},
	{
		Shift: false,
		Altgr: true,
		Caps:  false,
	},
	{
		Shift: false,
		Altgr: true,
		Caps:  true,
	},
	{
		Shift: true,
		Altgr: false,
		Caps:  false,
	},
	{
		Shift: true,
		Altgr: false,
		Caps:  true,
	},
	{
		Shift: true,
		Altgr: true,
		Caps:  false,
	},
	{
		Shift: true,
		Altgr: true,
		Caps:  true,
	},
}

// SingleKeyAction return the action for pressing different modifier with key-1.
func SingleKeyAction(needEsc, useShortcutForCapslock bool, modifiers ModifiersStatus, key input.EventCode, kb *input.KeyboardEventWriter) action.Action {
	return uiauto.Combine("click key based on modifers state",
		ifThen(needEsc, kb.TypeKeyAction(input.KEY_ESC)),
		ifThen(modifiers.Caps, capslockAction(useShortcutForCapslock, kb)),
		ifThen(modifiers.Shift, kb.AccelPressAction("Shift")),
		ifThen(modifiers.Altgr, kb.AccelPressAction("RightAlt")),
		kb.TypeKeyAction(key),
		ifThen(modifiers.Altgr, kb.AccelReleaseAction("RightAlt")),
		ifThen(modifiers.Shift, kb.AccelReleaseAction("Shift")),
		ifThen(modifiers.Caps, capslockAction(useShortcutForCapslock, kb)),
	)
}

// TwoKeysAction return the action for pressing different modifier with key-1 and key-2.
func TwoKeysAction(needEsc, useShortcutForCapslock bool, modifiers1, modifiers2 ModifiersStatus, key1, key2 input.EventCode, kb *input.KeyboardEventWriter) action.Action {
	return uiauto.Combine("click key based on modifers state",
		ifThen(needEsc, kb.TypeKeyAction(input.KEY_ESC)),

		ifThen(modifiers1.Caps, capslockAction(useShortcutForCapslock, kb)),
		ifThen(modifiers1.Shift, kb.AccelPressAction("Shift")),
		ifThen(modifiers1.Altgr, kb.AccelPressAction("RightAlt")),
		kb.TypeKeyAction(key1),
		ifThen(modifiers1.Altgr && !modifiers2.Altgr, kb.AccelReleaseAction("RightAlt")),
		ifThen(modifiers1.Shift && !modifiers2.Shift, kb.AccelReleaseAction("Shift")),
		ifThen(modifiers1.Caps && !modifiers2.Caps, capslockAction(useShortcutForCapslock, kb)),

		ifThen(!modifiers1.Caps && modifiers2.Caps, capslockAction(useShortcutForCapslock, kb)),
		ifThen(!modifiers1.Shift && modifiers2.Shift, kb.AccelPressAction("Shift")),
		ifThen(!modifiers1.Altgr && modifiers2.Altgr, kb.AccelPressAction("RightAlt")),
		kb.TypeKeyAction(key2),
		ifThen(modifiers2.Altgr, kb.AccelReleaseAction("RightAlt")),
		ifThen(modifiers2.Shift, kb.AccelReleaseAction("Shift")),
		ifThen(modifiers2.Caps, capslockAction(useShortcutForCapslock, kb)),
	)
}

func capslockAction(useShortcut bool, kb *input.KeyboardEventWriter) action.Action {
	// Alt+Search should be equivalent to Capslock, but unlike real Capslock
	// it unexpectedly disrupts dead-key composition (crbug/383673473), so
	// such shortcut should be avoided unless absolutely necessary.
	if useShortcut {
		return kb.AccelAction("Alt+Search")
	}
	return kb.TypeKeyAction(input.KEY_CAPSLOCK)
}

func ifThen(condition bool, action action.Action) uiauto.Action {
	return uiauto.IfSuccessThen(
		func(ctx context.Context) error {
			if condition {
				return nil
			}
			return errors.New("intended error")
		},
		action,
	)
}
