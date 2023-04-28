// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package common contains common data types and libraries used for video conferencing testing.
package common

// LaunchAppType is the type to launch meeting app.
type LaunchAppType int

// Available options to launch a meeting app.
const (
	LaunchAppInWeb LaunchAppType = iota
	LaunchAppInPWA
)
