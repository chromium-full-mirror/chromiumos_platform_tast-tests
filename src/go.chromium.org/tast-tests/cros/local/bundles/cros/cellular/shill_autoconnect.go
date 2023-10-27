// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/cellular"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:           ShillAutoconnect,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		Desc:           "Verifies that Shill auto-connects to a Cellular Service correctly",
		Contacts:       []string{"chromeos-cellular-team@google.com", "pholla@google.com"},
		BugComponent:   "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Attr:           []string{"group:cellular", "cellular_unstable", "cellular_sim_active", "cellular_run_isolated"},
		Fixture:        "cellular",
		Timeout:        3 * time.Minute,
	})
}

// ShillAutoconnect checks that shill autoconnects when ScanAndConnectToBestServices is called
// initially (i.e. after reboot or a Shill restart).
func ShillAutoconnect(ctx context.Context, s *testing.State) {
	helper := s.FixtValue().(*cellular.FixtData).Helper

	// Cellular will only auto-connect *once* when Manager.ScanAndConnectToBestServices is called,
	// so restart shill to ensure that auto-connect will happen.
	if err := helper.ResetShill(ctx); err != nil {
		s.Fatal("Failed to reset shill")
	}

	service, err := helper.FindServiceForDevice(ctx)
	if err != nil {
		s.Fatal("Failed to get Cellular Service: ", err)
	}

	if isConnected, err := service.IsConnected(ctx); err != nil {
		s.Fatal("IsConnected failed: ", err)
	} else if isConnected {
		s.Fatal("Cellular is connected after shill restart")
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	// Ensure that AutoConnect is enabled and disable any roaming restrictions.
	cleanup1, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyAutoConnect, true)
	if err != nil {
		s.Fatal("Could not initialize autoconnect to true: ", err)
	}
	defer cleanup1(cleanupCtx)
	cleanup2, err := helper.InitDeviceProperty(ctx, shillconst.DevicePropertyCellularPolicyAllowRoaming, true)
	if err != nil {
		s.Fatal("Could not set PolicyAllowRoaming to true: ", err)
	}
	defer cleanup2(cleanupCtx)
	cleanup3, err := helper.InitServiceProperty(ctx, shillconst.ServicePropertyCellularAllowRoaming, true)
	if err != nil {
		s.Fatal("Could not set AllowRoaming property to true: ", err)
	}
	defer cleanup3(cleanupCtx)

	// Manager.ScanAndConnectToBestServices will trigger an auto connect.
	if err := helper.Manager.ScanAndConnectToBestServices(ctx); err != nil {
		s.Fatal("ScanAndConnectToBestServices call failed: ", err)
	}

	timeout := 2 * time.Minute
	s.Logf("Waiting for %v for IsCOnnected = true", timeout)
	if err := service.WaitForProperty(ctx, shillconst.ServicePropertyIsConnected, true, timeout); err != nil {
		s.Fatalf("Service not connected after %v, error: %v", timeout, err)
	}
}
