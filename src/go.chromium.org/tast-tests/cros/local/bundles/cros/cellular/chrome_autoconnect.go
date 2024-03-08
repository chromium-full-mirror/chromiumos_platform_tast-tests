// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ChromeAutoconnect,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that Cellular service auto-connects after login",
		Contacts:       []string{"chromeos-cellular-team@google.com", "madhavadas@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_run_isolated"},
		Fixture:        "cellularWithChrome",
		Timeout:        3 * time.Minute,
	})
}

// ChromeAutoconnect checks that shill autoconnects after Chrome login.
func ChromeAutoconnect(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	service, err := helper.FindServiceForDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get Cellular Service: ", err)
	}

	timeout := 2 * time.Minute
	s.Logf("Waiting for %v for IsConnected = true", timeout)
	if err := service.WaitForProperty(ctx, shillconst.ServicePropertyIsConnected, true, timeout); err != nil {
		s.Fatalf("Service not connected after %v: %v", timeout, err)
	}
}
