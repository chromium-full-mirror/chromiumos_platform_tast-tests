// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/arcvpn"
	arcnet "go.chromium.org/tast-tests/cros/local/network/arc"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

const (
	pacProxyType    = "pac"
	manualProxyType = "manual"
)

// arcVPNProxySettingsTestCase describes the parameters of a single test case.
type arcVPNProxySettingsTestCase struct {
	// Type of proxy set in this test, either PAC URL or manual.
	proxyType string
	// Proxy configuration to be set in this test.
	proxyConfig map[string]interface{}
	// Expected proxy config property string in shill.
	proxyStr string
}

func init() {
	// Set of tests designed to test that proxy settings of ARC VPN services can be correctly set in shill.
	testing.AddTest(&testing.Test{
		Func:     ARCVPNProxySettings,
		Desc:     "Test if proxy settings of ARC VPN services can be correctly created in shill",
		Contacts: []string{"cros-networking@google.com", "chuweih@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Fixture:      "arcBooted",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"arc"},
		HardwareDeps: arc.ArcAppHwDeps,
		Params: []testing.Param{
			{
				Name: "manual_proxy",
				Val: arcVPNProxySettingsTestCase{
					proxyType: manualProxyType,
					proxyConfig: map[string]interface{}{"host": "hostname", "port": 8080,
						"exclusion_list": []string{
							"example.com",
							"localhost",
							"*.mydomain.com",
							"192.168.1.*"},
					},
					proxyStr: `{"bypass_list":"example.com;localhost;*.mydomain.com;192.168.1.*;","mode":"fixed_servers","server":"http=hostname:8080"}`,
				},
			},
			{
				Name: "pac_url_proxy",
				Val: arcVPNProxySettingsTestCase{
					proxyType:   pacProxyType,
					proxyConfig: map[string]interface{}{"pac_url": "http://test/test"},
					proxyStr:    `{"mode":"pac_script","pac_mandatory":false,"pac_url":"http://test/test"}`,
				},
			},
		},
	})
}

// ARCVPNProxySettings tests that proxy settings of ARC VPN services can be correctly set in shill.
func ARCVPNProxySettings(ctx context.Context, s *testing.State) {
	// If the main body of the test times out, we still want to reserve a
	// few seconds to allow for our cleanup code to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(cleanupCtx, 10*time.Second)
	defer cancel()

	a := s.FixtValue().(*arc.PreData).ARC
	tc := s.Param().(arcVPNProxySettingsTestCase)

	// Save the ARC network dumpsys as close to the time of error as possible, in case
	// further cleanup affects the network state.
	handler := arcnet.CreateNetworkDumpsysErrorHandler(cleanupCtx, a)
	s.AttachErrorHandlers(handler, handler)

	// Install and start the test app.
	cleanupFunc, err := arcvpn.InstallAndPreAuthorizeARCVPN(ctx, a)
	if err != nil {
		s.Fatal("Failed to set up ARC VPN test app: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	testing.ContextLog(ctx, "Starting ArcVpnTest app")
	if tc.proxyType == pacProxyType {
		pacURL := tc.proxyConfig["pac_url"].(string)
		if err := arcvpn.StartARCVPNWithPACURLProxy(ctx, a, pacURL); err != nil {
			s.Fatal("Failed to send ArcVpnTest app with PAC URL proxy: ", err)
		}
	} else {
		host := tc.proxyConfig["host"].(string)
		port := tc.proxyConfig["port"].(int)
		exclusionList := tc.proxyConfig["exclusion_list"].([]string)

		if err := arcvpn.StartARCVPNWithManualProxy(ctx, a, host, port, exclusionList...); err != nil {
			s.Fatal("Failed to send ArcVpnTest app with manual proxy: ", err)
		}
	}
	defer func() {
		if err := arcvpn.ForceStopARCVPN(cleanupCtx, a); err != nil {
			s.Error("Failed to clean up ARC VPN: ", err)
		}
	}()

	// Make sure our test app is connected.
	if err := arcvpn.WaitForARCServiceState(ctx, a, arcvpn.VPNTestAppPkg, arcvpn.VPNTestAppSvc, true); err != nil {
		s.Fatalf("Failed to start %s: %v", arcvpn.VPNTestAppSvc, err)
	}

	// Host traffic gets routed to the ARC VPN correctly. Make sure VPN service is functioning.
	if err := ping.ExpectPingSuccessWithTimeout(ctx, arcvpn.TunIP, "chronos", 10*time.Second); err != nil {
		s.Fatalf("Failed to ping %s from host: %v", arcvpn.TunIP, err)
	}

	m, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to shill Manager: ", err)
	}

	services, _, err := m.ServicesByTechnology(ctx, shill.TechnologyVPN)
	if err != nil {
		s.Fatal("Failed to get VPN services: ", err)
	}

	if len(services) != 1 {
		s.Fatal("Expect to find only one VPN service, but found:", len(services))
	}

	// Check that proxy config property in shill matches the expectation.
	service := services[0]
	p, err := service.GetProperties(ctx)
	if err != nil {
		s.Fatal("Failed to get VPN service properties: ", err)
	}

	proxyConfig, err := p.Get(shillconst.ServicePropertyProxyConfig)
	if err != nil {
		s.Fatal("Failed to get proxy config property from service: ", err)
	}
	expected := tc.proxyStr
	if proxyConfig != expected {
		s.Errorf("Proxy config string is %v, want: %v", proxyConfig, expected)
	}
}
