// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"go.chromium.org/tast/core/testing/hwdep"
)

// Experimental8GBModelConditions returns the DUT model dependency conditions
// of experimental 8GB models.
func Experimental8GBModelConditions() []hwdep.Condition {
	return []hwdep.Condition{
		// The list of target models.
		hwdep.Model("redrix", "kano", "yaviks", "yavikso", "joxer", "screebo", "marasov", "karis"),
		// Target on 8GB devices.
		hwdep.MinMemory(7000), hwdep.MaxMemory(9000)}
}
