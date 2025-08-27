// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

const (
	// RebootForProbeFunction is a fixture for runtimeprobe tests.
	// Reboot the DUT and wait for hardware_verifier to finish running at the
	// start of the fixture to make sure tests are run after a reboot.
	RebootForProbeFunction = "rebootForProbeFunction"

	// CleanupRuntimeHWID is a fixture for runtimeprobe tests.
	// Delete the Runtime HWID file from the DUT at the beginning and end of the
	// fixture.
	CleanupRuntimeHWID = "cleanupRuntimeHWID"
)
