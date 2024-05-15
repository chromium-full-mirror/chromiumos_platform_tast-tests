// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// Fixtures defined in go.chromium.org/tast-tests/cros/remote/updateutil/autoupdate_fixture.go.
const (
	// Fixture for autoupdate tests, ensures that the DUT stays on initially provisioned test image.
	Autoupdate = "autoupdate"
	// Fixture for forced autoupdate, copies implementation of Autoupdate, but removes ownership.
	ForcedAutoupdate = "forcedAutoupdate"
)
