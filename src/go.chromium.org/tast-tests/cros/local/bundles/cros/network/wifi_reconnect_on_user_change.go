// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package network

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/network/dns"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/network/hwsim"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/testhooks"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/httpserver"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     WifiReconnectOnUserChange,
		Desc:     "Verify the network setup on WiFi reconnect during login and logout",
		Contacts: []string{"cros-networking@google.com", "jiejiang@google.com"},
		// ChromeOS > Platform > System > Networking > Continuous Maintenance
		BugComponent: "b:1493959",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"wifi", "chrome"},
		Fixture:      "shillSimulatedWiFi.ehide",
		// This test performs multiple login and logout. Thus use a long timeout
		// here.
		Timeout: 6 * time.Minute,
	})
}

// WifiReconnectOnUserChange sets up a WiFi AP with EAP, and configures
// different credentials for the user profile and the device (default) profile
// (which is common setup in enterprise), so that a WiFi reconnect will be
// triggered during login or logout. The test verifies that routing, DNS, and
// web browsing in Chrome works after login and logout.
func WifiReconnectOnUserChange(ctx context.Context, s *testing.State) {
	// Use a shortened context for test operations to reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	hookEnv, err := testhooks.RunNetworkTestHooks(ctx,
		testhooks.NewSaveNetLogHook(),
		testhooks.NewDumpHostOnFailureHook(),
		testhooks.NewTcpdumpHook(),
		testhooks.NewDisablePortalDetectionHook(),
	)
	if err != nil {
		s.Fatal("Failed to run network test hooks: ", err)
	}
	s.AttachErrorHandlers(hookEnv.OnErrorHandler, hookEnv.OnFatalHandler)
	defer hookEnv.TearDownWithLogFailures(cleanupCtx, s.HasError)

	const (
		// Constants for EAP. We use different identity and password for user and
		// device profiles.
		userProfileIdentity   = "user"
		userProfilePassword   = "user-pwd"
		deviceProfileIdentity = "device"
		deviceProfilePassword = "device-pwd"

		// testHostName will be resolved to the server env, where there will be an
		// HTTP server which returns httpResponse on every request.
		testHostname = "test.wifi.reconnect"
		httpResponse = "Hello world"
	)

	mgr, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create shill manager proxy: ", err)
	}
	wifiMgr, err := shill.NewWifiManager(ctx, mgr)
	if err != nil {
		s.Fatal("Failed to obtain wifi manager: ", err)
	}

	// Prepare temp dir for hosting hostapd config files.
	tempDir, err := os.MkdirTemp(os.TempDir(), "")
	if err != nil {
		s.Fatal("Failed to create temp dir for hostapd files: ", err)
	}
	defer func() {
		if err := os.RemoveAll(tempDir); err != nil {
			s.Log("Failed to remove temp dir for hostapd files: ", err)
		}
	}()

	// Pre-allocate the IP subnets used for connecting the router and server envs.
	pool := subnet.NewPool()
	serverIPv4Subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		s.Fatal("Failed to allocate IPv4 subnet for the server env: ", err)
	}
	serverIPv6Subnet, err := pool.AllocNextIPv6Subnet()
	if err != nil {
		s.Fatal("Failed to allocate IPv6 subnet for the server env: ", err)
	}
	// Assumption: ConnectToRouter() will set the address ending with .2 in the
	// server env.
	serverIPv4 := serverIPv4Subnet.GetAddrEndWith(2)
	serverIPv6 := serverIPv6Subnet.GetAddrEndWith(2)

	// Create hostapd files for EAP-MSCHAPv2. This test is not for verifying EAP
	// WiFi so we only need to configure certs on the server side.
	hostapdConfLines := func() []string {
		certSet := certificate.TestCert1()

		eapUserDB := "* PEAP\n" +
			fmt.Sprintf("\"%s\" MSCHAPV2 \"%s\" [2]\n", userProfileIdentity, userProfilePassword) +
			fmt.Sprintf("\"%s\" MSCHAPV2 \"%s\" [2]\n", deviceProfileIdentity, deviceProfilePassword)

		eapUserDBPath := tempDir + "/eap_user"
		caCertPath := tempDir + "/ca.pem"
		serverCertPath := tempDir + "/server.pem"
		serverKeyPath := tempDir + "/server.key"
		for _, file := range []struct {
			path    string
			content string
		}{
			{eapUserDBPath, eapUserDB},
			{caCertPath, certSet.CACred.Cert},
			{serverCertPath, certSet.ServerCred.Cert},
			{serverKeyPath, certSet.ServerCred.PrivateKey},
		} {
			if err := os.WriteFile(file.path, []byte(file.content), 0644); err != nil {
				s.Fatalf("Failed to write %s: %v", file.path, err)
			}
		}

		return []string{
			"ieee8021x=1",
			"eap_server=1",
			"eap_user_file=" + eapUserDBPath,
			"ca_cert=" + caCertPath,
			"server_cert=" + serverCertPath,
			"private_key=" + serverKeyPath,
			"wpa=1",
			"wpa_key_mgmt=WPA-EAP",
		}
	}()

	// Start virtualnet with the AP.
	simWiFi := s.FixtValue().(*hwsim.ShillSimulatedWiFi)
	opts := virtualnet.EnvOptions{
		EnableDHCP:                true,
		RAServer:                  true,
		EnableDNS:                 true,
		ResolvedHost:              testHostname,
		ResolveHostToIP:           serverIPv4,
		HostapdAddtionalConfLines: hostapdConfLines,
	}
	wifi, err := virtualnet.CreateWifiRouterEnv(ctx, simWiFi.AP[0], mgr, pool, opts)
	if err != nil {
		s.Fatal("Failed to create virtual WiFi router: ", err)
	}
	defer func() {
		if err := wifi.Cleanup(cleanupCtx); err != nil {
			s.Error("Failed to clean up virtual WiFi router: ", err)
		}
	}()

	// Create a virtualnet Env to verify default route and host the HTTP server.
	serverEnv, err := virtualnet.CreateEnv(ctx, "server")
	if err != nil {
		s.Fatal("Failed to create virtualnet Env for server")
	}
	if err := serverEnv.ConnectToRouter(ctx, wifi.Router, serverIPv4Subnet, serverIPv6Subnet); err != nil {
		s.Fatal("Failed to connect server env to the router")
	}
	httpServer := httpserver.New(httpserver.TCP4, "80", func(rw http.ResponseWriter, req *http.Request) {
		if _, err := rw.Write([]byte(httpResponse)); err != nil {
			testing.ContextLog(ctx, "Failed to write response in HTTP server: ", err)
		}
	}, nil)
	if err := serverEnv.StartServer(ctx, "http", httpServer); err != nil {
		s.Fatal("Failed to start HTTP server in the server env: ", err)
	}

	// We cannot use the wifi.Service in the following test since the login /
	// logout below may change the service path. Reference the service by ssid
	// instead.
	ssid := wifi.SSID

	// Group of util functions.

	// Do login by starting Chrome.
	login := func(tag string) *chrome.Chrome {
		cr, err := chrome.New(ctx, chrome.KeepState())
		if err != nil {
			s.Fatalf("%s: Failed to login by starting Chrome: %v", tag, err)
		}
		return cr
	}

	// Do logout by restarting ui.
	logout := func(tag string) {
		if err := upstart.RestartJob(ctx, "ui"); err != nil {
			s.Fatalf("%s: Failed to logout by restarting ui: %v", tag, err)
		}
	}

	// Trigger a scan and return the wifi service with ssid.
	scanAndFindSvc := func(tag, ssid string) *shill.Service {
		s.Logf("Scanning for service with ssid=%s", ssid)
		svc, err := wifiMgr.ScanAndWaitForService(ctx, ssid, 30*time.Second)
		if err != nil {
			s.Fatalf("%s: Failed to scan service: %v", tag, err)
		}
		return svc
	}

	// Configure wifi svc with eapIdentity and eapPassword.
	configureSvc := func(tag string, svc *shill.Service, eapIdentity, eapPassword string) {
		props := map[string]interface{}{
			shillconst.ServicePropertyEAPMethod:       "PEAP",
			shillconst.ServicePropertyEAPInnerEAP:     "auth=MSCHAPV2",
			shillconst.ServicePropertyEAPIdentity:     eapIdentity,
			shillconst.ServicePropertyEAPPassword:     eapPassword,
			shillconst.ServicePropertyEAPKeyMgmt:      "WPA-EAP WPA-EAP-SHA256",
			shillconst.ServicePropertyEAPUseSystemCAs: false,
			shillconst.ServicePropertyAutoConnect:     true,
		}
		for k, v := range props {
			if err := svc.SetProperty(ctx, k, v); err != nil {
				s.Fatalf("%s: Failed to configure property %s with value %s: %v", tag, k, v, err)
			}
		}
		// We set AutoConnect above, but let's call Connect() here again to make
		// sure it will happen.
		if err := svc.Connect(ctx); err != nil {
			// The error messages are defined in Service::Connect() function in
			// platform2/shill/service.cc file.
			if !strings.Contains(err.Error(), "already connected") && !strings.Contains(err.Error(), "already connecting") {
				s.Fatalf("%s: Failed to call connect on the service: %v", tag, err)
			}
		}
	}

	// Verify that service goes to the online state.
	verifySvcOnline := func(tag string, svc *shill.Service) {
		s.Log("Waiting for WiFi service online")
		if err := svc.WaitForConnectedOrError(ctx); err != nil {
			s.Fatalf("%s: Failed to wait for wifi service connected: %v", tag, err)
		}
		if err := svc.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
			s.Fatalf("%s: Failed to wait for wifi service to be online: %v", tag, err)
		}
	}

	verifyIPProvision := func(tag string, svc *shill.Service) {
		s.Log("Verifying IPv4 and IPv6 provision")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			config, err := svc.GetNetworkConfig(ctx)
			if err != nil {
				return testing.PollBreak(err)
			}
			if !config.HasIPv4Address() {
				return errors.New("no IPv4 address")
			}
			if !config.HasIPv6Address() {
				return errors.New("no IPv6 address")
			}
			return nil
		}, &testing.PollOptions{
			// IP provision can take time.
			Timeout: 30 * time.Second,
		}); err != nil {
			s.Fatalf("%s: Failed to verify IP provision: %v", tag, err)
		}
	}

	// Verify that service is pingable by IP addresses.
	verifyIPConnectivity := func(tag string) {
		s.Log("Verifying pinging the server IP addresses")
		for _, ip := range []net.IP{serverIPv4, serverIPv6} {
			if err := ping.ExpectPingSuccessWithTimeout(ctx, ip.String(), "chronos", 5*time.Second); err != nil {
				s.Fatalf("%s: Failed to verify ping reachability to %s: %v", tag, ip, err)
			}
		}
	}

	// Verify that the hostname can be resolved to our expected IP.
	verifyDNS := func(tag string) {
		s.Log("Verifying DNS resolving")
		if err := dns.VerifyDNSResolve(ctx, "chronos", testHostname /*expectResolvable*/, true, serverIPv4.String()); err != nil {
			s.Fatalf("%s: Failed to verify DNS resolving: %v", tag, err)
		}
	}

	// Verify the name resolving and routing in Chrome by opening a Chrome page.
	verifyChromePage := func(tag string, cr *chrome.Chrome) {
		url := "http://" + testHostname
		s.Logf("Verifying opening %s in Chrome", url)

		conn, err := cr.NewConn(ctx, url)
		if err != nil {
			s.Fatalf("%s: Failed to open %s: %v", tag, url, err)
		}
		defer conn.Close()

		if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
			s.Fatalf("%s: Failed to wait for page to load: %v", tag, err)
		}

		content, err := conn.PageContent(ctx)
		if err != nil {
			s.Fatalf("%s: Failed to get page content: %v", tag, err)
		}

		if !strings.Contains(content, httpResponse) {
			s.Fatalf("%s: Unexpected page content: got `%s`, want `%s` in the output", tag, content, httpResponse)
		}
	}

	// Group of util functions -- end.

	s.Log("Logging in to configure WiFi service for user profile")
	func() {
		const tag = "Login #0"
		login(tag)
		svc := scanAndFindSvc(tag, ssid)
		configureSvc(tag, svc, userProfileIdentity, userProfilePassword)
		verifySvcOnline(tag, svc)
	}()

	s.Log("Logging out to configure WiFi service for device profile")
	func() {
		const tag = "Logout #0"
		logout(tag)
		svc := scanAndFindSvc(tag, ssid)
		if connected, err := svc.IsConnected(ctx); err != nil {
			s.Fatal("Failed to get service connected state: ", err)
		} else if connected {
			s.Fatal("Service is connected after logout, expected not connected since it's not configured on the device profile")
		}
		configureSvc(tag, svc, deviceProfileIdentity, deviceProfilePassword)
		verifySvcOnline(tag, svc)
	}()

	const iterCnt = 5
	for i := 1; i <= iterCnt; i++ {
		s.Logf("Test WiFi connection for login (%d/%d)", i, iterCnt)
		func() {
			tag := fmt.Sprintf("Login #%d", i)
			cr := login(tag)
			svc := scanAndFindSvc(tag, ssid)
			verifySvcOnline(tag, svc)
			verifyIPProvision(tag, svc)
			verifyIPConnectivity(tag)
			verifyDNS(tag)
			verifyChromePage(tag, cr)
		}()

		s.Logf("Test WiFi connection for logout (%d/%d)", i, iterCnt)
		func() {
			tag := fmt.Sprintf("Logout #%d", i)
			logout(tag)
			svc := scanAndFindSvc(tag, ssid)
			verifySvcOnline(tag, svc)
			verifyIPProvision(tag, svc)
			verifyIPConnectivity(tag)
			verifyDNS(tag)
			// We cannot open a Chrome page at login screen so skip the Chrome page test.
		}()
	}
}
