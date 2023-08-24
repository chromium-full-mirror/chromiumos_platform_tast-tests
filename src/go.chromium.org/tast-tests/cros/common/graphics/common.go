// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package graphics contains common variable to shared across remote and local packages.
package graphics

const (
	// GraphicsRemoteWatcherRebootFile is the file on the DUT to notify gpuRemoteWatcher fixture to reboot the machine.
	// /tmp is memory backed and should be cleared during a reboot.
	GraphicsRemoteWatcherRebootFile = "/tmp/gpuRemoteWatcherReboot"
)
