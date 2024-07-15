// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package nearbyshare

import "go.chromium.org/tast/core/testing"

// CrosAccountPoolVarName is the nearbyshare cros account pool name.
const CrosAccountPoolVarName = "nearbyshare.crosAccount"

// CrosAccount2PoolVarName is the nearbyshare cros account 2 pool name.
const CrosAccount2PoolVarName = "nearbyshare.crosAccount2"

// AndroidAccountPoolVarName is the nearbyshare android account pool name.
const AndroidAccountPoolVarName = "nearbyshare.androidAccount"

// DevAndroidAccountPoolVarName is the nearbyshare dev android account pool name.
const DevAndroidAccountPoolVarName = "nearbyshare.devAndroidAccount"

// ProdAndroidAccountPoolVarName is the nearbyshare prod android account pool name.
const ProdAndroidAccountPoolVarName = "nearbyshare.prodAndroidAccount"

const dmaCrosAccountPoolVarName = "nearbyshare.dmaCrosAccount"

const dmaCrosAccount2PoolVarName = "nearbyshare.dmaCrosAccount2"

const dmaAndroidAccountPoolVarName = "nearbyshare.dmaAndroidAccount"

const dmaDevAndroidAccountPoolVarName = "nearbyshare.dmaDevAndroidAccount"

const dmaProdAndroidAccountPoolVarName = "nearbyshare.dmaProdAndroidAccount"

var crosAccountPoolVar = testing.RegisterVarString(
	CrosAccountPoolVarName,
	"",
	"It contains creds in nearbyshare.crosAccount",
)

var dmaCrosAccountPoolVar = testing.RegisterVarString(
	dmaCrosAccountPoolVarName,
	"",
	"It contains creds in nearbyshare.dmaCrosAccount",
)

var crosAccount2PoolVar = testing.RegisterVarString(
	CrosAccount2PoolVarName,
	"",
	"It contains creds in nearbyshare.crosAccount2",
)

var dmaCrosAccount2PoolVar = testing.RegisterVarString(
	dmaCrosAccount2PoolVarName,
	"",
	"It contains creds in nearbyshare.dmaCrosAccount2",
)

var androidAccountPoolVar = testing.RegisterVarString(
	AndroidAccountPoolVarName,
	"",
	"It contains creds in nearbyshare.androidAccount",
)

var dmaAndroidAccountPoolVar = testing.RegisterVarString(
	dmaAndroidAccountPoolVarName,
	"",
	"It contains creds in nearbyshare.dmaAndroidAccount",
)

var devAndroidAccountPoolVar = testing.RegisterVarString(
	DevAndroidAccountPoolVarName,
	"",
	"It contains creds in nearbyshare.devAndroidAccount",
)

var dmaDevAndroidAccountPoolVar = testing.RegisterVarString(
	dmaDevAndroidAccountPoolVarName,
	"",
	"It contains creds in nearbyshare.dmaDevAndroidAccount",
)

var prodAndroidAccountPoolVar = testing.RegisterVarString(
	ProdAndroidAccountPoolVarName,
	"",
	"It contains creds in nearbyshare.prodAndroidAccount",
)

var dmaProdAndroidAccountPoolVar = testing.RegisterVarString(
	dmaProdAndroidAccountPoolVarName,
	"",
	"It contains creds in nearbyshare.dmaProdAndroidAccount",
)

// CrosAccountPoolValue returns credentials from nearbyshare.crosAccount.
func CrosAccountPoolValue() string {
	return crosAccountPoolVar.Value()
}

// DmaCrosAccountPoolValue returns credentials from nearbyshare.dmaCrosAccount.
func DmaCrosAccountPoolValue() string {
	return dmaCrosAccountPoolVar.Value()
}

// CrosAccount2PoolValue returns credentials from nearbyshare.crosAccount2.
func CrosAccount2PoolValue() string {
	return crosAccount2PoolVar.Value()
}

// DmaCrosAccount2PoolValue returns credentials from nearbyshare.dmaCrosAccount2.
func DmaCrosAccount2PoolValue() string {
	return dmaCrosAccount2PoolVar.Value()
}

// AndroidAccountPoolValue returns credentials from nearbyshare.androidAccount.
func AndroidAccountPoolValue() string {
	return androidAccountPoolVar.Value()
}

// DmaAndroidAccountPoolValue returns credentials from nearbyshare.dmaAndroidAccount.
func DmaAndroidAccountPoolValue() string {
	return dmaAndroidAccountPoolVar.Value()
}

// DevAndroidAccountPoolValue returns credentials from nearbyshare.devAndroidAccount.
func DevAndroidAccountPoolValue() string {
	return devAndroidAccountPoolVar.Value()
}

// DmaDevAndroidAccountPoolValue returns credentials from nearbyshare.dmaDevAndroidAccount.
func DmaDevAndroidAccountPoolValue() string {
	return dmaDevAndroidAccountPoolVar.Value()
}

// ProdAndroidAccountPoolValue returns credentials from nearbyshare.prodAndroidAccount.
func ProdAndroidAccountPoolValue() string {
	return prodAndroidAccountPoolVar.Value()
}

// DmaProdAndroidAccountPoolValue returns credentials from nearbyshare.dmaProdAndroidAccount.
func DmaProdAndroidAccountPoolValue() string {
	return dmaProdAndroidAccountPoolVar.Value()
}
