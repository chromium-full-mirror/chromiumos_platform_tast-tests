// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CheckSignalQuality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that host has signal quality above threshold via cellular interface",
		Contacts:     []string{"chromeos-cellular-team@google.com", "nmarupaka@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:         []string{"group:cellular", "cellular_unstable", "cellular_sim_active"},
		Fixture:      "cellular",
		Timeout:      3 * time.Minute,
		SoftwareDeps: []string{"chrome"},
	})
}

// CheckSignalQuality needs to be run to verify that the DUT has sufficient signal coverage to execute other network related test cases
func CheckSignalQuality(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper
	if _, err := helper.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to cellular service: ", err)
	}

	if err := helper.ConnectAndCheckSignalQuality(ctx); err != nil {
		s.Fatal("Failed to check signal quality: ", err)
	}

	// Nothing else to be done in this test. Creating the helper validates the cellular test environment.
	// Disconnect cellular before exiting the test
	if _, err := helper.Disconnect(ctx); err != nil {
		s.Fatal("Unable to Disconnect from default service: ", err)
	}
}
