// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package virtualnet

import (
	"context"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/network/ehideconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/ehide"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type ehideEnv struct {
	*Env
}

// CreateRouterEnvWithInternet creates a virtualnet router which provides
// Internet connectivity. This function requires ehide running on the DUT and
// there is IPv4 connectivity on the physical network. The caller should call
// Cleanup() on the returned object after this environment is no longer needed.
//
// Implementation details:
//
// This function will create a virtualnet env served as router to DUT and uses
// the ehide netns as the upstream router, as follows:
//
//	DUT ----> router env --(nat)--> ehide --(nat)--> physical network.
//
// The created router only provides IPv4 to DUT now. The main reason is that
// RDNSS is not being handled now, so DNS won't work on an IPv6-only
// environment. Thus disable IPv6 fully for consistency.
func CreateRouterEnvWithInternet(ctx context.Context, pool *subnet.Pool) (retEnv *ehideEnv, retErr error) {
	if isOn, err := ehide.IsOn(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to check ehide state")
	} else if !isOn {
		return nil, errors.New("failed to assert ehide on")
	}

	router := env.New("router")
	if err := router.SetUp(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to set up router env")
	}

	e := &ehideEnv{router}
	defer func() {
		if retErr == nil {
			return
		}
		if err := e.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clean up ehide env after setup failure: ", err)
		}
	}()

	if err := e.setUp(ctx, pool); err != nil {
		return nil, errors.Wrap(err, "failed to set up ehide env")
	}

	testing.ContextLog(ctx, "Created virtualnet env with Internet connectivity")

	return e, nil
}

func (e *ehideEnv) setUp(ctx context.Context, pool *subnet.Pool) error {
	// Give env an alias to improve readability in this function.
	router := e.Env

	// Find the interface with IPv4 connectivity in ehide.
	physicalIface, err := findIPv4DefaultRouteIfaceInEhide(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to find interface with IPv4 default route in ehide netns")
	}

	// The implementation assumes having exclusive control over the iptables in
	// ehide netns.
	if err := assertNATTableEmptyInEhide(ctx); err != nil {
		return errors.Wrap(err, "failed to assert nat table empty in iptables")
	}

	// Interface names of the veth pair connecting the two namespaces.
	const routerVeth = "veth-ehide-r"
	const ehideVeth = "veth-ehide-e"

	subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		return errors.Wrap(err, "failed to allocate IPv4 subnet for connecting router and ehide netns")
	}

	routerAddr := subnet.GetAddrEndWith(2)
	ehideAddr := subnet.GetAddrEndWith(1)

	// Create veth pair to connect router netns and ehide netns.
	vethSetupCmds := [][]string{
		{"ip", "link", "add", routerVeth, "type", "veth", "peer", ehideVeth, "netns", ehideconst.NetNSName},
		{"ip", "link", "set", routerVeth, "netns", e.NetNSName},
	}
	for _, cmd := range vethSetupCmds {
		if err := testexec.CommandContext(ctx, cmd[0], cmd[1:]...).Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to set up veth pair")
		}
	}

	// Configure ehide netns.
	ehideCmds := [][]string{
		// Configure interface to contact router netns.
		{"ifconfig", ehideVeth, ehideAddr.String() + "/" + strconv.Itoa(subnet.PrefixLen()), "up"},
		// Install NAT rule so that return traffic can be routed back properly from
		// the upstream network.
		{"iptables", "-t", "nat", "-I", "POSTROUTING", "-o", physicalIface, "-j", "MASQUERADE", "-w"},
	}
	for _, cmd := range ehideCmds {
		ipCmd := append([]string{"netns", "exec", ehideconst.NetNSName}, cmd...)
		if err := testexec.CommandContext(ctx, "ip", ipCmd...).Run(testexec.DumpLogOnError); err != nil {
			return errors.Wrap(err, "failed to configure ehide netns")
		}
	}

	// Configure router netns.
	routerCmds := [][]string{
		// Configure interface to contact router netns.
		{"ifconfig", routerVeth, routerAddr.String() + "/" + strconv.Itoa(subnet.PrefixLen()), "up"},
		// Install NAT rule so that return traffic can be routed back properly from
		// the upstream network.
		{"iptables", "-t", "nat", "-I", "POSTROUTING", "-o", routerVeth, "-j", "MASQUERADE", "-w"},
		// Install default route to route egress traffic properly to the upstream
		// network.
		{"ip", "route", "add", "default", "via", ehideAddr.String(), "dev", routerVeth},
	}
	for _, cmd := range routerCmds {
		if err := router.RunWithoutChroot(ctx, cmd...); err != nil {
			return errors.Wrap(err, "failed to configure router netns")
		}
	}

	// Copy the resolv.conf for the physical network to the router netns, so that
	// dnsmasq in the router netns can use it to do DNS query from upstream.
	targetPath := router.ChrootPath("/etc/resolv.conf")
	if err := testexec.CommandContext(ctx, "cp", ehideconst.ResolvConfPath, targetPath).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to set resolv.conf in router netns")
	}
	dhcpPool, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		return errors.Wrap(err, "failed to allocate pool for DHCP")
	}

	// Set up DHCP server and DNS server separately. We want the DNS server to be
	// listening on all interfaces so that it can be used via VPN is a VPN server
	// is started in this env, but the DHCP functionality does not work
	// WithAllInterfaces().
	dhcpServer := dnsmasq.New(
		dnsmasq.WithDHCPServer(dhcpPool),
		dnsmasq.WithDHCPNameServers([]string{}, true),
	)
	if err := router.StartServer(ctx, "dhcp_server", dhcpServer); err != nil {
		return errors.Wrap(err, "failed to start dnsmasq as dhcp server")
	}

	dnsServer := dnsmasq.New(
		dnsmasq.WithLocalResolvConf(),
		dnsmasq.WithAllInterfaces(),
	)
	if err := router.StartServer(ctx, "dns_server", dnsServer); err != nil {
		return errors.Wrap(err, "failed to start dnsmasq as dns server")
	}

	return nil
}

func (e *ehideEnv) Cleanup(ctx context.Context) error {
	var errs []error

	iptablesCmd := []string{"iptables", "-F", "-t", "nat", "-w"}
	if err := runCmdInEhideNetNS(ctx, iptablesCmd...); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to flush iptables for cleanup"))
	}

	if err := e.Env.Cleanup(ctx); err != nil {
		errs = append(errs, errors.Wrap(err, "failed to clean up virtualnet env"))
	}

	return errors.Join(errs...)
}

// findIPv4DefaultRouteIfaceInEhide returns the interface which has an IPv4
// default route installed. Returns err if there is no such interface or there
// are multiple default routes.
func findIPv4DefaultRouteIfaceInEhide(ctx context.Context) (string, error) {
	cmd := createCmdInEhideNetNS(ctx, strings.Fields("ip -4 route list default")...)
	output, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to run `ip route` in ehide netns")
	}

	// Count the number of default routes in the output. Note that there is no
	// built-in function for counting elements in a slice, so do it on a string.
	str := string(output)
	if cnt := strings.Count(str, "default"); cnt != 1 {
		return "", errors.Errorf("got %d default routes, want 1, output: %s", cnt, str)
	}

	tokens := strings.Fields(str)
	for i, token := range tokens {
		if token == "dev" && i < len(tokens)-1 {
			return tokens[i+1], nil
		}
	}

	return "", errors.Errorf("unexpected `ip route` output: %s", str)
}

// assertNATTableEmptyInEhide checks if the POSTROUTING chain of the nat table
// of iptables is empty.
func assertNATTableEmptyInEhide(ctx context.Context) error {
	const cmdStr = "iptables -t nat -S POSTROUTING -w"
	cmd := createCmdInEhideNetNS(ctx, strings.Fields(cmdStr)...)
	output, err := cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return errors.Wrapf(err, "failed to run `%s` in ehide netns", cmdStr)
	}

	str := string(output)
	str = strings.TrimSpace(str)
	lineCnt := len(strings.Split(str, "\n"))
	// There will be a default policy rule in the table, so cnt=1 indicates that
	// there is no other rules.
	if lineCnt != 1 {
		return errors.Errorf("unexpected line number in output of `%s`: got %d, want 1, output %s", cmdStr, lineCnt, str)
	}
	return nil
}

func createCmdInEhideNetNS(ctx context.Context, args ...string) *testexec.Cmd {
	cmd := append([]string{"ip", "netns", "exec", ehideconst.NetNSName}, args...)
	return testexec.CommandContext(ctx, cmd[0], cmd[1:]...)
}

func runCmdInEhideNetNS(ctx context.Context, args ...string) error {
	if err := createCmdInEhideNetNS(ctx, args...).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to run command in ehide netns")
	}
	return nil
}
