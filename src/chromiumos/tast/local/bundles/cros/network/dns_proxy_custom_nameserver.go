// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"runtime"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/local/bundles/cros/network/dns"
	"chromiumos/tast/local/crostini"
	"chromiumos/tast/local/multivm"
	"chromiumos/tast/local/network/virtualnet/subnet"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DNSProxyCustomNameserver,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify dns-proxy is elided when a custom nameserver is used",
		Contacts:     []string{"cros-networking@google.com", "jasongustaman@google.com", "garrick@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "vm_host", "arc", "dlc", "no_kernel_upstream"},
		Data:         []string{crostini.GetContainerMetadataArtifact("buster", false), crostini.GetContainerRootfsArtifact("buster", false), digExecutable()},
		Pre:          multivm.ArcCrostiniStartedWithDNSProxy(),
		HardwareDeps: crostini.CrostiniStable,
		Timeout:      5 * time.Minute,
	})
}

// DNSProxyCustomNameserver verifies that when a plaintext DNS query is made to a specific nameserver,
// explicitly overriding the network configuration, that dns-proxy is elided and the query goes directly to that server.
func DNSProxyCustomNameserver(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a few
	// seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	// Set up virtualnet environment.
	pool := subnet.NewPool()
	env, err := dns.NewEnv(ctx, pool)
	if err != nil {
		s.Fatal("Failed to setup DNS env: ", err)
	}
	defer env.Cleanup(cleanupCtx)

	// Create a new DNS server that acts as the custom name server.
	v4Subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate IPv4 subnet: ", err)
	}
	v6Subnet, err := pool.AllocNextIPv6Subnet()
	if err != nil {
		s.Fatal("Failed to allocate IPv6 subnet: ", err)
	}
	server, err := dns.NewServer(ctx, "c_server", v4Subnet, v6Subnet, env.Router, env.Certs)
	if err != nil {
		s.Fatal("Failed to create custom DNS nameserver: ", err)
	}
	defer server.Cleanup(cleanupCtx)

	addrs, err := server.GetVethInAddrs(ctx)
	if err != nil {
		s.Fatal("Failed to get custom DNS nameserver's IP address: ", err)
	}

	// Ensure plaintext query.
	pre := s.PreValue().(*multivm.PreData)
	cr := pre.Chrome
	a := multivm.ARCFromPre(pre)
	tconn := pre.TestAPIConn
	if err := dns.SetDoHMode(ctx, cr, tconn, dns.DoHOff, dns.GoogleDoHProvider); err != nil {
		s.Fatal("Failed to set DNS-over-HTTPS mode: ", err)
	}

	p, err := dns.InstallDigInARC(ctx, a, s.DataPath(digExecutable()))
	if err != nil {
		s.Error("Failed to install dig in ARC: ", err)
	}

	// By default, host DNS queries work as-is.
	// TODO(b/232882301) - Add Crostini
	tc := []dns.ProxyTestCase{
		{Client: dns.System},
		{Client: dns.User},
		{Client: dns.Chrome},
		{Client: dns.ARC},
	}
	if errs := dns.TestQueryDNSProxy(ctx, tc, a, nil, dns.NewQueryOptions()); len(errs) > 0 {
		s.Error("Failed initial DNS check: ", errs)
	}

	// Confirm that host queries to a different nameserver also work.
	opts := dns.NewQueryOptions()
	opts.Nameserver = addrs.IPv4Addr.To4().String()
	opts.ARCDigPath = p
	if errs := dns.TestQueryDNSProxy(ctx, tc, a, nil, opts); len(errs) > 0 {
		s.Error("Failed nameserver confirmation check: ", errs)
	}

	// Now block plaintext DNS traffic to that server and confirm failure to verify.
	nss, err := dns.ProxyNamespaces(ctx)
	if err != nil {
		s.Fatal("Failed to get DNS proxy's network namespaces: ", err)
	}
	if errs := dns.NewPlaintextBlock(nss, []string{env.Router.VethOutName}, opts.Nameserver).Run(ctx, func(ctx context.Context) {
		for i := 0; i < len(tc); i++ {
			tc[i].ExpectErr = true
		}
		opts.Domain = dns.RandDomain()
		if errs := dns.TestQueryDNSProxy(ctx, tc, a, nil, opts); len(errs) > 0 {
			s.Error("Failed nameserver verification: ", errs)
		}
	}); len(errs) > 0 {
		s.Fatal("Failed to block DNS to nameserver: ", errs)
	}
}

// digExecutable returns the dig executable name based on arch.
func digExecutable() string {
	switch runtime.GOARCH {
	case "arm":
		return "dig_arm"
	case "arm64":
		return "dig_arm64"
	default:
		return "dig_amd64"
	}
}
