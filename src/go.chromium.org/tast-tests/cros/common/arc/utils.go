// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import "go.chromium.org/tast/core/testing"

// ManagedAccountPoolVarName is the arc managed account pool name.
const ManagedAccountPoolVarName = "arc.managedAccountPool"

// DrivefsPoolVarName is the drive fs pool name.
const DrivefsPoolVarName = "arc.Drivefs.AccountPool"

// SharesheetPoolVarName is the share sheet pool name.
const SharesheetPoolVarName = "arc.Sharesheet.AccountPool"

// Managed3pEmmAccountVarName is the managed 3p emm account.
const Managed3pEmmAccountVarName = "arc.managed3pEmmAccount"

// ChildAccountVarName is the arc child account.
const ChildAccountVarName = "arc.childAccount"

// ChildDMAAccountVarName is the arc child dma account.
const childDMAAccountVarName = "arc.childDMAAccount"

// ParentAccountVarName is the arc parent account.
const ParentAccountVarName = "arc.parentAccount"

// ParentDMAAccountVarName is the arc parent account.
const parentDMAAccountVarName = "arc.parentDMAAccount"

const managedDMAAccountPoolVarName = "arc.managedDMAAccountPool"

var managedAccountPoolVar = testing.RegisterVarString(
	ManagedAccountPoolVarName,
	"",
	"It contains creds in arc.managedAccountPool",
)

var managedDMAAccountPoolVar = testing.RegisterVarString(
	managedDMAAccountPoolVarName,
	"",
	"It contains creds in arc.managedDMAAccountPool",
)

var drivefsPoolVar = testing.RegisterVarString(
	DrivefsPoolVarName,
	"",
	"It contains creds in arc.Drivefs.AccountPool",
)

var sharesheetPoolVar = testing.RegisterVarString(
	SharesheetPoolVarName,
	"",
	"It contains creds in arc.Sharesheet.AccountPool",
)

var managed3pEmmAccountVar = testing.RegisterVarString(
	Managed3pEmmAccountVarName,
	"",
	"It contains creds in arc.managedDMAAccountPool",
)

var childAccountVar = testing.RegisterVarString(
	ChildAccountVarName,
	"",
	"It contains creds in arc.childAccount",
)

var childDMAAccountVar = testing.RegisterVarString(
	childDMAAccountVarName,
	"",
	"It contains creds in arc.childDMAAccount",
)

var parentAccountVar = testing.RegisterVarString(
	ParentAccountVarName,
	"",
	"It contains creds in arc.parentAccount",
)

var parentDMAAccountVar = testing.RegisterVarString(
	parentDMAAccountVarName,
	"",
	"It contains creds in arc.parentDMAAccount",
)

// ManagedAccountPoolValue returns credentials from arc.managedAccountPool.
func ManagedAccountPoolValue() string {
	return managedAccountPoolVar.Value()
}

// ManagedDMAAccountPoolValue returns credentials from arc.managedDMAAccountPool.
func ManagedDMAAccountPoolValue() string {
	return managedDMAAccountPoolVar.Value()
}

// DrivefsPoolValue returns credentials from arc.Drivefs.AccountPool.
func DrivefsPoolValue() string {
	return drivefsPoolVar.Value()
}

// SharesheetPoolValue returns credentials from arc.Sharesheet.AccountPool.
func SharesheetPoolValue() string {
	return sharesheetPoolVar.Value()
}

// Managed3pEmmAccountValue returns credentials from arc.managedDMAAccountPool.
func Managed3pEmmAccountValue() string {
	return managed3pEmmAccountVar.Value()
}

// ChildAccountValue returns credentials from arc.childAccount.
func ChildAccountValue() string {
	return childAccountVar.Value()
}

// ChildDMAAccountValue returns credentials from arc.childDMAAccount.
func ChildDMAAccountValue() string {
	return childDMAAccountVar.Value()
}

// ParentAccountValue returns credentials from arc.parentAccount.
func ParentAccountValue() string {
	return parentAccountVar.Value()
}

// ParentDMAAccountValue returns credentials from arc.parentDMAAccount.
func ParentDMAAccountValue() string {
	return parentDMAAccountVar.Value()
}
