// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/policyutil"
	pb "go.chromium.org/tast-tests/cros/services/cros/network"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const systemProxyForArcTestTimeout = 10 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         SystemProxyForArc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that ARC++ apps can successfully connect to the remote host through the system-proxy daemon",
		Contacts: []string{
			"chromeos-commercial-networking@google.com", // Team
			"acostinas@google.com",                      // Test author
		},
		BugComponent: "b:1000044",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		ServiceDeps:  []string{"tast.cros.network.TestArcConnectivityAppService"},
		Timeout:      systemProxyForArcTestTimeout,
		SoftwareDeps: []string{"reboot", "chrome", "chrome_internal", "tpm2"},
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_vm_r"},
		}},
		SearchFlags: []*testing.StringPair{
			{
				Key: "feature_id",
				// Verify that traffic from the device goes through the managed proxy (COM_FOUND_CUJ6_TASK3_WF1).
				Value: "screenplay-4b1ea96e-e06c-4c3a-b153-626de2ebdaf8",
			},
		},
	})
}

func SystemProxyForArc(ctx context.Context, s *testing.State) {
	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(ctx)
	if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	bc := pb.NewTestArcConnectivityAppServiceClient(cl.Conn)

	if _, err := bc.RunTest(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to run test: ", err)

	}
}
