// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mousekeys provides functions to assist with interacting with the MouseKeys feature.
package mousekeys

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/a11y"
	"go.chromium.org/tast-tests/cros/local/input"
)

// PerformActionsForIdleTest performs a sequence of mouse movements forming a rough rectangle,
// alternating mouse buttons and performing clicks. No apps are launched
// as part of the clicking.
func PerformActionsForIdleTest(ctx context.Context, kb *input.KeyboardEventWriter) error {
	currentMouseButton := a11y.LeftMouseButton
	for i := 0; i < 7; i++ {
		actions := []a11y.MouseAction{
			a11y.MouseActionMoveRight, a11y.MouseActionMoveRight, a11y.MouseActionMoveRight,
			a11y.MouseActionMoveLeft, a11y.MouseActionMoveLeft, a11y.MouseActionMoveLeft,
			a11y.MouseActionMoveLeft, a11y.MouseActionMoveLeft, a11y.MouseActionMoveLeft,
			a11y.MouseActionMoveUp, a11y.MouseActionMoveUp, a11y.MouseActionMoveUp, a11y.MouseActionMoveUp,
			a11y.MouseActionMoveDown, a11y.MouseActionMoveDown,
			a11y.MouseActionSwitchMouseButton,
			a11y.MouseActionClick,
			a11y.MouseActionMoveLeft, a11y.MouseActionMoveLeft, a11y.MouseActionMoveLeft,
			a11y.MouseActionMoveLeft, a11y.MouseActionMoveLeft, a11y.MouseActionMoveLeft,
			a11y.MouseActionMoveRight, a11y.MouseActionMoveRight, a11y.MouseActionMoveRight,
			a11y.MouseActionMoveUp, a11y.MouseActionMoveUp,
			a11y.MouseActionClick,
			a11y.MouseActionSwitchMouseButton,
			a11y.MouseActionMoveRight, a11y.MouseActionClick,
			a11y.MouseActionSwitchMouseButton,
			a11y.MouseActionMoveLeft, a11y.MouseActionClick,
		}

		var err error
		for _, action := range actions {
			if action == a11y.MouseActionSwitchMouseButton {
				targetButton := a11y.RightMouseButton
				if currentMouseButton == a11y.RightMouseButton {
					targetButton = a11y.LeftMouseButton
				}
				if currentMouseButton, err = a11y.ChangeMouseButton(ctx, kb, currentMouseButton, targetButton); err != nil {
					return err
				}
			} else {
				if err := kb.Type(ctx, a11y.MouseActionToKeyboardKey(action)); err != nil {
					return err
				}
			}
		}
	}
	return nil
}
