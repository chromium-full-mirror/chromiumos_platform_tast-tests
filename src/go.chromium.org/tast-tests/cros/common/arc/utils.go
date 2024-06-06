// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import "go.chromium.org/tast/core/testing"

// ManagedAccountPoolVarName is the arc managed account pool name.
const ManagedAccountPoolVarName = "arc.managedAccountPool"

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

// ManagedAccountPoolValue returns credentials from arc.managedAccountPool.
func ManagedAccountPoolValue() string {
	return managedAccountPoolVar.Value()
}

// ManagedDMAAccountPoolValue returns credentials from arc.managedDMAAccountPool.
func ManagedDMAAccountPoolValue() string {
	return managedDMAAccountPoolVar.Value()
}
