// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPHostname,
		Desc:     "Verify the hostname option sent by the DHCP client",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > baseOS > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "group:release-health", "release-health_network"},
		// DHCP hostname property is written into the default profile but not user
		// profile. Use shillReset to guarantee it is clean before and after the
		// test.
		Fixture: "shillReset.ehide",
		Params: []testing.Param{
			{
				Val: testhooks.Dhcpcd7,
			},
			{
				Name: "dhcpcd10",
				Val:  testhooks.Dhcpcd10,
			},
		},
	})
}

// DHCPHostname verifies the hostname option sent by the DHCP client is the
// value set in shill. Since this property is a global property in shill, this
// test also verifies that the value does not change across profiles.
func DHCPHostname(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dhcpcdVersion := s.Param().(testhooks.DhcpcdVersion)
	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewSetDhcpcdVersionHook(dhcpcdVersion),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	// Prepare the environment.
	pool := subnet.NewPool()
	svc, rt, err := virtualnet.CreateRouterEnv(ctx, manager, pool, virtualnet.EnvOptions{})
	if err != nil {
		s.Fatal("Failed to create router env: ", err)
	}
	defer func() {
		if err := rt.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up router: ", err)
		}
	}()
	subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for DHCP: ", err)
	}
	dnsmasqServer := dnsmasq.New(dnsmasq.WithDHCPServer(subnet))
	if err := rt.StartServer(ctx, "dnsmasq", dnsmasqServer); err != nil {
		s.Fatal("Failed to start dnsmasq: ", err)
	}

	reconnectService := func() {
		if err := svc.Disconnect(ctx); err != nil {
			s.Fatal("Failed to disconnect the service: ", err)
		}
		if err := waitForIPv4Teardown(ctx, rt.VethOutName); err != nil {
			s.Fatal("Failed to wait for IPv4 address teardown: ", err)
		}
		state, err := svc.GetState(ctx)
		if err != nil {
			s.Fatal("Failed to get service state: ", err)
		}
		if state == shillconst.ServiceStateIdle || state == shillconst.ServiceStateFailure {
			if err := svc.Connect(ctx); err != nil {
				s.Fatal("Failed to connect the service: ", err)
			}
		}
		if err := svc.WaitForConnectedOrError(ctx); err != nil {
			s.Fatal("Failed to wait for service in test online: ", err)
		}
	}

	setHostname := func(val string) {
		testing.ContextLogf(ctx, "Set hostname to %q and reconnect the service", val)
		if err := manager.SetProperty(ctx, shillconst.ManagerPropertyDHCPHostname, val); err != nil {
			s.Fatal("Failed to set hostname property in shill: ", err)
		}
		reconnectService()
	}

	var pushedProfiles []string
	defer func() {
		for i := len(pushedProfiles) - 1; i >= 0; i-- {
			name := pushedProfiles[i]
			if err := manager.PopProfile(cleanupCtx, name); err != nil {
				testing.ContextLogf(cleanupCtx, "Failed to pop %s profile during cleanup: %v", name, err)
			}
			if err := manager.RemoveProfile(cleanupCtx, name); err != nil {
				testing.ContextLogf(cleanupCtx, "Failed to remove %s profile during cleanup: %v", name, err)
			}
		}
	}()

	pushProfile := func(name string) {
		testing.ContextLogf(ctx, "Push profile %s and reconnect the service", name)
		if _, err := manager.CreateProfile(ctx, name); err != nil {
			s.Fatalf("Failed to create %s profile: %v", name, err)
		}
		if _, err := manager.PushProfile(ctx, name); err != nil {
			if rmErr := manager.RemoveProfile(cleanupCtx, name); rmErr != nil {
				testing.ContextLogf(cleanupCtx, "Failed to remove %s profile after PushProfile failure: %v", name, rmErr)
			}
			s.Fatalf("Failed to push %s profile: %v", name, err)
		}
		pushedProfiles = append(pushedProfiles, name)
		reconnectService()
	}

	popProfile := func(name string) {
		testing.ContextLogf(ctx, "Pop profile %s and reconnect the service", name)
		if err := manager.PopProfile(ctx, name); err != nil {
			s.Fatalf("Failed to pop %s profile: %v", name, err)
		}
		if err := manager.RemoveProfile(ctx, name); err != nil {
			s.Fatalf("Failed to remove %s profile: %v", name, err)
		}
		if len(pushedProfiles) > 0 && pushedProfiles[len(pushedProfiles)-1] == name {
			pushedProfiles = pushedProfiles[:len(pushedProfiles)-1]
		}
		reconnectService()
	}

	// Helper function to check that if the leases in dnsmasq contain val as the hostname.
	hasHostname := func(val string) bool {
		leases, err := dnsmasqServer.GetLeases(ctx)
		if err != nil {
			s.Fatal("Failed to get leases from dnsmasq: ", err)
		}
		for _, l := range leases {
			if l.Hostname == val {
				return true
			}
		}
		return false
	}

	const hostname1 = "test-hostname-1"
	const hostname2 = "test-hostname-2"
	const profileName1 = "profile1"
	const profileName2 = "profile2"

	setHostname(hostname1)
	if !hasHostname(hostname1) {
		s.Fatal("Leases in dnsmasq does not contain the expected hostname")
	}

	pushProfile(profileName1)
	if !hasHostname(hostname1) {
		s.Fatal("Leases in dnsmasq does not contain the expected hostname")
	}

	setHostname(hostname2)
	if !hasHostname(hostname2) {
		s.Fatal("Leases in dnsmasq does not contain the expected hostname")
	}

	popProfile(profileName1)
	if !hasHostname(hostname2) {
		s.Fatal("Leases in dnsmasq does not contain the expected hostname")
	}

	pushProfile(profileName2)
	if !hasHostname(hostname2) {
		s.Fatal("Leases in dnsmasq does not contain the expected hostname")
	}

	setHostname("")
	if hasHostname(hostname1) || hasHostname(hostname2) {
		s.Fatal("Leases in dnsmasq still contains the hostname which should be removed")
	}

	popProfile(profileName2)
	if hasHostname(hostname1) || hasHostname(hostname2) {
		s.Fatal("Leases in dnsmasq still contains the hostname which should be removed")
	}
}

func waitForIPv4Teardown(ctx context.Context, ifaceName string) error {
	return testing.Poll(ctx, func(ctx context.Context) error {
		iface, err := net.InterfaceByName(ifaceName)
		if err != nil {
			return errors.Wrapf(err, "failed to get interface %s", ifaceName)
		}
		addrs, err := iface.Addrs()
		if err != nil {
			return errors.Wrapf(err, "failed to get addresses on %s", ifaceName)
		}
		for _, addr := range addrs {
			if ipnet, ok := addr.(*net.IPNet); ok && ipnet.IP.To4() != nil {
				return errors.Errorf("interface %s still has IPv4 address %s", ifaceName, ipnet.IP)
			}
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 50 * time.Millisecond})
}
