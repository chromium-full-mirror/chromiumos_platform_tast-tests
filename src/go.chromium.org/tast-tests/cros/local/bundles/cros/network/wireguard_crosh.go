// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"reflect"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/vpn"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/exec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		// For the test naming, we cannot use WireGuardCrosh because it is not
		// consistent with the file name.
		Func:     WireguardCrosh,
		Desc:     "Verify using wireguard command in crosh to manage a wireguard setrvice",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking
		BugComponent: "b:156085",
		Attr:         []string{"group:network", "network_platform_unstable"},
		SoftwareDeps: []string{"wireguard"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func WireguardCrosh(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}
	popProfile, err := m.PushTestProfile(ctx)
	if err != nil {
		s.Fatal("Failed to push test profile: ", err)
	}
	defer popProfile(cleanupCtx)
	networkEnv, err := vpn.CreateNetworkTopology(ctx)
	if err != nil {
		s.Fatal("Failed to create network topology for VPN tests: ", err)
	}
	defer func() {
		if err := networkEnv.TearDown(cleanupCtx); err != nil {
			s.Error("Failed to tear down network topology for VPN tests: ", err)
		}
	}()
	peer1Env := networkEnv.Server1
	peer2Env := networkEnv.Server2

	const wgSvcName = "wg-test"
	const dnsServer = "192.168.0.1"

	// Helper function which returns shill service with name wgSvcName, nil if not
	// found.
	getShillSvc := func() *shill.Service {
		svc, err := m.FindMatchingService(ctx, map[string]interface{}{
			shillconst.ServicePropertyType: shillconst.TypeVPN,
			shillconst.ServicePropertyName: wgSvcName,
		})
		if err != nil && err.Error() != shillconst.ErrorMatchingServiceNotFound {
			s.Fatal("Failed to find WireGuard service in shill: ", err)
		}
		return svc
	}

	// Helper functions to run wireguard command in crosh. Some commands need
	// interaction using stdin, input will be piped into stdin of crosh.
	execWGCmdWithInput := func(input string, args ...string) string {
		croshArgs := []string{"--", "wireguard"}
		croshArgs = append(croshArgs, args...)
		cmd := testexec.CommandContext(ctx, "crosh", croshArgs...)
		if len(input) != 0 {
			stdin, err := cmd.StdinPipe()
			if err != nil {
				s.Fatal("Failed to open stdin pipe: ", err)
			}
			if _, err := stdin.Write([]byte(input)); err != nil {
				s.Fatal("Failed to write to stdin: ", err)
			}
		}
		o, err := cmd.Output(exec.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to run wireguard %v: %v", args, err)
		}
		return string(o)
	}

	execWGCmd := func(args ...string) string {
		return execWGCmdWithInput("", args...)
	}

	testing.ContextLog(ctx, "Verifying service creation via `wireguard new`")
	svc := func() *shill.Service {
		execWGCmd("new", wgSvcName)
		svc := getShillSvc()
		if svc == nil {
			s.Fatal("Failed to find created service in shill")
		}
		return svc
	}()

	testing.ContextLog(ctx, "Verifying getting public key via `wireguard show`")
	clientPublicKey := func() string {
		showOutput := execWGCmd("show", wgSvcName)
		matches := regexp.MustCompile(`public key: (\S+)`).FindStringSubmatch(showOutput)
		if len(matches) != 2 {
			s.Fatal("Failed to parse result from `wireguard show`: ", showOutput)
		}
		return matches[1]
	}()

	startWGServer := func(env *virtualnet.Env, usePSK, isSecondServer bool) (*vpn.Server, func()) {
		opts := []vpn.Option{
			vpn.WithIPType(vpn.IPTypeIPv4AndIPv6),
			vpn.WithWGClientPublicKey(clientPublicKey),
			vpn.WithWGUsePSK(usePSK),
		}
		if isSecondServer {
			opts = append(opts, vpn.WGSecondServerDefaultOptions...)
		}
		svr, err := vpn.StartServer(ctx, env, vpn.TypeWireGuard, opts...)
		if err != nil {
			s.Fatal("Failed to start WireGuard server: ", err)
		}
		return svr, func() {
			if err := svr.Exit(cleanupCtx); err != nil {
				s.Error("Failed to stop WireGuard server: ", err)
			}
		}
	}
	peer1, cleanup1 := startWGServer(peer1Env, false, false)
	defer cleanup1()
	peer2, cleanup2 := startWGServer(peer2Env, true, true)
	defer cleanup2()

	testing.ContextLog(ctx, "Verifying service configuration via `wireguard set`")
	func() {
		properties, err := vpn.CreateProperties(peer1, peer2)
		if err != nil {
			s.Fatal("Failed to generate properties dict: ", err)
		}
		clientIPs := strings.Join(properties["WireGuard.IPAddress"].([]string), ",")
		execWGCmd("set", wgSvcName, "local-ip", clientIPs, "dns", dnsServer)

		peerProps := properties["WireGuard.Peers"].([]map[string]string)
		if len(peerProps) != 2 {
			s.Fatalf("Unexpected generated properties: len(peerProps)=%v, want 2", len(peerProps))
		}
		for _, p := range peerProps {
			args := []string{
				"set", wgSvcName, "peer", p["PublicKey"], "endpoint", p["Endpoint"],
				"allowed-ips", p["AllowedIPs"], "persistent-keepalive", "10",
			}
			// Note that preshared-key is input via stdin.
			psk := p["PresharedKey"]
			if len(psk) != 0 {
				args = append(args, "preshared-key")
			}
			execWGCmdWithInput(psk+"\n", args...)
		}
	}()

	testing.ContextLog(ctx, "Verifying service connect via `wireguard connect`")
	func() {
		execWGCmd("connect", wgSvcName)
		if err := svc.WaitForConnectedOrError(ctx); err != nil {
			s.Fatal("Failed to wait for WireGuard service connected: ", err)
		}
		// Since wg is stateless, connected state does not mean VPN tunnel is setup.
		// Check routing here to make sure service is configured properly.
		for _, addr := range []string{peer1.OverlayIPv4, peer1.OverlayIPv6, peer2.OverlayIPv4, peer2.OverlayIPv6} {
			if err := ping.ExpectPingSuccessWithTimeout(ctx, addr, "chronos", 10*time.Second); err != nil {
				s.Fatal("Failed to verify ping after connect: ", err)
			}
		}
		ipconfigs, err := svc.GetIPConfigs(ctx)
		if err != nil {
			s.Fatal("Failed to get IPConfigs after service is connected: ", err)
		}
		// Check DNS is configured by reading IPConfig objects from shill.
		var gotDNS []string
		wantDNS := []string{dnsServer}
		for _, ipconfig := range ipconfigs {
			props, err := ipconfig.GetIPProperties(ctx)
			if err != nil {
				s.Fatal("Failed to get properties on the IPConfig object: ", err)
			}
			gotDNS = append(gotDNS, props.NameServers...)
		}
		if !reflect.DeepEqual(gotDNS, wantDNS) {
			s.Fatalf("Unexpected DNS servers: got %v, want %v", gotDNS, wantDNS)
		}
	}()

	testing.ContextLog(ctx, "Verifying service disconnect via `wireguard disconnect`")
	func() {
		execWGCmd("disconnect", wgSvcName)
		if err := svc.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateIdle, 5*time.Second); err != nil {
			s.Fatal("Failed to wait for WireGuard service disconnected")
		}
	}()

	testing.ContextLog(ctx, "Verifying service removal via `wireguard del`")
	func() {
		execWGCmd("del", wgSvcName)
		if getShillSvc() != nil {
			s.Fatal("Shill service still exists")
		}
	}()
}
