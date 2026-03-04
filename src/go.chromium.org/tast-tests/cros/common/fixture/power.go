// Copyright 2026 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fixture

const (
	// TuwunelServerSetup is a remote fixture that sets up a Tuwunel server on the host.
	// It also establishes a port forward from the DUT to the host, allowing tests
	// on the DUT to communicate with the server.
	TuwunelServerSetup = "tuwunelServerSetup"
)
