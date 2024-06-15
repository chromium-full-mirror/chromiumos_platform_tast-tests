// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import "go.chromium.org/tast/core/testing"

// AccountPoolVarName is the assistant account pool name.
const AccountPoolVarName = "assistant.accountPool"

var accountPoolVar = testing.RegisterVarString(
	AccountPoolVarName,
	"",
	"It contains creds in assistant.accountPool",
)

// AccountPoolValue returns credentials from assistant.accountPool.
func AccountPoolValue() string {
	return accountPoolVar.Value()
}
