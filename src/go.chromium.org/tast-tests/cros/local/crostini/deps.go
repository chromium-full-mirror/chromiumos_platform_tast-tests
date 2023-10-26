// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crostini

import (
	"go.chromium.org/tast/core/framework/protocol"
	"go.chromium.org/tast/core/testing/hwdep"
)

// hwdepOr returns a hwdep.Condition which is the logical OR of two hwdep.Conditions.
func hwdepOr(lhs, rhs hwdep.Condition) hwdep.Condition {
	return hwdep.Condition{Satisfied: func(f *protocol.HardwareFeatures) (bool, string, error) {
		lhsSatisfied, lhsReason, err := lhs.Satisfied(f)
		if err != nil {
			return false, "", err
		}

		rhsSatisfied, rhsReason, err := rhs.Satisfied(f)
		if err != nil {
			return false, "", err
		}

		if lhsSatisfied || rhsSatisfied {
			return true, "", nil
		}

		return false, lhsReason + " AND " + rhsReason, nil
	}}
}
