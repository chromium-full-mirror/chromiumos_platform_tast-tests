// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arcappcompat

import "go.chromium.org/tast/core/testing"

// AccountVarName is the arcappcompat account name.
const AccountVarName = "arcappcompat.Account"

const dmaAccountVarName = "arcappcompat.DmaAccount"

var accountVar = testing.RegisterVarString(
	AccountVarName,
	"",
	"It contains creds in arcappcompat.Account",
)

var dmaAccountVar = testing.RegisterVarString(
	dmaAccountVarName,
	"",
	"It contains creds in arcappcompat.DmaAccount",
)

// AccountValue returns credentials from arcappcompat.Account.
func AccountValue() string {
	return accountVar.Value()
}

// DmaAccountValue returns credentials from arcappcompat.DmaAccount.
func DmaAccountValue() string {
	return dmaAccountVar.Value()
}
