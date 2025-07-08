// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crossdevice

import "go.chromium.org/tast/core/testing"

// DefaultCrossDevicePoolVarName is the crossdevice default account pool name.
const DefaultCrossDevicePoolVarName = "crossdevice.defaultAccount"

// SmartLockPoolVarName is the smart lock account pool name.
const SmartLockPoolVarName = "crossdevice.smartLockAccount"

const dmaDefaultCrossDevicePoolVarName = "crossdevice.dmaDefaultAccount"

const dmaSmartLockPoolVarName = "crossdevice.dmaSmartLockAccount"

var defaultCrossDevicePoolVar = testing.RegisterVarString(
	DefaultCrossDevicePoolVarName,
	"",
	"It contains creds in crossdevice.defaultAccount",
)

var dmaDefaultCrossDevicePoolVar = testing.RegisterVarString(
	dmaDefaultCrossDevicePoolVarName,
	"",
	"It contains creds in crossdevice.dmaDefaultAccount",
)

var smartLockPoolVar = testing.RegisterVarString(
	SmartLockPoolVarName,
	"",
	"It contains creds in crossdevice.smartLockAccount",
)

var dmaSmartLockPoolVar = testing.RegisterVarString(
	dmaSmartLockPoolVarName,
	"",
	"It contains creds in crossdevice.dmaSmartLockAccount",
)

// DefaultCrossDevicePoolValue returns credentials from crossdevice.defaultAccount.
func DefaultCrossDevicePoolValue() string {
	return defaultCrossDevicePoolVar.Value()
}

// DmaDefaultCrossDevicePoolValue returns credentials from crossdevice.dmaDefaultAccount.
func DmaDefaultCrossDevicePoolValue() string {
	return dmaDefaultCrossDevicePoolVar.Value()
}

// SmartLockPoolValue returns credentials from crossdevice.smartLockAccount.
func SmartLockPoolValue() string {
	return smartLockPoolVar.Value()
}

// DmaSmartLockPoolValue returns credentials from crossdevice.dmaSmartLockAccount.
func DmaSmartLockPoolValue() string {
	return dmaSmartLockPoolVar.Value()
}
