// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package filesconsts constants for Files app/File Manager related apps/packages.
// Used to avoid circular imports across those packages.
package filesconsts

// Provider is either Google Drive or Microsoft OneDrive.
type Provider string

// Cloud Provider names.
const (
	OneDrive Provider = "onedrive"
	DriveFs  Provider = "drivefs"
)
