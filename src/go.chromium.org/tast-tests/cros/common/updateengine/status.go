// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package updateengine provides ways to interact with update_engine daemon and utilities.
package updateengine

// UpdateStatus is the type of update_engine status used internally during tast.
type UpdateStatus string

// List of UpdateStatus update_engine currently supports.
const (
	UpdateStatusIdle                UpdateStatus = "UPDATE_STATUS_IDLE"
	UpdateStatusReportingErrorEvent UpdateStatus = "UPDATE_STATUS_REPORTING_ERROR_EVENT"
	UpdateStatusUpdatedButDeferred  UpdateStatus = "UPDATE_STATUS_UPDATED_BUT_DEFERRED"
	UpdateStatusUpdatedNeedReboot   UpdateStatus = "UPDATE_STATUS_UPDATED_NEED_REBOOT"
)
