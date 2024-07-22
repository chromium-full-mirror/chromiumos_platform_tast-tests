// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ambient

import "go.chromium.org/tast/core/testing"

// AccountVarName is the ambient account name.
const AccountVarName = "ambient.account"

var accountVar = testing.RegisterVarString(
	AccountVarName,
	"",
	"It contains creds in ambient.account",
)

// AccountValue returns credentials from ambient.account.
func AccountValue() string {
	return accountVar.Value()
}
