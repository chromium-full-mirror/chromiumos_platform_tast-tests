// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package passpoint

const (
	// ProfileUser is the name of the fake test user profile
	ProfileUser = "test"
)

// Domains and Organisation Identifier (OI) used to test Passpoint
// network selection. The domains and OIs are extracted from Passpoint
// specification v3.2 - Appendix C.
// OI is a 24-bit OUI, 24-bit CID, or 36-bit OUI-36 of IEEE standards.
// The value is expected to be represented as a hex string of length 6 or 10.
const (
	BlueDomain        = "sp-blue.com"
	GreenDomain       = "sp-green.com"
	RedDomain         = "sp-red.com"
	HomeOI            = "871d2e"
	RoamingOI1        = "001bc50050"
	RoamingOI2        = "001bc500b5"
	PasspointProvider = "Passpoint provider"
	PasspointApp      = "app.passpoint.example.com"
	TrustedCA         = "Trusted CA"
	SystemCA          = "System CAs"
)
