// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package constant contains common variables for mantis tast tests.
package constant

import "time"

// Mantis related constant
const (
	ImageTestFileName       = "a_horse_20241127.png"
	UnsafeImageTestFileName = "unsafe_image_20250116.jpg"
	MantisDLCID             = "ml-dlc-302a455f-5453-43fb-a6a1-d856e6fe6435"
	PowerMetricInterval     = 5 * time.Second
	// DLC download might take up to 20 minutes.
	PowerTestTimeout                    = 20 * time.Minute
	DefaultTestTimeout                  = 20 * time.Minute
	DefaultUITimeout                    = 5 * time.Second
	DefaultImageDiffPercentageThreshold = float64(3)
)
