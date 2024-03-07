// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

const (
	// CameraServiceReady ensures camera service is ready.
	CameraServiceReady = "cameraServiceReady"
	// CameraConnectorReady ensures camera connector is ready without any user.
	CameraConnectorReady = "cameraConnectorReady"

	// CameraEnumerated ensures all the built-in cameras are enumerated.
	// This is a remote fixture meant to be used as parent of a local fixture.
	CameraEnumerated = "cameraEnumerated"
)
