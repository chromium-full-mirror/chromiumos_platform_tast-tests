// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package family

import "go.chromium.org/tast/core/testing"

// HohAccountVarName is the family hoh account name.
const HohAccountVarName = "family.hohAccount"

// ParentAccountVarName is the family parent account name.
const ParentAccountVarName = "family.parentAccount"

// UnicornAllowlistAccountVarName is the family unicorn allowlist account name.
const UnicornAllowlistAccountVarName = "family.unicornAllowlistAccount"

// UnicornAccountVarName is the family unicorn account name.
const UnicornAccountVarName = "family.unicornAccount"

// GellerAccountVarName is the family geller account name.
const GellerAccountVarName = "family.gellerAccount"

// GriffinAccountVarName is the family griffin account name.
const GriffinAccountVarName = "family.griffinAccount"

const hohDMAAccountVarName = "family.hohDMAAccount"

const parentDMAAccountVarName = "family.parentDMAAccount"

const unicornAllowlistDMAAccountVarName = "family.unicornAllowlistDMAAccount"

const unicornDMAAccountVarName = "family.unicornDMAAccount"

const gellerDMAAccountVarName = "family.gellerDMAAccount"

const griffinDMAAccountVarName = "family.griffinDMAAccount"

var hohAccountVar = testing.RegisterVarString(
	HohAccountVarName,
	"",
	"It contains creds in family.hohAccount",
)

var hohDMAAccountVar = testing.RegisterVarString(
	hohDMAAccountVarName,
	"",
	"It contains creds in family.hohDMAAccount",
)

var parentAccountVar = testing.RegisterVarString(
	ParentAccountVarName,
	"",
	"It contains creds in family.parentAccount",
)

var parentDMAAccountVar = testing.RegisterVarString(
	parentDMAAccountVarName,
	"",
	"It contains creds in family.parentDMAAccount",
)

var unicornAllowlistAccountVar = testing.RegisterVarString(
	UnicornAllowlistAccountVarName,
	"",
	"It contains creds in family.unicornAllowlistAccount",
)

var unicornAllowlistDMAAccountVar = testing.RegisterVarString(
	unicornAllowlistDMAAccountVarName,
	"",
	"It contains creds in family.unicornAllowlistDMAAccount",
)

var unicornAccountVar = testing.RegisterVarString(
	UnicornAccountVarName,
	"",
	"It contains creds in family.unicornAccount",
)

var unicornDMAAccountVar = testing.RegisterVarString(
	unicornDMAAccountVarName,
	"",
	"It contains creds in family.unicornDMAAccount",
)

var gellerAccountVar = testing.RegisterVarString(
	GellerAccountVarName,
	"",
	"It contains creds in family.gellerAccount",
)

var gellerDMAAccountVar = testing.RegisterVarString(
	gellerDMAAccountVarName,
	"",
	"It contains creds in family.gellerDMAAccount",
)

var griffinAccountVar = testing.RegisterVarString(
	GriffinAccountVarName,
	"",
	"It contains creds in family.griffinAccount",
)

var griffinDMAAccountVar = testing.RegisterVarString(
	griffinDMAAccountVarName,
	"",
	"It contains creds in family.griffinDMAAccount",
)

// HohAccountValue returns credentials from family.hohAccount.
func HohAccountValue() string {
	return hohAccountVar.Value()
}

// HohDMAAccountValue returns credentials from family.hohDMAAccount.
func HohDMAAccountValue() string {
	return hohDMAAccountVar.Value()
}

// ParentAccountValue returns credentials from family.parentAccount.
func ParentAccountValue() string {
	return parentAccountVar.Value()
}

// ParentDMAAccountValue returns credentials from family.parentDMAAccount.
func ParentDMAAccountValue() string {
	return parentDMAAccountVar.Value()
}

// UnicornAllowlistAccountValue returns credentials from family.parentAccount.
func UnicornAllowlistAccountValue() string {
	return unicornAllowlistAccountVar.Value()
}

// UnicornAllowlistDMAAccountValue returns credentials from family.unicornAllowlistDMAAccount.
func UnicornAllowlistDMAAccountValue() string {
	return unicornAllowlistDMAAccountVar.Value()
}

// UnicornAccountValue returns credentials from family.parentAccount.
func UnicornAccountValue() string {
	return unicornAccountVar.Value()
}

// UnicornDMAAccountValue returns credentials from family.unicornDMAAccount.
func UnicornDMAAccountValue() string {
	return unicornDMAAccountVar.Value()
}

// GellerAccountValue returns credentials from family.parentAccount.
func GellerAccountValue() string {
	return gellerAccountVar.Value()
}

// GellerDMAAccountValue returns credentials from family.gellerDMAAccount.
func GellerDMAAccountValue() string {
	return gellerDMAAccountVar.Value()
}

// GriffinAccountValue returns credentials from family.parentAccount.
func GriffinAccountValue() string {
	return griffinAccountVar.Value()
}

// GriffinDMAAccountValue returns credentials from family.griffinDMAAccount.
func GriffinDMAAccountValue() string {
	return griffinDMAAccountVar.Value()
}
