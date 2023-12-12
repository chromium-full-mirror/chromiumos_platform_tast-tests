// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dns

import (
	"bytes"
	"context"
	"html/template"
	"io/ioutil"
	"net"
	"net/http"
	"os"
	"regexp"
	"strconv"
	"time"

	"github.com/google/go-cmp/cmp"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/certs"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/dnsmasq"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/httpserver"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DoHMode defines possible type of DNS-over-HTTPS.
type DoHMode int

const (
	// DoHOff is a mode that resolves DNS through plaintext.
	DoHOff DoHMode = iota
	// DoHAutomatic is a mode that automatically chose between plaintext and secure DNS.
	DoHAutomatic
	// DoHAlwaysOn is a mode that resolves DNS through secure DNS.
	DoHAlwaysOn
)

// Client defines the client resolving DNS.
type Client int

const (
	// System is a DNS client type for systems.
	System Client = iota
	// User is a DNS client type for users (e.g. cups, tlsdate).
	User
	// Chrome is a DNS client type with user 'chronos'.
	Chrome
	// Crostini is a DNS client type for Crostini.
	Crostini
	// ARC is a DNS client type for ARC.
	ARC
)

// Config defines a DNS-related config.
type Config struct {
	IPv4Nameservers      []string
	IPv6Nameservers      []string
	IPv4DomainSearchList []string
	// IPv6 domain search list is not supported by shill (b/172214013).
}

// virtualnet's priority used in DNS tests.
const (
	HighPriority = 5
	BasePriority = 3
	LowPriority  = 1
)

// Env wraps the test environment created for DNS tests.
type Env struct {
	Router       *env.Env
	server       *env.Env
	manager      *shill.Manager
	Certs        *certs.Certs
	cleanupCerts func(context.Context)
}

// GoogleDoHProvider is the Google DNS-over-HTTPS provider.
const GoogleDoHProvider = "https://dns.google/dns-query"

// ExampleDoHProvider is a fake DNS-over-HTTPS provider used for testing using virtualnet package.
// The URL must match the CA certificate used by virtualnet/certs/cert.go.
const ExampleDoHProvider = "https://www.example.com/dns-query"

// DNSProxyIPv4Prefix is the prefix used for DNS proxy's namespaces.
const DNSProxyIPv4Prefix = "100.115.92"

// DigProxyIPRE is the regular expressions for DNS proxy IP inside dig output.
var DigProxyIPRE = regexp.MustCompile(`SERVER: 100.115.92.\d+#53`)

// ARCQueryRCodeRE is the regular expression to get the return code of ARC DNS query.
// RCode output is in the form of: "rcode: No error (0)"
var ARCQueryRCodeRE = regexp.MustCompile(`rcode: .* \(([0-9]+)\)`)

// ARCNameserversRE is the regular expression to get ARC's R+ current nameservers.
// The output is taken from ARC's `dumpsys wifi tools dns`:
// ARC P: "... DnsAddresses: [100.115.92.138,/2a00:79e1:abc:f605:7078:8fff:fed5:f010,] ... "
// ARC R+: "... DnsAddresses: [ /100.115.92.138,/2a00:79e1:abc:f605:7078:8fff:fed5:f010 ] ..."
var ARCNameserversRE = regexp.MustCompile(`Combined LinkProperties:.*DnsAddresses: \[ ?(?:(?:[a-zA-Z0-9\-\.]*\/)?([0-9a-f\.\:]+))?(?:,(?:[a-zA-Z0-9\-\.]*\/)?([0-9a-f\.\:]+))*,? ?\]`)

// DNS proxy run path, contains resolv.conf.
const proxyRunPath = "/run/dns-proxy"

// ResolvConfPath points to the resolv.conf file for name resolution.
const ResolvConfPath = "/etc/resolv.conf"

// Template for the expected /etc/resolv.conf's data.
const resolvConfTemplate = `
{{- range .nameservers -}}
nameserver {{.}}
{{end -}}
{{if .search_lists -}}
search{{range .search_lists}} {{.}}{{end}}
{{end -}}
options single-request timeout:1 attempts:5
`

// GetClientString get the string representation of a DNS client.
func GetClientString(c Client) string {
	switch c {
	case System:
		return "system"
	case User:
		return "user"
	case Chrome:
		return "Chrome"
	case Crostini:
		return "Crostini"
	case ARC:
		return "ARC"
	default:
		return ""
	}
}

// SetDoHMode updates ChromeOS setting to change DNS-over-HTTPS mode.
// Returns a cleanup that updates the DoH mode to DoH off.
func SetDoHMode(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, mode DoHMode, dohProvider string) (func(context.Context), error) {
	if err := setDoHMode(ctx, cr, tconn, mode, dohProvider); err != nil {
		return nil, err
	}
	return func(ctx context.Context) {
		if err := setDoHMode(ctx, cr, tconn, DoHOff, "" /* dohProvider */); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup DoH mode to DoH off: ", err)
		}
	}, nil
}

// setDoHMode updates ChromeOS setting to change DNS-over-HTTPS mode.
func setDoHMode(ctx context.Context, cr *chrome.Chrome, tconn *chrome.TestConn, mode DoHMode, dohProvider string) error {
	conn, err := apps.LaunchOSSettings(ctx, cr, "chrome://os-settings/osPrivacy")
	if err != nil {
		return errors.Wrap(err, "failed to get connection to OS Settings")
	}
	defer conn.Close()

	ac := uiauto.New(tconn)

	// Toggle secure DNS, the UI might lag, keep trying until secure DNS is toggled to the expected state.
	leftClickAc := ac.WithInterval(2 * time.Second)
	var toggleSecureDNS = func(ctx context.Context, check checked.Checked) error {
		tb := nodewith.Role(role.ToggleButton).Name("Encrypt the names of sites you visit")
		var secureDNSChecked = func(ctx context.Context) error {
			tbInfo, err := ac.Info(ctx, tb)
			if err != nil {
				return errors.Wrap(err, "failed to find secure DNS toggle button")
			}
			if tbInfo.Checked != check {
				return errors.Errorf("secure DNS toggle button checked: %s", check)
			}
			return nil
		}
		if err := leftClickAc.LeftClickUntil(tb, secureDNSChecked)(ctx); err != nil {
			return errors.Wrap(err, "failed to toggle secure DNS button")
		}
		return nil
	}

	switch mode {
	case DoHOff:
		if err := toggleSecureDNS(ctx, checked.False); err != nil {
			return err
		}
		break
	case DoHAutomatic:
		if err := toggleSecureDNS(ctx, checked.True); err != nil {
			return err
		}
		selectNode := nodewith.Name("Select DNS provider").Role(role.ComboBoxSelect)
		optionNode := nodewith.Name("OS default (when available)").Role(role.ListBoxOption)
		if err := uiauto.Combine("enable secure DNS automatic mode",
			ac.WithTimeout(10*time.Second).WaitUntilExists(selectNode),
			ac.WithInterval(3*time.Second).LeftClickUntil(selectNode, ac.Exists(optionNode)),
			ac.WithInterval(3*time.Second).LeftClickUntil(optionNode, ac.Gone(optionNode)),
		)(ctx); err != nil {
			return err
		}
		break
	case DoHAlwaysOn:
		if err := toggleSecureDNS(ctx, checked.True); err != nil {
			return err
		}

		// Get a handle to the input keyboard.
		kb, err := input.Keyboard(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get keyboard")
		}
		defer kb.Close(ctx)

		selectNode := nodewith.Name("Select DNS provider").Role(role.ComboBoxSelect)
		optionNode := nodewith.Name("Add custom DNS service provider").Role(role.ListBoxOption)
		textNode := nodewith.Name("Enter custom DNS query URL").Role(role.TextField)
		if err := uiauto.Combine("enable DoH always on with a custom provider",
			// Click add custom DNS service provider option.
			ac.WithTimeout(10*time.Second).WaitUntilExists(selectNode),
			ac.WithInterval(3*time.Second).LeftClickUntil(selectNode, ac.Exists(optionNode)),
			ac.WithInterval(3*time.Second).LeftClickUntil(optionNode, ac.Exists(textNode)),
			// Input a custom DoH provider.
			ac.EnsureFocused(textNode),
			kb.AccelAction("Ctrl+A"),
			kb.AccelAction("Backspace"),
			kb.TypeAction(dohProvider),
			kb.AccelAction("Enter"),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to enable DoH with a custom provider")
		}
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if m, err := getDoHMode(ctx); err != nil {
			return err
		} else if m != mode {
			return errors.New("failed to get the correct DoH mode")
		}
		return nil
	}, &testing.PollOptions{Timeout: 3 * time.Second}); err != nil {
		return err
	}
	return nil
}

// RandDomain returns a random domain name that can be useful for avoiding caching while testing DNS queries.
func RandDomain() string {
	return strconv.FormatInt(time.Now().UnixNano(), 16) + ".com"
}

// QueryOptions are provided to QueryDNS to configure the lookup query.
// If ARCDigPath and Nameserver is not empty, ARC's query uses dig.
// Otherwise, ARC will query through `dumpsys wifi tools dns`.
type QueryOptions struct {
	Domain     string
	Nameserver string
	ARCDigPath string
}

// NewQueryOptions returns a new options pre-populated with a random domain for testing.
func NewQueryOptions() *QueryOptions {
	return &QueryOptions{
		Domain: RandDomain(),
	}
}

func (o QueryOptions) digArgs() []string {
	args := []string{o.Domain}
	if o.Nameserver != "" {
		args = append(args, "@"+o.Nameserver)
	}
	return args
}

// QueryDNS resolves a domain through DNS with a specific client.
func QueryDNS(ctx context.Context, c Client, a *arc.ARC, cont *vm.Container, opts *QueryOptions) error {
	args := opts.digArgs()
	var u string
	switch c {
	case System:
		return testexec.CommandContext(ctx, "dig", args...).Run()
	case User:
		u = "cups"
	case Chrome:
		u = "chronos"
	case Crostini:
		return cont.Command(ctx, append([]string{"dig"}, args...)...).Run()
	case ARC:
		// ARC does not support querying a specific nameserver.
		// Use dig binary to do so.
		if opts.ARCDigPath != "" && opts.Nameserver != "" {
			return a.Command(ctx, opts.ARCDigPath, args...).Run()
		}
		out, err := a.Command(ctx, "dumpsys", "wifi", "tools", "dns", "host_default", opts.Domain).Output()
		if err != nil {
			return errors.Wrap(err, "failed to do ARC DNS query")
		}
		if matches := ARCQueryRCodeRE.FindStringSubmatch(string(out)); matches != nil {
			// Only NOERROR and NXDOMAIN are allowed.
			if matches[1] == "0" || matches[1] == "3" {
				return nil
			}
			return errors.New("failed to resolve domain, got errcode " + matches[1])
		}
		return errors.New("failed to resolve domain: " + string(out))
	default:
		return errors.New("unknown client")
	}
	return testexec.CommandContext(ctx, "sudo", append([]string{"-u", u, "dig"}, args...)...).Run()
}

// ProxyTestCase contains test case for DNS proxy tests.
type ProxyTestCase struct {
	Client     Client
	ExpectErr  bool
	AllowRetry bool
}

// TestQueryDNSProxy runs a set of test cases for DNS proxy.
func TestQueryDNSProxy(ctx context.Context, tcs []ProxyTestCase, a *arc.ARC, cont *vm.Container, opts *QueryOptions) []error {
	var errs []error
	for _, tc := range tcs {
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			var err error
			qErr := QueryDNS(ctx, tc.Client, a, cont, opts)
			if qErr != nil && !tc.ExpectErr {
				err = errors.Wrapf(qErr, "DNS query failed for %s", GetClientString(tc.Client))
			}
			if qErr == nil && tc.ExpectErr {
				err = errors.Errorf("successful DNS query for %s, but expected failure", GetClientString(tc.Client))
			}
			if !tc.AllowRetry {
				return testing.PollBreak(err)
			}
			return err
		}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
			errs = append(errs, err)
		}
	}
	return errs
}

// InstallDigInARC installs a statically-linked binary of dig inside ARC.
// Returns the installed path of dig.
func InstallDigInARC(ctx context.Context, a *arc.ARC, execPath string) (string, error) {
	p, err := a.PushFileToTmpDir(ctx, execPath)
	if err != nil {
		return "", errors.Wrapf(err, "failed to push %q to tmpdir", execPath)
	}
	if err := a.Command(ctx, "chmod", "0755", p).Run(testexec.DumpLogOnError); err != nil {
		return "", errors.Wrapf(err, "failed to change the permission of %q", p)
	}
	return p, nil
}

// InstallDigInContainer verifies dig is installed in the container.
// Does not actually install dig.
func InstallDigInContainer(ctx context.Context, cont *vm.Container) error {
	return cont.Command(ctx, "dig", "-v").Run()
}

// getDoHProviders returns the current DNS-over-HTTPS providers.
func getDoHProviders(ctx context.Context) (map[string]interface{}, error) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create shill manager object")
	}

	props, err := m.GetProperties(ctx)
	if err != nil {
		return nil, err
	}
	out, err := props.Get(shillconst.ManagerPropertyDOHProviders)
	if err != nil {
		return nil, err
	}
	providers, ok := out.(map[string]interface{})
	if !ok {
		return nil, errors.Errorf("property %s is not a map of string to interface: %q", shillconst.ManagerPropertyDOHProviders, out)
	}
	return providers, nil
}

// getDoHMode returns the current DNS-over-HTTPS mode.
func getDoHMode(ctx context.Context) (DoHMode, error) {
	providers, err := getDoHProviders(ctx)
	if err != nil || len(providers) == 0 {
		return DoHOff, err
	}
	for _, ns := range providers {
		if ns == "" {
			continue
		}
		return DoHAutomatic, nil
	}
	return DoHAlwaysOn, nil
}

// DigMatch runs dig to check name resolution works and verifies the expected server was used.
func DigMatch(ctx context.Context, re *regexp.Regexp, match bool) error {
	out, err := testexec.CommandContext(ctx, "dig", "google.com").Output()
	if err != nil {
		return errors.Wrap(err, "dig failed")
	}
	if re.MatchString(string(out)) != match {
		return errors.New("dig used unexpected nameserver")
	}
	return nil
}

// queryDNS queries DNS to |addr| through UDP port 53 and returns the response.
func queryDNS(ctx context.Context, msg []byte, addr string) ([]byte, error) {
	var d net.Dialer
	conn, err := d.DialContext(ctx, "udp", addr+":53")
	if err != nil {
		return nil, err
	}
	defer conn.Close()

	if _, err := conn.Write(msg); err != nil {
		return nil, err
	}

	resp := make([]byte, 512)
	n, err := conn.Read(resp)
	if err != nil {
		return nil, err
	}
	return resp[:n], nil
}

// DoHResponder returns a function that responds to HTTPS queries by proxying the queries to DNS server on |addr|.
func DoHResponder(ctx context.Context, addr string) func(http.ResponseWriter, *http.Request) {
	return func(rw http.ResponseWriter, req *http.Request) {
		msg, err := ioutil.ReadAll(req.Body)
		if err != nil {
			testing.ContextLog(ctx, "Failed to read HTTPS request: ", err)
			return
		}

		resp, err := queryDNS(ctx, msg, addr)
		if err != nil {
			testing.ContextLog(ctx, "Failed to query DNS: ", err)
			return
		}

		rw.Header().Set("content-type", "application/dns-message")
		if _, err := rw.Write(resp); err != nil {
			testing.ContextLog(ctx, "Failed to write HTTPS response: ", err)
		}
	}
}

// Cleanup cleans anything that is set up through NewEnv. This needs to be called whenever env is not needed anymore.
func (e *Env) Cleanup(ctx context.Context) {
	if e.server != nil {
		if err := e.server.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup server env: ", err)
		}
	}
	if e.Router != nil {
		if err := e.Router.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup router env: ", err)
		}
	}
	if e.cleanupCerts != nil {
		e.cleanupCerts(ctx)
		if err := RestartDNSProxy(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to restart DNS proxy: ", err)
		}
	}
	if e.manager != nil {
		if err := e.manager.SetProperty(ctx, shillconst.ProfilePropertyCheckPortalList, "ethernet,wifi,cellular"); err != nil {
			testing.ContextLog(ctx, "Failed to revert check portal list property: ", err)
		}
	}
}

// NewEnv creates a DNS environment including router that acts as the default network and a server that responds to DNS and DoH queries.
// On success, the caller is responsible to cleanup the environment through the |Cleanup| function.
func NewEnv(ctx context.Context, pool *subnet.Pool) (env *Env, err error) {
	e := &Env{}
	defer func() {
		if e != nil {
			e.Cleanup(ctx)
		}
	}()

	// Shill-related setup.
	e.manager, err = shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create manager proxy")
	}
	testing.ContextLog(ctx, "Disabling portal detection on ethernet")
	if err := e.manager.SetProperty(ctx, shillconst.ProfilePropertyCheckPortalList, "wifi,cellular"); err != nil {
		return nil, errors.Wrap(err, "failed to disable portal detection on ethernet")
	}
	testing.ContextLog(ctx, "Resetting ethernet ephemeral priority")
	if err := virtualnet.ResetEthernetEphemeralPriority(ctx, e.manager); err != nil {
		return nil, errors.Wrap(err, "failed to reset ethernet ephemeral priority")
	}

	// Install test certificates for HTTPS server. In doing so, virtualnet/certs will mount a test certificate directory.
	// Because DNS proxy lives in its own namespace, it needs to be restarted to be able to see the test certificates.
	httpsCerts := certs.New(certs.SSLCrtPath, certificate.TestCert3())
	e.cleanupCerts, err = httpsCerts.InstallTestCerts(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to setup certificates")
	}
	if err := RestartDNSProxy(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to restart DNS proxy")
	}

	// Allocate subnet for DNS server.
	serverIPv4Subnet, err := pool.AllocNextIPv4Subnet()
	if err != nil {
		return nil, errors.Wrap(err, "failed to allocate v4 subnet")
	}
	serverIPv6Subnet, err := pool.AllocNextIPv6Subnet()
	if err != nil {
		return nil, errors.Wrap(err, "failed to allocate v6 subnet")
	}
	// This assumes that the server will use the IPv4 address xx.xx.xx.2 from env's ConnectToRouter internal implementation.
	serverAddr := serverIPv4Subnet.GetAddrEndWith(2)

	var svc *shill.Service
	svc, e.Router, err = virtualnet.CreateRouterEnv(ctx, e.manager, pool, virtualnet.EnvOptions{
		Priority:       5,
		NameSuffix:     "",
		IPv4DNSServers: []string{serverAddr.String()},
		EnableDHCP:     true,
		RAServer:       false,
	})
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up router env")
	}

	if err := svc.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to wait for base service online")
	}

	e.server, err = NewServer(ctx, "server", serverIPv4Subnet, serverIPv6Subnet, e.Router, httpsCerts)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up server env")
	}

	env = e
	e = nil
	return env, nil
}

// NewServer creates a server that responds to DNS and DoH queries.
func NewServer(ctx context.Context, envName string, ipv4Subnet *subnet.IPv4Subnet, ipv6Subnet *subnet.IPv6Subnet, routerEnv *env.Env, httpsCerts *certs.Certs) (*env.Env, error) {
	success := false

	server, err := virtualnet.CreateEnv(ctx, envName)
	if err != nil {
		return nil, errors.Wrap(err, "failed to set up server env")
	}
	defer func() {
		if success {
			return
		}
		if err := server.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup server env: ", err)
		}
	}()

	if err := server.ConnectToRouter(ctx, routerEnv, ipv4Subnet, ipv6Subnet); err != nil {
		return nil, errors.Wrap(err, "failed to connect server to router")
	}

	// Get server IPv4 address.
	addr, err := server.GetVethInAddrs(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get env addresses")
	}

	// Start a DNS server.
	if err := server.StartServer(ctx, "dnsmasq", dnsmasq.New(
		dnsmasq.WithResolveHost("", addr.IPv4Addr),
		dnsmasq.WithAllInterfaces(),
	)); err != nil {
		return nil, errors.Wrap(err, "failed to start dnsmasq")
	}

	// Start a DoH server.
	httpsserver := httpserver.New(httpserver.TCP4, "443", DoHResponder(ctx, addr.IPv4Addr.String()), httpsCerts)
	if err := server.StartServer(ctx, "httpsserver", httpsserver); err != nil {
		return nil, errors.Wrap(err, "failed to start DoH server")
	}

	success = true
	return server, nil
}

// EnvOptionsFromConfig creates virtualnet.EnvOptions from DNS config, name suffix, and priority.
func EnvOptionsFromConfig(config Config, nameSuffix string, priority int) virtualnet.EnvOptions {
	opts := virtualnet.EnvOptions{
		Priority:   priority,
		NameSuffix: nameSuffix,
	}
	if len(config.IPv4Nameservers) > 0 || len(config.IPv4DomainSearchList) > 0 {
		opts.EnableDHCP = true
		opts.IPv4DNSServers = config.IPv4Nameservers
		opts.IPv4DomainSearchList = config.IPv4DomainSearchList
	}
	if len(config.IPv6Nameservers) > 0 {
		opts.RAServer = true
		opts.IPv6DNSServers = config.IPv6Nameservers
	}
	return opts
}

// WaitForNameServers waits until svc contains nameservers in its IPConfig.
func WaitForNameServers(ctx context.Context, svc *shill.Service, nameservers []string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		cfgs, err := svc.GetIPConfigs(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get IPConfig")
		}
		gotNss := make(map[string]struct{})
		for _, cfg := range cfgs {
			p, err := cfg.GetIPProperties(ctx)
			if err != nil {
				return errors.Wrap(err, "failed to get IPConfig properties")
			}
			for _, nameserver := range p.NameServers {
				gotNss[nameserver] = struct{}{}
			}
		}
		wantNss := make(map[string]struct{})
		for _, nameserver := range nameservers {
			wantNss[nameserver] = struct{}{}
		}
		if !cmp.Equal(gotNss, wantNss) {
			return errors.Errorf("Nameservers mismatch: got %v, want %v", gotNss, wantNss)
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to wait for nameservers")
	}
	return nil
}

// NewShillService creates a shill service with a certain DNS configuration.
func NewShillService(ctx context.Context, opts virtualnet.EnvOptions, pool *subnet.Pool) (*shill.Service, *virtualnet.Env, error) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create manager proxy")
	}
	svc, r, err := virtualnet.CreateRouterEnv(ctx, m, pool, opts)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to set up shill service")
	}
	success := false
	defer func() {
		if success {
			return
		}
		if err := r.Cleanup(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to cleanup router: ", err)
		}
	}()
	if err := svc.WaitForProperty(ctx, shillconst.ServicePropertyState, shillconst.ServiceStateOnline, 10*time.Second); err != nil {
		return nil, nil, errors.Wrap(err, "failed to wait for shill service to be online")
	}
	if err := WaitForNameServers(ctx, svc, append(opts.IPv4DNSServers, opts.IPv6DNSServers...)); err != nil {
		return nil, nil, err
	}

	success = true
	return svc, r, nil
}

// VerifyResolvConfContents returns an error if /etc/resolv.conf contents does not match the expected config.
// When DNS proxy is enabled, /etc/resolv.conf is expected to be replaced by the proxy's addresses.
func VerifyResolvConfContents(ctx context.Context, config Config, proxyEnabled bool) error {
	var nameservers []template.HTML
	if proxyEnabled {
		nameservers = expectedNameserversWithDNSProxy(ctx, config)
	} else {
		nameservers = expectedNameserversWithoutDNSProxy(ctx, config)
	}

	// Build the expected /etc/resolv.conf data.
	vals := map[string]interface{}{
		"nameservers":  nameservers,
		"search_lists": config.IPv4DomainSearchList,
	}
	b := &bytes.Buffer{}
	template.Must(template.New("").Parse(resolvConfTemplate)).Execute(b, vals)
	re := regexp.MustCompile("^" + b.String() + "$")

	// Read the actual /etc/resolv.conf data.
	d, err := os.ReadFile(ResolvConfPath)
	if err != nil {
		return errors.Wrap(err, "failed to read resolv.conf")
	}
	if !re.MatchString(string(d)) {
		return errors.Errorf("resolv.conf value does not match, got: %s, want: %s", d, b)
	}
	return nil
}

// expectedNameserversWithoutDNSProxy gets the expected /etc/resolv.conf nameservers without DNS proxy overwriting it.
func expectedNameserversWithoutDNSProxy(ctx context.Context, config Config) []template.HTML {
	var nss []template.HTML
	for _, ns := range config.IPv6Nameservers {
		nss = append(nss, template.HTML(ns))
	}
	for _, ns := range config.IPv4Nameservers {
		nss = append(nss, template.HTML(ns))
	}
	return nss
}

// expectedNameserversWithDNSProxy gets the expected /etc/resolv.conf nameservers
// regex when DNS proxy is overwriting it.
func expectedNameserversWithDNSProxy(ctx context.Context, config Config) []template.HTML {
	// template.HTML to avoid the regex to be escaped.
	var nssRE []template.HTML
	if len(config.IPv4Nameservers) > 0 {
		nssRE = append(nssRE, DNSProxyIPv4Prefix+".\\d+")
	}
	if len(config.IPv6Nameservers) > 0 {
		nssRE = append(nssRE, "([a-f0-9:]+:+)+[a-f0-9]+")
	}
	return nssRE
}
