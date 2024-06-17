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
