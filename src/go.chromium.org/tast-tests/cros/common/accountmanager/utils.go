// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package accountmanager

import "go.chromium.org/tast/core/testing"

// AccountPoolVarName is the accountmanager account pool name.
const AccountPoolVarName = "accountmanager.accountPool"

var accountPoolVar = testing.RegisterVarString(
	AccountPoolVarName,
	"",
	"It contains creds in accountmanager.accountPool",
)

// AccountPoolValue returns credentials from accountmanager.accountPool.
func AccountPoolValue() string {
	return accountPoolVar.Value()
}
