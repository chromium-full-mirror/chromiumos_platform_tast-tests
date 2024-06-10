// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package bundleutil provides utilities for all bundles to use.
package bundleutil

import (
	"strconv"

	"go.chromium.org/tast/core/testing"
)

const (
	mib                      = 1024 * 1024
	defaultLowSpaceThreshold = 1000 // 1gb default.
)

var (
	lowSpaceThresholdVar = testing.RegisterVarString(
		"bundleutil.low_space_threshold",
		"1000",
		"The low space threshod in mb for the bundle to decide if purging data is needed",
	)
	lowSpaceThreshold uint64 = 0
)

// LowSpaceThreshold return the low space threshold for purging data.
func LowSpaceThreshold() uint64 {
	if lowSpaceThreshold == 0 {
		lowSpaceThreshold = defaultLowSpaceThreshold
		s := lowSpaceThresholdVar.Value()
		if v, err := strconv.ParseUint(s, 10, 64); err == nil {
			lowSpaceThreshold = v
		}
	}
	return lowSpaceThreshold * mib
}
