// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package connectivityfwdumps

import "go.chromium.org/tast/core/testing"

// GaiaLoginAccountVarName is the connectivityfwdumps gaia login account pool name.
const GaiaLoginAccountVarName = "connectivityfwdumps.gaiaLoginAccount"

var gaiaLoginAccountVar = testing.RegisterVarString(
	GaiaLoginAccountVarName,
	"",
	"It contains creds in connectivityfwdumps.gaiaLoginAccount",
)

// GaiaLoginAccountValue returns credentials from connectivityfwdumps.gaiaLoginAccount.
func GaiaLoginAccountValue() string {
	return gaiaLoginAccountVar.Value()
}
