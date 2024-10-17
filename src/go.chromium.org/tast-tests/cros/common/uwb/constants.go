// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package uwb is the supporting library that facilitates usage of UWB in CrOS testing.
package uwb

// Constants for UWB tests
const (
	DistanceAcceptableErrorMargin = uint32(10)
	AngleAcceptableErrorMargin    = float64(5.0)
	AngleAcceptableMinFOM         = 90
	DistanceMinAcceptablePercent  = 95
	AngleMinAcceptablePercent     = 95
)
