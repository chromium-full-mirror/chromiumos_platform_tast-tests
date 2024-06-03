// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package drivefs

import "go.chromium.org/tast/core/testing"

// AccountPoolVarName is the drivefs account pool name.
const AccountPoolVarName = "drivefs.accountPool"

var accountPoolVar = testing.RegisterVarString(
	AccountPoolVarName,
	"",
	"It contains creds in drivefs.accountPool",
)

// AccountPoolValue returns credentials from drivefs.accountPool.
func AccountPoolValue() string {
	return accountPoolVar.Value()
}
