// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// Fixtures defined in go.chromium.org/tast-tests/cros/remote/updateutil/updateengine_fixture.go.
const (
	// Fixture for tests that use update engine. Ensures that update engine status
	// is reset at the end of the test.
	UpdateEngine = "updateEngine"
	// UpdateEngineEnrolled provides enrollment as well.
	UpdateEngineEnrolled = "updateEngineEnrolled"
	// UpdateEngineCleanOwnership provides clean ownership and takes care of status for
	// update engine.
	UpdateEngineCleanOwnership = "updateEngineCleanOwnership"
)
