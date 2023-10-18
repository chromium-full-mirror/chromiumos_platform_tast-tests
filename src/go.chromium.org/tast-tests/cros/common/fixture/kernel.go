// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// Fixtures defined in src/go.chromium.org/tast-tests/cros/local/kernel/fixture.go
const (
	// HighResTimerOff is a fixture to turn off high res timer.
	HighResTimerOff = "highResTimerOff"
	// HighResTimerOffEnrolled is the same as HighResTimerOff with enrollment.
	HighResTimerOffEnrolled = "highResTimerOffEnrolled"
	// HighResTimerOff with gpuWatchHangs as parent.
	HighResTimerOffGpuWatchHangs = "highResTimerOffGpuWatchHangs"
)
