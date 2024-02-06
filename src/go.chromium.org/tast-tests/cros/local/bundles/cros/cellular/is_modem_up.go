// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/modemmanager"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IsModemUp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that a modem is available",
		Contacts:     []string{"chromeos-cellular-team@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_cq"},
		Fixture:      "cellularDUTCheckLocal",
		Timeout:      3 * time.Minute,
	})
}

// IsModemUp checks if the fixture and pre test executed successfully.
func IsModemUp(ctx context.Context, s *testing.State) {
	_, err := modemmanager.NewModem(ctx)
	if err != nil {
		s.Fatal("Could not find MM dbus object: ", err)
	}
}
