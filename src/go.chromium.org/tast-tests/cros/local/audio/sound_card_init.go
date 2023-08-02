// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package audio

import "go.chromium.org/tast/core/testing/hwdep"

// SoundCardInitConditions returns conditions for sound_card_init to run successfully.
func SoundCardInitConditions() []hwdep.Condition {
	return []hwdep.Condition{
		hwdep.SmartAmp(),
		hwdep.SkipOnModel("atlas", "nocturne", "volteer2", "helios",
			// TODO(b/283089078)
			"geralt"),
	}
}
