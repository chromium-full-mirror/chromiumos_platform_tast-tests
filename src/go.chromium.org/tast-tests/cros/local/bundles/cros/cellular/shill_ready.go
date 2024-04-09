// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/testing"
)

// Note: This test enables Cellular if not already enabled.

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShillReady,
		Desc:         "Verifies that Shill is running and that a Cellular Device and connectable Service are present",
		Contacts:     []string{"chromeos-cellular-team@google.com", "cros-network-health-team@google.com", "ejcaruso@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_sim_active", "cellular_cq", "cellular_ota_avl"},
		Fixture:      "cellular",
	})
}

func ShillReady(ctx context.Context, s *testing.State) {
	helper, err := cellular.NewHelper(ctx)
	if err != nil {
		s.Fatal("Failed to create cellular.Helper: ", err)
	}
	// Ensure that a Cellular Service was created.
	if _, err := helper.FindService(ctx); err != nil {
		s.Fatal("Unable to find Cellular Service: ", err)
	}
}
