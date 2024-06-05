// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import "go.chromium.org/tast/core/testing"

// GaiaPoolDefaultVarName is the ui default pool name.
const GaiaPoolDefaultVarName = "ui.gaiaPoolDefault"

// CUJAccountPoolVarName is the ui cujAccountPool name.
const CUJAccountPoolVarName = "ui.cujAccountPool"

var gaiaPoolDefaultVar = testing.RegisterVarString(
	GaiaPoolDefaultVarName,
	"",
	"It contains creds in ui.gaiaPoolDefault",
)

var gaiaDMAPoolDefaultVar = testing.RegisterVarString(
	"ui.gaiaDMAPoolDefault",
	"",
	"It contains creds in ui.gaiaDMAPoolDefault",
)

var cujAccountPoolVar = testing.RegisterVarString(
	CUJAccountPoolVarName,
	"",
	"It contains creds in ui.cujAccountPool",
)

// GaiaPoolDefaultValue returns credentials from ui.gaiaPoolDefault.
func GaiaPoolDefaultValue() string {
	return gaiaPoolDefaultVar.Value()
}

// GaiaDMAPoolDefaultValue returns credentials from ui.gaiaDMAPoolDefault.
func GaiaDMAPoolDefaultValue() string {
	return gaiaDMAPoolDefaultVar.Value()
}

// CUJAccountPoolValue returns credentials from ui.cujAccountPool.
func CUJAccountPoolValue() string {
	return cujAccountPoolVar.Value()
}
