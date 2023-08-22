// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// Fixtured defined in go.chromium.org/tast-tests/cros/remote/policyutil/desk_api_*_fixture.go
const (
	// Enrolled desk API feature using TAPE leased account for lacros.
	DeskAPILacros = "deskAPILacros"
	// Enrolled desk API feature using TAPE leased account for ash.
	DeskAPIAsh = "deskAPIAsh"
)

// DeskFixtData holds the data for DeskAPI fixture.
type DeskFixtData struct {
	Username string
	Password string
}
