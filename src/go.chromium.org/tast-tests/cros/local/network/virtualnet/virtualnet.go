// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package virtualnet

import (
	"context"
	"encoding/hex"
	"fmt"
	"net"
	"net/http"
	"os"
	"strings"
	"time"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/httpserver"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/radvd"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Env simulates a node on the network, which can run different kinds of servers
// (e.g., DHCP, HTTP) on it. An Env can be attached to the DUT directly (i.e.,
// as a router), or connected to each other to simulate a complex network
// topology. In implementation, an Env consists of a network namespace and a
// pair of veth interfaces, and all the servers (commands) will be ran with
// chroot and minijail inside it, so it should not interfere with the
// environment on the host. See go/cros-routing-tests for more details.
type Env = env.Env

// CreateEnv sets up an Env. name will be used in the name of netns and veth
// interfaces, and it should be unique among Envs. The Env created by this
// function will not be visible to shill, and should usually be connected to
// other Envs. To simulate a network on DUT, use CreateRouterEnv() or other
// variants instead.
func CreateEnv(ctx context.Context, name string) (*Env, error) {
	e := env.NewHidden(name)
	if err := e.SetUp(ctx); err != nil {
		return nil, err
	}
	return e, nil
}

// EnvOptions contains the options that can be used to set up a virtualnet Env.
type EnvOptions struct {
	// Priority is the service priority in shill for the exposed interface. Please
	// refer to the EphemeralPriority property in service-api.txt for details.
	Priority int
	// NameSuffix is used to differentiate different virtualnet Env. Its length
	// cannot be longer than 3 due to IFNAMSIZ, and can be left empty if only one
	// Env is used.
	NameSuffix string
	// EnableDHCP enables the DHCP server in the Env. IPv4 address can be obtained
	// on the interface by DHCP.
	EnableDHCP bool
	// IPv4DNSServers specifies the IPv4 DNS servers to be advertised by the router (dnsmasq).
	IPv4DNSServers []string
	// IPv4DomainSearchList specifies the IPv4 DNS search list to be advertised
	// by the router (dnsmasq). EnableDHCP must be set for this option.
	IPv4DomainSearchList []string
	// IPv4MTU specifies the customized MTU to be advertised by the router (dnsmasq).
	// EnableDHCP must be set for this option.
	IPv4MTU int
	// RAServer enables the RA server in the Env. IPv6 addresses can be obtained
	// on the interface by SLAAC.
	RAServer bool
	// IPv6DNSServers specifies the DNS servers to be advertised by RDNSS through
	// RA. If not specified, an IP of router itself will be used.
	IPv6DNSServers []string
	// HTTPServerResponseHandler is the handler function for the HTTP server
	// to customize how the server should respond to requests. If the handler is
	// set, then this enables the HTTP server in the Env.
	HTTPServerResponseHandler func(rw http.ResponseWriter, req *http.Request)
	// HTTPSServerResponseHandler is the handler function for the HTTPS server
	// to customize how the server should respond to requests. If the handler is
	// set, then this enables the HTTPS server in the Env.
	HTTPSServerResponseHandler func(rw http.ResponseWriter, req *http.Request)
	// HTTPS certs is the cert directory and a cert store to be used by HTTPS server.
	HTTPSCerts *certs.Certs
	// EnableDNS enables the DNS functionality. It only support resolving ResolvedHost
	// to a single IP address ResolveHostToIP provided during configuration.
	EnableDNS bool
	// ResolvedHost is the hostname to force a specific IPv4 or IPv6 address.
	// When ResolvedHost is queried from dnsmasq, dnsmasq will respond with ResolveHostToIP.
	// If resolvedHost is not set, it matches any domain in dnsmasq configuration.
	ResolvedHost string
	// ResolveHostToIP is the IP address returned when doing a DNS query for ResolvedHost.
	// If resolveHostToIP is not set, resolvedHost is resolved to an IPv4 or IPv6 address
	// to the gateway of the virtualnet Env created with these EnvOptions.
	ResolveHostToIP net.IP
	// CapportURL is the URL of the CAPPORT API. If CapportURL is set, dnsmasq will send
	// the DHCP option 114 with CapportURL, defined at RFC 8910.
	CapportURL string
}

// ResetEthernetProperties resets all properties which can affect test runs to
// the default values, on all Ethernet services. Note that resetting the
// properties on a single service may not be reliable, due to our implementation
// of the ethernet_any service, and thus we do a global reset here.
func ResetEthernetProperties(ctx context.Context, m *shill.Manager) error {
	svcs, _, err := m.ServicesByTechnology(ctx, shill.TechnologyEthernet)
	if err != nil {
		return errors.Wrap(err, "failed to get Ethernet services")
	}
	for _, svc := range svcs {
		// Reset EphemeralPriority to 0, otherwise the physical network may have a
		// higher priority than the one created in virtualnet.
		if err := svc.SetProperty(ctx, shillconst.ServicePropertyEphemeralPriority, 0); err != nil {
			return errors.Wrapf(err, "failed to reset EphemeralPriority on %s", svc.ObjectPath())
		}
		// Reset CheckPortal to "auto". CheckPortal Service property has a higher
		// priority over the CheckPortalList Manager priority. If its value is
		// "true", our disable portal detection logic in the test code won't take
		// effect, i.e., portal detection will still be ran on this service.
		if err := svc.SetProperty(ctx, shillconst.ServicePropertyCheckPortal, "auto"); err != nil {
			return errors.Wrapf(err, "failed to reset CheckPortal on %s", svc.ObjectPath())
		}
	}
	return nil
}

// CreateRouterEnv creates a virtualnet Env with the given options. On success,
// returns the corresponding shill Service and Env object. It's caller's
// responsibility to call Cleanup() on the returned Env object.
func CreateRouterEnv(ctx context.Context, m *shill.Manager, pool *subnet.Pool, opts EnvOptions) (*shill.Service, *Env, error) {
	router := env.New("router" + opts.NameSuffix)
	if err := router.SetUp(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to set up the router env")
	}

	success := false
	defer func() {
		if success {
			return
		}
		if err := router.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup router env: ", err)
		}
	}()

	if err := startServersInRouter(ctx, router, pool, opts); err != nil {
		return nil, nil, err
	}

	svc, err := findEthernetServiceByIfName(ctx, m, router.VethOutName)
	if err != nil {
		return nil, nil, errors.Wrapf(err, "failed to find shill service for %s", router.VethOutName)
	}

	if err := svc.SetProperty(ctx, shillconst.ServicePropertyEphemeralPriority, opts.Priority); err != nil {
		return nil, nil, errors.Wrap(err, "failed to configure priority on interface")
	}

	// Do a reconnect to speed up the DHCP process, since when the veth device is
	// created, DHCP server may not be ready, and thus the first DHCP DISCOVER
	// packet will be lost, and result a long timeout to send the second one. Note
	// that this speed-up is in a best effort way since it cannot be guaranteed
	// that the DHCP server is ready this time.
	if opts.EnableDHCP {
		if err := svc.Reconnect(ctx); err != nil {
			return nil, nil, errors.Wrap(err, "failed to reconnect the veth service")
		}
	}

	testing.ContextLogf(ctx, "virtualnet env %s started", router.NetNSName)
	success = true
	return svc, router, nil
}

// CreateRouterServerEnv creates two virtualnet Envs with the given options.
// This first one (router) simulates the first hop for Internet access which is
// connected to DUT directly, and the second one (server) simulates a server on
// the Internet. This setup is useful when we need to test something that cannot
// be done in local subnet, e.g., to test the default routes. On success, it's
// caller's responsibility to call Cleanup() on the returned Env objects.
func CreateRouterServerEnv(ctx context.Context, m *shill.Manager, pool *subnet.Pool, opts EnvOptions) (svc *shill.Service, routerEnv, serverEnv *Env, err error) {
	success := false

	svc, router, err := CreateRouterEnv(ctx, m, pool, opts)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to create router env")
	}
	defer func() {
		if success {
			return
		}
		if err := router.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup router env: ", err)
		}
	}()

	server, err := CreateEnv(ctx, "server"+opts.NameSuffix)
	if err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to set up server env")
	}
	defer func() {
		if success {
			return
		}
		if err := server.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup server env: ", err)
		}
	}()

	if err := server.ConnectToRouterWithPool(ctx, router, pool); err != nil {
		return nil, nil, nil, errors.Wrap(err, "failed to connect server to router")
	}

	success = true
	return svc, router, server, nil
}

func startServersInRouter(ctx context.Context, router *Env, pool *subnet.Pool, opts EnvOptions) error {
	var dnsmasqOpts []dnsmasq.Option
	if opts.EnableDHCP {
		v4Subnet, err := pool.AllocNextIPv4Subnet()
		if err != nil {
			return errors.Wrap(err, "failed to allocate v4 subnet for DHCP")
		}
		dnsmasqOpts = append(dnsmasqOpts, dnsmasq.WithDHCPServer(v4Subnet), dnsmasq.WithDHCPNameServers(opts.IPv4DNSServers), dnsmasq.WithDHCPDomainSearchList(opts.IPv4DomainSearchList))
	}
	if opts.EnableDNS {
		dnsmasqOpts = append(dnsmasqOpts, dnsmasq.WithResolveHost(opts.ResolvedHost, opts.ResolveHostToIP))
	}
	if opts.IPv4MTU > 0 {
		dnsmasqOpts = append(dnsmasqOpts, dnsmasq.WithMTU(opts.IPv4MTU))
	}
	if opts.CapportURL != "" {
		dnsmasqOpts = append(dnsmasqOpts, dnsmasq.WithCapportAPI(opts.CapportURL))
	}
	if len(dnsmasqOpts) > 0 {
		dnsmasq := dnsmasq.New(dnsmasqOpts...)
		if err := router.StartServer(ctx, "dnsmasq", dnsmasq); err != nil {
			return errors.Wrap(err, "failed to start dnsmasq")
		}
	}

	if opts.RAServer {
		v6Prefix, err := pool.AllocNextIPv6Subnet()
		if err != nil {
			return errors.Wrap(err, "failed to allocate v6 prefix for RA server")
		}

		// Set up a fix address in the subnet for the router
		ipv6Addr := v6Prefix.IP.To16()
		var selfIPv6Addr net.IP
		selfIPv6Addr = append([]byte{}, ipv6Addr...)
		selfIPv6Addr[14] = 16 // (prefix)::1000
		if err := router.ConfigureInterface(ctx, router.VethInName, selfIPv6Addr, v6Prefix); err != nil {
			return errors.Wrapf(err, "failed to configure static IPv6 address on %s", router.VethInName)
		}

		var rdnssServers []string
		rdnssServers = append(rdnssServers, opts.IPv6DNSServers...)
		if len(rdnssServers) == 0 {
			// Note that in the current implementation, shill requires an IPv6
			// connection has both address and DNS servers. Therefore if not
			// explicitly provided we'll use our own address as DNS server.
			rdnssServers = append(rdnssServers, selfIPv6Addr.String())
		}
		radvd := radvd.New(v6Prefix, rdnssServers)
		if err := router.StartServer(ctx, "radvd", radvd); err != nil {
			return errors.Wrap(err, "failed to start radvd")
		}
	}

	if opts.HTTPServerResponseHandler != nil {
		serverIPv4 := httpserver.New(httpserver.TCP4, "80", opts.HTTPServerResponseHandler, nil)
		if err := router.StartServer(ctx, "httpserver_ipv4", serverIPv4); err != nil {
			return errors.Wrap(err, "failed to start IPv4 http server")
		}
		serverIPv6 := httpserver.New(httpserver.TCP6, "80", opts.HTTPServerResponseHandler, nil)
		if err := router.StartServer(ctx, "httpserver_ipv6", serverIPv6); err != nil {
			return errors.Wrap(err, "failed to start IPv6 http server")
		}
	}

	if opts.HTTPSServerResponseHandler != nil {
		if opts.HTTPSCerts == nil {
			return errors.New("failed to create https server: empty certificate option")
		}
		serverIPv4 := httpserver.New(httpserver.TCP4, "443", opts.HTTPSServerResponseHandler, opts.HTTPSCerts)
		if err := router.StartServer(ctx, "httpsserver_ipv4", serverIPv4); err != nil {
			return errors.Wrap(err, "failed to start IPv4 https server")
		}
		serverIPv6 := httpserver.New(httpserver.TCP6, "443", opts.HTTPSServerResponseHandler, opts.HTTPSCerts)
		if err := router.StartServer(ctx, "httpsserver_ipv6", serverIPv6); err != nil {
			return errors.Wrap(err, "failed to start IPv6 https server")
		}
	}

	return nil
}

func findEthernetServiceByIfName(ctx context.Context, m *shill.Manager, ifName string) (*shill.Service, error) {
	testing.ContextLogf(ctx, "Waiting for device %s showing up", ifName)

	// Shill needs to flush the profile to the disk to finish this step, and we
	// observed that it can take a long time on some boards under some certain
	// situation (e.g., ~7s on grunt), so use a large timeout value here. See
	// b/303138488 for details.
	device, err := m.WaitForDeviceByName(ctx, ifName, 15*time.Second)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to find the device with interface name %s", ifName)
	}
	testing.ContextLog(ctx, "Waiting for service being selected on device: ", ifName)
	service, err := device.WaitForSelectedService(ctx, 5*time.Second)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get the selected service")
	}
	return service, nil
}

type wifiEnv struct {
	// Service is the shill service corresponding to this AP.
	Service *shill.Service
	// Router is the Env which simulates the WiFi router. Servers can be ran on it.
	Router *Env

	hostapd *testexec.Cmd

	// br is the Bridge interface name for connecting apIf and out interface of
	// wifi.Router. This interface will be started by hostapd and removed manually
	// by the test code.
	br string
}

// Cleanup shuts down all the servers.
func (e *wifiEnv) Cleanup(ctx context.Context) error {
	if e.Router != nil {
		if err := e.Router.Cleanup(ctx); err != nil {
			return err
		}
	}
	if e.hostapd != nil && e.hostapd.Process != nil {
		// Stop hostapd using SIGTERM to let the process stop the access point
		// correctly and remove the bridge interface.
		if err := e.hostapd.Signal(unix.SIGTERM); err != nil {
			return errors.Wrap(err, "failed to kill hostapd process")
		}
		e.hostapd.Wait()
	}
	return nil
}

// CreateWifiRouterEnv create a virtualnet Env with the given options. Different
// from CreateRouterEnv, the created Env will be shown as a WiFi service in
// shill. apIf should be an interface created by the mac80211_hwsim kernel
// module, and this interface will be used to simulate the access point.
// shillSimulatedWiFi fixture can be used to create such interfaces. Note that
// the returned shill service will not be connected automatically.
func CreateWifiRouterEnv(ctx context.Context, apIf string, m *shill.Manager, pool *subnet.Pool, opts EnvOptions) (*wifiEnv, error) {
	wifi := &wifiEnv{}
	wifi.Router = env.NewHidden("wifi" + opts.NameSuffix)
	if err := wifi.Router.SetUp(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to set up the router env")
	}

	success := false
	cleanupCtx := ctx
	ctx, _ = ctxutil.Shorten(ctx, 10*time.Second)
	defer func(ctx context.Context) {
		if success {
			return
		}
		if err := wifi.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to clean up WiFi Env")
		}
	}(cleanupCtx)

	if err := startServersInRouter(ctx, wifi.Router, pool, opts); err != nil {
		return nil, err
	}

	ssid := "test-ap" + opts.NameSuffix
	hexSSID := hex.EncodeToString([]byte(ssid))
	svcProps := map[string]interface{}{
		shillconst.ServicePropertyType:        shillconst.TypeWifi,
		shillconst.ServicePropertyWiFiHexSSID: strings.ToUpper(hexSSID),
	}

	// Remove the service if shill already knows this service. It can be leftover
	// from the previous tests.
	svc, err := m.FindMatchingService(ctx, svcProps)
	if err != nil && err.Error() != shillconst.ErrorMatchingServiceNotFound {
		return nil, errors.Wrap(err, "failed to check service existence before starting hostapd")
	}
	if svc != nil {
		if err := svc.Remove(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to remove the WiFi service")
		}
	}

	// Remove the DHCP lease file if it exists one to make sure we have a clean setup.
	leaseFile := fmt.Sprintf("/var/lib/dhcpcd7/wifi_any_%s_managed_none.lease", hexSSID)
	if _, err := os.Stat(leaseFile); err == nil {
		testing.ContextLogf(ctx, "DHCP lease file exists for %s, removing it", ssid)
		if err := os.Remove(leaseFile); err != nil {
			return nil, errors.Wrap(err, "failed to remove lease file")
		}
	} else if !errors.Is(err, os.ErrNotExist) {
		return nil, errors.Wrap(err, "failed to check lease file")
	}

	// Name it with prefix "veth" to avoid shill managing it.
	wifi.br = "veth-br0" + opts.NameSuffix

	// Start hostapd. We don't care about how the WiFi layer is working here so
	// just use the simplest setup.
	outDir, ok := testing.ContextOutDir(ctx)
	if !ok {
		return nil, errors.New("failed to get ContextOutDir")
	}
	hostapdConf := strings.Join([]string{
		"interface=" + apIf,
		"ssid=" + ssid,
		"bridge=" + wifi.br,
		// Set a channel to speed up the AP setup.
		"channel=1",
	}, "\n")
	hostapdConfFile, err := os.CreateTemp(outDir, "hostapd*.conf")
	if err != nil {
		return nil, errors.Wrap(err, "failed to create hostapd config file")
	}
	defer os.Remove(hostapdConfFile.Name())
	if _, err := hostapdConfFile.WriteString(hostapdConf); err != nil {
		return nil, errors.Wrap(err, "failed to write hostapd config file")
	}
	wifi.hostapd = testexec.CommandContext(ctx, "hostapd", hostapdConfFile.Name())
	if err := wifi.hostapd.Start(); err != nil {
		return nil, errors.Wrap(err, "failed to start hostapd")
	}

	testing.ContextLogf(ctx, "Waiting for %s interface up", wifi.br)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := net.InterfaceByName(wifi.br); err != nil {
			return errors.Errorf("failed to get %s interface", wifi.br)
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to wait for bridge interface to appear")
	}

	// Bridge interfaces.
	for _, cmd := range [][]string{
		{"ip", "link", "set", wifi.br, "up"},
		{"ip", "link", "set", wifi.Router.VethOutName, "up"},
		{"brctl", "addif", wifi.br, wifi.Router.VethOutName},
	} {
		if err := testexec.CommandContext(ctx, cmd[0], cmd[1:]...).Run(testexec.DumpLogOnError); err != nil {
			return nil, errors.Wrap(err, "failed to bridge the interfaces")
		}
	}

	// Disable and re-enable the WiFi device to force stopping the ongoing scan,
	// to make sure that the scan is triggered after AP is created, otherwise the
	// AP cannot be found by the scan (b/262366537).
	d, err := m.DeviceByType(ctx, shillconst.TypeWifi)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get WiFi device")
	}
	if err := d.Disable(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to disable WiFi device")
	}
	if err := d.Enable(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to enable WiFi device")
	}

	wifim, err := shill.NewWifiManager(ctx, m)
	if err != nil {
		return nil, errors.Wrap(err, "failed to obtain Wi-Fi manager")
	}

	// Delay to wait for a network to be discovered.
	const scanAndWaitTimeout = 30 * time.Second
	// Trigger a scan.
	wifi.Service, err = wifim.ScanAndWaitForService(ctx, ssid, scanAndWaitTimeout)
	if err != nil {
		return nil, errors.Wrap(err, "failed to request an active scan")
	}

	success = true
	testing.ContextLogf(ctx, "Virtual WiFi router env with SSID %s has been set up", ssid)
	return wifi, nil
}
