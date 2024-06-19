// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import "go.chromium.org/tast/core/testing"

// WarnAccountPoolVarName is the filemanager warn account pool name.
const WarnAccountPoolVarName = "filemanager.DrivefsPooledStorage.WarnAccountPool"

// FullAccountPoolVarName is the filemanager full account pool name.
const FullAccountPoolVarName = "filemanager.DrivefsPooledStorage.FullAccountPool"

// OrgFullAccountPoolVarName is the filemanager org full account pool name.
const OrgFullAccountPoolVarName = "filemanager.DrivefsPooledStorage.OrgFullAccountPool"

const warnDMAAccountPoolVarName = "filemanager.DrivefsPooledStorage.WarnDMAAccountPool"

const fullDMAAccountPoolVarName = "filemanager.DrivefsPooledStorage.FullDMAAccountPool"

const orgFullDMAAccountPoolVarName = "filemanager.DrivefsPooledStorage.OrgFullDMAAccountPool"

var warnAccountPoolVar = testing.RegisterVarString(
	WarnAccountPoolVarName,
	"",
	"It contains creds in filemanager.DrivefsPooledStorage.WarnAccountPool",
)

// WarnAccountPoolValue returns credentials from filemanager.DrivefsPooledStorage.WarnAccountPool.
func WarnAccountPoolValue() string {
	return warnAccountPoolVar.Value()
}

var warnDMAAccountPoolVar = testing.RegisterVarString(
	warnDMAAccountPoolVarName,
	"",
	"It contains creds in filemanager.DrivefsPooledStorage.WarnDMAAccountPool",
)

// WarnDMAAccountPoolValue returns credentials from filemanager.DrivefsPooledStorage.WarnDMAAccountPool.
func WarnDMAAccountPoolValue() string {
	return warnDMAAccountPoolVar.Value()
}

var fullAccountPoolVar = testing.RegisterVarString(
	FullAccountPoolVarName,
	"",
	"It contains creds in filemanager.DrivefsPooledStorage.FullAccountPool",
)

// FullAccountPoolValue returns credentials from filemanager.DrivefsPooledStorage.FullAccountPool.
func FullAccountPoolValue() string {
	return fullAccountPoolVar.Value()
}

var fullDMAAccountPoolVar = testing.RegisterVarString(
	fullDMAAccountPoolVarName,
	"",
	"It contains creds in filemanager.DrivefsPooledStorage.FullDMAAccountPool",
)

// FullDMAAccountPoolValue returns credentials from filemanager.DrivefsPooledStorage.FullDMAAccountPool.
func FullDMAAccountPoolValue() string {
	return fullDMAAccountPoolVar.Value()
}

var orgFullAccountPoolVar = testing.RegisterVarString(
	OrgFullAccountPoolVarName,
	"",
	"It contains creds in filemanager.DrivefsPooledStorage.OrgFullAccountPool",
)

// OrgFullAccountPoolValue returns credentials from filemanager.DrivefsPooledStorage.FullAccountPool.
func OrgFullAccountPoolValue() string {
	return orgFullAccountPoolVar.Value()
}

var orgFullDMAAccountPoolVar = testing.RegisterVarString(
	orgFullDMAAccountPoolVarName,
	"",
	"It contains creds in filemanager.DrivefsPooledStorage.OrgFullDMAAccountPool",
)

// OrgFullDMAAccountPoolValue returns credentials from filemanager.DrivefsPooledStorage.FullDMAAccountPool.
func OrgFullDMAAccountPoolValue() string {
	return orgFullDMAAccountPoolVar.Value()
}
