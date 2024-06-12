// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import "go.chromium.org/tast/core/testing"

// ManagedUserAccountPoolVarName is the policy managed user account pool name.
const ManagedUserAccountPoolVarName = "policy.managedUserAccountPool"

var managedUserAccountPoolVar = testing.RegisterVarString(
	ManagedUserAccountPoolVarName,
	"",
	"It contains creds in policy.managedUserAccountPool",
)

// ManagedUserAccountPoolValue returns credentials from policy.managedUserAccountPool.
func ManagedUserAccountPoolValue() string {
	return managedUserAccountPoolVar.Value()
}
