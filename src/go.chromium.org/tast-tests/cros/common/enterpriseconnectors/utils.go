// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package enterpriseconnectors

import "go.chromium.org/tast/core/testing"

// AshAccount1VarName is the enterpriseconnectors ash account1 name.
const AshAccount1VarName = "enterpriseconnectors.ashAccount1"

// AshAccount2VarName is the enterpriseconnectors ash account2 name.
const AshAccount2VarName = "enterpriseconnectors.ashAccount2"

// AshAccount3VarName is the enterpriseconnectors ash account3 name.
const AshAccount3VarName = "enterpriseconnectors.ashAccount3"

const ashDMAAccount1VarName = "enterpriseconnectors.ashDMAAccount1"
const ashDMAAccount2VarName = "enterpriseconnectors.ashDMAAccount2"
const ashDMAAccount3VarName = "enterpriseconnectors.ashDMAAccount3"

var ashAccount1Var = testing.RegisterVarString(
	AshAccount1VarName,
	"",
	"It contains creds in enterpriseconnectors.ashAccount1",
)

var ashAccount2Var = testing.RegisterVarString(
	AshAccount2VarName,
	"",
	"It contains creds in enterpriseconnectors.ashAccount2",
)

var ashAccount3Var = testing.RegisterVarString(
	AshAccount3VarName,
	"",
	"It contains creds in enterpriseconnectors.ashAccount3",
)

var ashDMAAccount1Var = testing.RegisterVarString(
	ashDMAAccount1VarName,
	"",
	"It contains creds in enterpriseconnectors.ashDMAAccount1",
)

var ashDMAAccount2Var = testing.RegisterVarString(
	ashDMAAccount2VarName,
	"",
	"It contains creds in enterpriseconnectors.ashDMAAccount2",
)

var ashDMAAccount3Var = testing.RegisterVarString(
	ashDMAAccount3VarName,
	"",
	"It contains creds in enterpriseconnectors.ashDMAAccount3",
)

// AshAccount1Value returns credentials from enterpriseconnectors.ashAccount1.
func AshAccount1Value() string {
	return ashAccount1Var.Value()
}

// AshAccount2Value returns credentials from enterpriseconnectors.ashAccount2.
func AshAccount2Value() string {
	return ashAccount2Var.Value()
}

// AshAccount3Value returns credentials from enterpriseconnectors.ashAccount3.
func AshAccount3Value() string {
	return ashAccount3Var.Value()
}

// AshDMAAccount1Value returns credentials from enterpriseconnectors.ashDMAAccount1.
func AshDMAAccount1Value() string {
	return ashDMAAccount1Var.Value()
}

// AshDMAAccount2Value returns credentials from enterpriseconnectors.ashDMAAccount2.
func AshDMAAccount2Value() string {
	return ashDMAAccount2Var.Value()
}

// AshDMAAccount3Value returns credentials from enterpriseconnectors.ashDMAAccount3.
func AshDMAAccount3Value() string {
	return ashDMAAccount3Var.Value()
}
