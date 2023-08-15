// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bruschetta"
	"go.chromium.org/tast-tests/cros/local/guestos"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BruschettaConnectivity,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks IPv4 and IPv6 connectivity inside Bruschetta VM",
		Contacts:     []string{"cros-networking@google.com", "taoyl@google.com", "clumptini@google.com"},
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "vm_host", "untrusted_vm", "dlc", "amd64"},
		HardwareDeps: bruschetta.BruschettaHwDeps,
		Fixture:      bruschetta.BruschettaFixture,
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				Name: "dualstack",
				Val:  guestos.ConnectivityTestParams{},
			}, {
				Name: "v6only",
				Val:  guestos.ConnectivityTestParams{V6Only: true},
			},
		},
	})
}

func BruschettaConnectivity(ctx context.Context, s *testing.State) {
	bru := s.FixtValue().(bruschetta.FixtureData).BruschettaVM
	v6only := s.Param().(guestos.ConnectivityTestParams).V6Only

	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up test topology.
	testEnv := routing.NewSimpleNetworkEnv(!v6only, true, !v6only, true)
	if err := testEnv.SetUp(ctx); err != nil {
		s.Fatal("Failed to set up routing test env: ", err)
	}
	defer func(ctx context.Context) {
		if err := testEnv.TearDown(ctx); err != nil {
			s.Error("Failed to tear down routing test env: ", err)
		}
	}(cleanupCtx)

	for _, err := range guestos.NetworkConnectivity(ctx, bru, v6only, testEnv) {
		s.Error("Connectivity test failed: ", err)
	}
}
