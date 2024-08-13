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
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/radvd"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     EnableRFC8925OnWifi,
		Desc:     "Verify that RFC8925 is enabled on a WiFi service properly",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi"},
		Fixture:      "shillSimulatedWiFi",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{{
			Name: "non_link_local_dns",
			Val:  false,
		}, {
			Name: "link_local_dns",
			Val:  true,
		}},
	})
}

// EnableRFC8925OnWifi verifies that for a WiFi service: 1) the first connection
// will always have RFC8925 disabled; b) the seconds connection will have
// RFC8925 enabled if in the first connection there is a non-link-local IPv6 DNS
// server.
// TODO(b/345372970): This test mainly verifies the behavior of
// crrev.com/c/5711233, which is workaround for b/345372970. After the
// underlying dnsproxy issue is fixed, we can rework this test to be more
// general (also on Ethernet, etc.).
func EnableRFC8925OnWifi(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	useLinkLocalIPv6DNS := s.Param().(bool)

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

	// Avoid the created service being persisted into the profile.
	removeProfile, err := shill.LogOutUserAndPushTestProfile(ctx)
	if err != nil {
		s.Fatal("Failed to prepare test profile: ", err)
	}
	defer removeProfile(cleanupCtx)

	// Create a virtual WiFi environment without starting any server. We need to
	// tune the servers behavior in this test.
	simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	pool := subnet.NewPool()
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], m, pool, virtualnet.EnvOptions{})
	if err != nil {
		s.Fatal("Failed to create virtual WiFi router: ", err)
	}
	defer func() {
		if err := wifi.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up virtual WiFi router: ", err)
		}
	}()

	// Allocate subnets for IP provisioning.
	v4Subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for DHCP: ", err)
	}
	v6Subnet, err := pool.AllocNextIPv6Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for SLAAC: ", err)
	}

	// Start dnsmasq for DHCP server. Configure it with RFC8925.
	dnsmasqServer := dnsmasq.New(
		dnsmasq.WithDHCPServer(v4Subnet),
		dnsmasq.WithDHCPIPv6OnlyPreferred(300),
	)
	if err := wifi.Router.StartServer(ctx, "dnsmasq", dnsmasqServer); err != nil {
		s.Fatal("Failed to start dnsmasq in router: ", err)
	}

	// Start radvd for SLAAC. Configure DNS servers based on the test param. Note
	// that the DNS address does not need to be reachable so random address is
	// fine.
	var v6DNS []string
	if useLinkLocalIPv6DNS {
		v6DNS = []string{"fe80::1"}
	} else {
		v6DNS = []string{"fd00::1"}
	}
	radvd := radvd.New(v6Subnet, v6DNS)
	if err := wifi.Router.StartServer(ctx, "radvd", radvd); err != nil {
		s.Fatal("Failed to start radvd in router: ", err)
	}

	// Helper function to connect the WiFi service and verify that if IPv4 is
	// provisioned.
	verifyConnectAndIPv4 := func(expectIPv4 bool) error {
		if err := wifi.Service.Connect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to the wifi service")
		}
		if err := wifi.Service.WaitForConnectedOrError(ctx); err != nil {
			return errors.Wrap(err, "failed to wait for wifi service connected")
		}

		const provisionTimeout = 5 * time.Second
		s.Logf("Waiting %v seconds for DHCP provision", provisionTimeout.Seconds())
		// GoBigSleepLint: We want to verify that IPv4 is not provisioned in a
		// certain time, so must use a sleep here (without checking the
		// implementation details, e.g,, logs). Do sleep also for the hasIPv4 case
		// for simplicity (also to check if the timeout value is long enough).
		testing.Sleep(ctx, provisionTimeout)

		config, err := wifi.Service.GetNetworkConfig(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get NetworkConfig on the WiFi Service")
		}
		if !config.HasIPv6Address() {
			return errors.Wrapf(err, "unexpected NetworkConfig for IPv6: got %+v, want IPv6 address", config)
		}
		if config.HasIPv4Address() != expectIPv4 {
			wantStr := "IPv4 address"
			if !expectIPv4 {
				wantStr = "no IPv4 address"
			}
			return errors.Wrapf(err, "unexpected NetworkConfig for IPv4: got %+v, want %s", config, wantStr)
		}

		return nil
	}

	s.Log("Connecting to the WiFi for the first time")
	if err := verifyConnectAndIPv4(true /*expectIPv4*/); err != nil {
		s.Fatal("Failed to verify the first connection: ", err)
	}

	s.Log("Disconnecting from the WiFi service")
	if err := wifi.Service.Disconnect(ctx); err != nil {
		s.Fatal("Failed to disconnect the first connection: ", err)
	}

	s.Log("Connecting to the WiFi for the second time")
	if err := verifyConnectAndIPv4(useLinkLocalIPv6DNS /*expectIPv4*/); err != nil {
		s.Fatal("Failed to verify the second connection: ", err)
	}
}
