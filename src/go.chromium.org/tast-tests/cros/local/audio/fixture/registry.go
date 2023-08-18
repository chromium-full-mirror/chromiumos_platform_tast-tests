// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

import (
	"go.chromium.org/tast/core/testing"
)

var registeredFixtures = map[string]bool{}

// maybeRegisterFixture registers the fixture f if it is not registered yet.
// Returns f.Name.
func maybeRegisterFixture(f *testing.Fixture) string {
	if !registeredFixtures[f.Name] {
		registeredFixtures[f.Name] = true
		testing.AddFixture(f)
	}
	return f.Name
}
