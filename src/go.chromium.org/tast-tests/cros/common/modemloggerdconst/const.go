// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package modemloggerdconst defines the constants for Modemloggerd
// https://chromium.googlesource.com/chromiumos/platform2/+/HEAD/modemloggerd/README.md
// This package is defined under common/ as they might be used in both
// local and remote tests.
package modemloggerdconst

// Modemloggerd D-Bus constants.
const (
	DBusModemloggerdManagerPath      = "/org/chromium/Modemloggerd/Manager"
	DBusModemloggerdService          = "org.chromium.Modemloggerd"
	DBusModemloggerdManagerInterface = "org.chromium.Modemloggerd.Manager"
	DBusModemloggerdModemInterface   = "org.chromium.Modemloggerd.Modem"
)

// Modemloggerd.Modem properties
const (
	ManagerPropertyAvailableModems = "AvailableModems"
)

// Modemloggerd.Modem methods
const (
	ModemMethodStart        = "Start"
	ModemMethodStop         = "Stop"
	ModemMethodSetOutputDir = "SetOutputDir"
	ModemMethodSetEnabled   = "SetEnabled"
)
