// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"chromiumos/tast/local/cellular"
	"chromiumos/tast/local/modemmanager"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: IsConnected,
		Desc: "Verifies that Shill can connect to a network",
		Contacts: []string{
			"chromeos-cellular-team@google.com",
		},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_run_isolated"},
		Fixture:      "cellular",
		Timeout:      5 * time.Minute,
	})
}

func IsConnected(ctx context.Context, s *testing.State) {
	if _, err := modemmanager.NewModemWithSim(ctx); err != nil {
		s.Fatal("Could not find MM dbus object with a valid sim: ", err)
	}
	helper, err := cellular.NewHelperWithConnectedCellular(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}
}
