// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/dhcp"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     DHCPRenew,
		Desc:     "Verify the DHCP behavior in the case that no DHCP server during renew",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:network", "network_platform_unstable"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

// DHCPRenew verify the DHCP packets sent out by DUT in the following scenario:
//
//  1. DUT (DHCP client) does the DHCP negotiation with the server, gets a lease,
//     and enters the BOUND state.
//  2. DHCP server disappears from the network.
//  3. T1 expires, DUT enters RENEWING state, sends out REQUEST packet, but does
//     not get replies.
//  4. T2 expires, DUT enters REBINDING state, sends out REQUEST packet, but does
//     not get replies.
//  5. Lease expires, DUT enters INIT state, sends out DISCOVER packet, but does
//     not get replies.
//  6. Shill turns the network state into Failure.
func DHCPRenew(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create manager proxy: ", err)
	}

	// Prepare the environment.
	pool := subnet.NewPool()
	svc, rt, err := virtualnet.CreateRouterEnv(ctx, manager, pool, virtualnet.EnvOptions{})
	defer func() {
		if err := rt.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up router: ", err)
		}
	}()
	subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate subnet for DHCP: ", err)
	}

	gatewayIP := subnet.GetAddrEndWith(1)
	intendedIP := subnet.GetAddrEndWith(2)

	// Install gateway address and routes.
	if err := rt.ConfigureInterface(ctx, rt.VethInName, gatewayIP, subnet); err != nil {
		s.Fatal("Failed to install address on router: ", err)
	}

	// Note that dhcpcd has a 20 seconds minimal accepted lease time.
	const (
		leaseSeconds   = 20
		leaseT1Seconds = 10
		leaseT2Seconds = 15
	)

	dhcpOpts := dhcp.NewOptionMap(
		gatewayIP, intendedIP,
		dhcp.WithOptionLeaseTime(leaseSeconds),
		dhcp.WithOptionT1(leaseT1Seconds),
		dhcp.WithOptionT2(leaseT2Seconds),
	)

	// Verify that:
	// - DHCP server receives the DISCOVER and REQUEST packets in order.
	// - Shill service state becomes connected.
	testing.ContextLog(ctx, "Verifying DHCP negotiation")

	discoveryRule := dhcp.NewRespondToDiscovery(intendedIP.String(), gatewayIP.String(),
		dhcpOpts, dhcp.FieldMap{}, true /*shouldRespond*/)
	requestRule := dhcp.NewRespondToRequest(intendedIP.String(), gatewayIP.String(),
		dhcpOpts, dhcp.FieldMap{}, true, /*shouldRespond*/
		gatewayIP.String(), intendedIP.String(), true /*expSvrIPSet*/)
	requestRule.SetIsFinalHandler(true)

	if _, errs := dhcp.RunTestWithEnv(ctx, rt, []dhcp.HandlingRule{*discoveryRule, *requestRule}, func(ctx context.Context) error {
		return svc.WaitForConnectedOrError(ctx)
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation: ", err)
		}
		return
	}

	// Verify that:
	// - DHCP server receives two REQUEST packets for RENEWING and REBINDING,
	//   and then a DISCOVER packet in order.
	// - Shill service state becomes failure.
	testing.ContextLog(ctx, "Verifying DHCP renewal")

	t1Rule := dhcp.NewRespondToRequest(intendedIP.String(), gatewayIP.String(),
		dhcp.OptionMap{}, dhcp.FieldMap{}, false /*shouldRespond*/, "", "", false /*expSvrIPSet*/)
	t2Rule := dhcp.NewRespondToPostT2Request(intendedIP.String(), gatewayIP.String(),
		dhcp.OptionMap{}, dhcp.FieldMap{}, false /*shouldRespond*/, "")
	discoveryRule.SetIsFinalHandler(true)

	// The time values here are not accurate, so we allow +1/-1 delta in the
	// verification.
	leaseStartTime := time.Now()
	t1 := leaseStartTime.Add(leaseT1Seconds * time.Second)
	t1Rule.SetTargetTime(t1.Add(-1*time.Second), t1.Add(time.Second))
	t2 := leaseStartTime.Add(leaseT2Seconds * time.Second)
	t2Rule.SetTargetTime(t2.Add(-1*time.Second), t2.Add(time.Second))

	if _, errs := dhcp.RunTestWithEnv(ctx, rt, []dhcp.HandlingRule{*t1Rule, *t2Rule, *discoveryRule}, func(ctx context.Context) error {
		pw, err := svc.CreateWatcher(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create watcher")
		}
		defer pw.Close(ctx)

		const timeout = leaseSeconds*time.Second + routing.DHCPTimeout + routing.DHCPExtraTimeout
		timeoutCtx, cancel := context.WithTimeout(ctx, timeout)
		defer cancel()
		if err := pw.Expect(timeoutCtx, shillconst.ServicePropertyState, shillconst.ServiceStateFailure); err != nil {
			return errors.Wrap(err, "failed to wait for service failure")
		}
		return nil
	}); len(errs) > 0 {
		for _, err := range errs {
			s.Error("Failed to verify DHCP negotiation for renewal: ", err)
		}
	}
}
