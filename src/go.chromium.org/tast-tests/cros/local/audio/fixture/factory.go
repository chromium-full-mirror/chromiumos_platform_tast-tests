// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// ParameterizedFixture is an interface for fixtures that can be parameterized.
type ParameterizedFixture interface {
	// Instance returns the instance name of the fixture instance for this
	// parameterized fixture.
	Instance() string
}
