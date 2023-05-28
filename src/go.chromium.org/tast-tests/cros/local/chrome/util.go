// Copyright 2017 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package chrome

import (
	"go.chromium.org/tast-tests/cros/local/chrome/internal/driver"
)

// PrepareForRestart prepares for Chrome restart.
//
// This function removes a debugging port file for a current Chrome process.
// By calling this function before purposefully restarting Chrome, you can
// reliably connect to a new Chrome process without accidentally connecting to
// an old Chrome process by a race condition.
func PrepareForRestart() error {
	return driver.PrepareForRestart()
}
