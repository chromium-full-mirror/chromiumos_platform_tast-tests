// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dev

import "go.chromium.org/tast/core/testing"

// AccountVarName is the dev account name.
const AccountVarName = "dev.account"

var accountVar = testing.RegisterVarString(
	AccountVarName,
	"",
	"It contains creds in dev.account",
)

// AccountValue returns credentials from dev.account.
func AccountValue() string {
	return accountVar.Value()
}
