// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package cellularconst defines the constants for Cellular
// This package is defined under common/ as they might be used in both
// local and remote tests.
package cellularconst

import "fmt"

// ModemType is the type of modem used in a device.
type ModemType uint32

// Supported modem types.
const (
	ModemTypeL850 ModemType = iota
	ModemTypeNL668
	ModemTypeFM350
	ModemTypeFM101
	ModemTypeSC7180 // trogdor
	ModemTypeSC7280 // herobrine
)

func (e ModemType) String() string {
	switch e {
	case ModemTypeL850:
		return "L850"
	case ModemTypeNL668:
		return "NL668"
	case ModemTypeFM350:
		return "FM350"
	case ModemTypeFM101:
		return "FM101"
	case ModemTypeSC7180:
		return "SC7180"
	case ModemTypeSC7280:
		return "SC7280"
	default:
		return fmt.Sprintf("%d", int(e))
	}
}
