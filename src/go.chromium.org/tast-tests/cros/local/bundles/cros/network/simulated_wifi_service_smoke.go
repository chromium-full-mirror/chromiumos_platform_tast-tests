// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     SimulatedWifiServiceSmoke,
		Desc:     "A smoke test to verify the IP provision on a WiFi service based on hwsim",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"wifi"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Fixture:      "shillSimulatedWiFi",
		Params: []testing.Param{{
			Name: "ipv4",
			Val: virtualnet.EnvOptions{
				EnableDHCP: true,
			},
		}, {
			Name: "ipv6",
			Val: virtualnet.EnvOptions{
				RAServer: true,
			},
		}},
	})
}

func SimulatedWifiServiceSmoke(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewTcpdumpHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}

	simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	pool := subnet.NewPool()
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], m, pool, s.Param().(virtualnet.EnvOptions))
	if err != nil {
		s.Fatal("Failed to create virtual WiFi router: ", err)
	}
	defer func() {
		if err := wifi.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up virtual WiFi router: ", err)
		}
	}()

	if err := wifi.Service.Connect(ctx); err != nil {
		s.Fatal("Failed to connect to the wifi service: ", err)
	}

	if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
		s.Fatal("Failed to wait for wifi service connected: ", err)
	}
}
