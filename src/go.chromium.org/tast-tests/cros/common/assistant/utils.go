// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import "go.chromium.org/tast/core/testing"

// AccountPoolVarName is the assistant account pool name.
const AccountPoolVarName = "assistant.accountPool"

const dmaAccountPoolVarName = "assistant.dmaAccountPool"

var accountPoolVar = testing.RegisterVarString(
	AccountPoolVarName,
	"",
	"It contains creds in assistant.accountPool",
)

var dmaAccountPoolVar = testing.RegisterVarString(
	dmaAccountPoolVarName,
	"",
	"It contains creds in assistant.dmaAccountPool",
)

// AccountPoolValue returns credentials from assistant.accountPool.
func AccountPoolValue() string {
	return accountPoolVar.Value()
}

// DmaAccountPoolValue returns credentials from dmaAccountPool.accountPool.
func DmaAccountPoolValue() string {
	return dmaAccountPoolVar.Value()
}
