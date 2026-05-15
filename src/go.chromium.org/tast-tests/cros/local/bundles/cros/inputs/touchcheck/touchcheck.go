// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package touchcheck

import (
	"go.chromium.org/tast/core/framework/protocol"
	"go.chromium.org/tast/core/testing/hwdep"
)

// TouchscreenOrTouchpad returns a hardware dependency condition that is satisfied if the DUT has a built-in touchscreen or touchpad.
func TouchscreenOrTouchpad() hwdep.Condition {
	return hwdep.Condition{
		Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
			// Leverage Tast's built-in touchscreen check
			hasTouchScreen, _, errTouchScreen := hwdep.TouchScreen().Satisfied(f)
			if errTouchScreen != nil {
				return false, "Failed to check touchscreen", errTouchScreen
			}

			// Leverage Tast's built-in touchpad check
			hasTouchPad, _, errTouchPad := hwdep.Touchpad().Satisfied(f)
			if errTouchPad != nil {
				return false, "Failed to check touchpad", errTouchPad
			}

			if hasTouchScreen || hasTouchPad {
				return true, "", nil
			}
			return false, "DUT does not have a built-in touchscreen or touchpad", nil
		},
	}
}
