// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

// Fixtures defined in go.chromium.org/tast-tests/cros/remote/demomode/fixture.go.
const (
	// PostDemoModeOOBE is the name for the fixture that clicks through Demo Mode OOBE setup.
	// Enrolls through Alpha DMServer.
	PostDemoModeOOBEAlpha = "postDemoModeOOBEAlpha"
	// PostDemoModeOOBE is the name for the fixture that clicks through Demo Mode OOBE setup.
	// Enrolls through Prod DMServer.
	PostDemoModeOOBEProd = "postDemoModeOOBEProd"
	// PostDemoModeOOBECloudGaming is similar to PostDemoModeOOBE, except Cloud Gaming
	// customizations are enabled (i.e. the CloudGamingDevice feature is enabled).
	PostDemoModeOOBECloudGaming = "postDemoModeOOBECloudGaming"
)
