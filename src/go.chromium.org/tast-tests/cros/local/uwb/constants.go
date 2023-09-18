// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uwb

// Constants used for dbus control
const (
	dbusService                       = "org.chromium.uwbd"
	dbusPath                          = "/org/chromium/uwbd"
	dbusEnable                        = "Enable"
	dbusDisable                       = "Disable"
	dbusSetLoggerMode                 = "SetLoggerMode"
	dbusInitSession                   = "InitSession"
	dbusDeinitSession                 = "DeinitSession"
	dbusStartRanging                  = "StartRanging"
	dbusStopRanging                   = "StopRanging"
	dbusSessionParams                 = "SessionParams"
	dbusReconfigure                   = "Reconfigure"
	dbusUpdateControllerMulticastList = "UpdateControllerMulticastList"
	dbusRawUciCmd                     = "RawUciCmd"
	dbusOnRangeDataReceived           = "OnRangeDataReceived"
)
