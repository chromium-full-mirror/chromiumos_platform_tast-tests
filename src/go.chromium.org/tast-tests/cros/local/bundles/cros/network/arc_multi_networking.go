// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"net"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// routerServerWrapper is a wrapper for a router and server network.
type routerServerWrapper struct {
	// router is the router env (the local subnet).
	router *env.Env
	// server is the server env (the local subnet).
	server *env.Env
	// nameSuffix is the suffix of the router.
	nameSuffix string
	// ifName is the "physical" interface name of the router starting with eth.
	ifName string
	// routerAddr is the server address of the router.
	routerAddr net.IP
	// serverAddr is the server address of the server.
	serverAddr net.IP
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCMultiNetworking,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies guest network connectivity upon physical interface change",
		Contacts:     []string{"cros-networking@google.com", "ningyuan@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		// TODO(b/331845752): Disable the test in CQ temporarily.
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      10 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_container"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
	})
}

// ARCMultiNetworking tests connectivity from ARC as multiple interfaces are
// connected, removed, reconnected, and ARC restart.
// Network topology: DUT - router A - server A, DUT - router B - server B.
// router A and router B are distinguished by its "physical" ifname on DUT.
// The connectivity is checked by pinging the router IP address, and server IP
// address.
// Test procedures are:
// 1. Connect router A, then router B, then check connection.
// 2. Remove router A, then router B, then check removal of interface in ARC.
// 3. Connect router B, then router A, then check connection.
// 4. Reboot ARC, then check connection.
func ARCMultiNetworking(ctx context.Context, s *testing.State) {
	const (
		networkInitializationPollTimeout = 10 * time.Second // The time to wait for patchpaneld to set up virtual network after physical network changes.
		networkRemovalPollTimeout        = 1 * time.Second  // The time to wait for configurations after physical network is removed.
		// b:329799545: Delay added to simulate the actual delay when
		// physical devices are added. Virtual device added simultaneously may
		// be mismatched in ARC guest.
		networkAdditionDelay = 400 * time.Millisecond // The time to wait before adding each network device.
	)

	// Reserve some time for cleanup code.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 20*time.Second)
	defer cancel()

	startARC := func() *arc.ARC {
		cr, err := chrome.New(ctx, chrome.ARCEnabled(), chrome.UnRestrictARCCPU())
		if err != nil {
			s.Fatal("Failed to connect to Chrome: ", err)
		}
		defer cr.Close(cleanupCtx)
		a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
		if err != nil {
			s.Fatal("Failed to start ARC: ", err)
		}
		return a
	}
	closeARC := func(ctx context.Context, a *arc.ARC) {
		if err := a.Close(ctx); err != nil {
			s.Fatalf("Failed to close arc observer: %s", err)
		}
	}
	a := startARC()
	defer closeARC(cleanupCtx, a)
	shillManager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill client: ", err)
	}
	restoreEthernet, err := arcnet.HideUnusedEthernet(ctx, shillManager)
	if err != nil {
		s.Fatal("Failed to hide unused ethernet: ", err)
	}
	defer restoreEthernet(cleanupCtx)

	s.Log("Testing multinet behavior on device addition")

	routerAOpt := virtualnet.EnvOptions{
		NameSuffix: "A",
		EnableDHCP: true,
		RAServer:   false,
		EnableDNS:  false,
	}
	routerBOpt := virtualnet.EnvOptions{
		NameSuffix: "B",
		EnableDHCP: true,
		RAServer:   false,
		EnableDNS:  false,
	}
	pool := subnet.NewPool()

	getIPv4Address := func(e *env.Env) (net.IP, error) {
		ifaceAddrs, err := e.WaitForVethInAddrs(ctx, true, false)
		if err != nil {
			return nil, err
		}
		return ifaceAddrs.IPv4Addr, nil
	}

	createRouterServerWrapper := func(opt virtualnet.EnvOptions, pool *subnet.Pool) *routerServerWrapper {
		var rsw routerServerWrapper
		_, rsw.router, rsw.server, err = virtualnet.CreateRouterServerEnv(ctx, shillManager, pool, opt)
		if err != nil {
			s.Fatalf("Failed to create router %s: %s", opt.NameSuffix, err)
		}
		rsw.routerAddr, err = getIPv4Address(rsw.router)
		if err != nil {
			s.Errorf("Failed to get router %s IP: %s", opt.NameSuffix, err)
		}
		rsw.serverAddr, err = getIPv4Address(rsw.server)
		if err != nil {
			s.Errorf("Failed to get server %s IP: %s", opt.NameSuffix, err)
		}
		rsw.ifName = rsw.router.VethOutName
		rsw.nameSuffix = opt.NameSuffix
		return &rsw
	}
	cleanRouterServerWrapper := func(ctx context.Context, rsw *routerServerWrapper) {
		if rsw.router != nil {
			if err := rsw.router.Cleanup(ctx); err != nil {
				s.Errorf("Failed to clean router env: %s", err)
			}
		}
		rsw.router = nil
		if rsw.server != nil {
			if err := rsw.server.Cleanup(ctx); err != nil {
				s.Errorf("Failed to clean server env: %s", err)
			}
		}
		rsw.server = nil
	}
	routerA := createRouterServerWrapper(routerAOpt, pool)
	defer cleanRouterServerWrapper(cleanupCtx, routerA)
	// b:329799545: Delay added to simulate the actual delay when physical
	// devices are added. Virtual device added simultaneously may be mismatched
	// in ARC guest.
	// GoBigSleepLint: Wait between consequent NIC addition.
	testing.Sleep(ctx, networkAdditionDelay)
	routerB := createRouterServerWrapper(routerBOpt, pool)
	defer cleanRouterServerWrapper(cleanupCtx, routerB)

	checkARCConnect := func(rsw *routerServerWrapper, a *arc.ARC) {
		ipAddrs := []net.IP{rsw.routerAddr, rsw.serverAddr}
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			arcIfName, err := arcnet.GetARCInterfaceName(ctx, rsw.ifName)
			s.Logf("Check connection on %s, mapped to ARC interface %s", rsw.ifName, arcIfName)
			if err != nil {
				return errors.Wrap(err, "failed to get ARC interface name")
			}
			// Verify pinging to ipAddrs.
			for _, ip := range ipAddrs {
				if err := arcnet.ExpectPingSuccess(ctx, a, arcIfName, ip.String()); err != nil {
					return errors.Wrapf(err, "failed to ping %s", ip.String())
				}
			}
			return nil
		}, &testing.PollOptions{Timeout: networkInitializationPollTimeout}); err != nil {
			s.Fatalf("ARC Connectivity check failed for interface %s: %s", rsw.ifName, err)
		}
	}
	checkARCConnect(routerA, a)
	checkARCConnect(routerB, a)

	s.Log("Removing interfaces and verify their removal")
	cleanRouterServerWrapper(ctx, routerA)
	cleanRouterServerWrapper(ctx, routerB)

	expectInterfaceRemoved := func(ifName string) {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			arcIfName, err := arcnet.GetARCInterfaceName(ctx, ifName)
			if err == nil {
				return errors.Errorf("interface %s exist, arc ifname: %s", ifName, arcIfName)
			}
			if strings.Contains(err.Error(), "no ARC device matching") {
				return nil
			}
			return errors.Wrap(err, "failed to get arc interface")
		}, &testing.PollOptions{Timeout: networkRemovalPollTimeout}); err != nil {
			s.Fatalf("Failed to verify interface removal for %s: %s", ifName, err)
		}
	}
	expectInterfaceRemoved(routerA.ifName)
	expectInterfaceRemoved(routerB.ifName)

	s.Log("Recreating network interface and topology")
	routerBNew := createRouterServerWrapper(routerBOpt, pool)
	defer cleanRouterServerWrapper(cleanupCtx, routerBNew)
	// b:329799545: Delay added to simulate the actual delay when physical
	// devices are added. Virtual device added simultaneously may be mismatched
	// in ARC guest.
	// GoBigSleepLint: Wait between consequent NIC addition.
	testing.Sleep(ctx, networkAdditionDelay)
	routerANew := createRouterServerWrapper(routerAOpt, pool)
	defer cleanRouterServerWrapper(cleanupCtx, routerANew)
	checkARCConnect(routerANew, a)
	checkARCConnect(routerBNew, a)

	s.Log("Rebooting ARC")
	aNew := startARC()
	defer closeARC(cleanupCtx, aNew)
	checkARCConnect(routerANew, aNew)
	checkARCConnect(routerBNew, aNew)

}
