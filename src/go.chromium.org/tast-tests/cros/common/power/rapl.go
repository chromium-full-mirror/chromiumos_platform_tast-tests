// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

// Categories of rapl measurements.
const (
	Package0 = "package-0"
	Core     = "core"
	Uncore   = "uncore"
	Dram     = "dram"

	// Note: psys is not supported on ChromeOS. It is ignored if it appears
	// in any RAPL files.
	Psys = "psys"
)
