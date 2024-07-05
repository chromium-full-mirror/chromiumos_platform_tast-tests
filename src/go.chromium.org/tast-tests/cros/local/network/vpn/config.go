// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vpn

import (
	"fmt"
	"net"

	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
)

// Config contains the parameters (for both client and server) to configure a
// VPN connection.
type Config struct {
	Type          Type
	MTU           int
	Metered       bool
	SearchDomains []string
	proxyConfig   string

	IPsecAuthType IPsecAuthType

	// Parameters for an L2TP/IPsec VPN connection.
	IPsecUseXauth bool

	// Parameters for an OpenVPN connection.
	openVPNUseUserPassword  bool
	openVPNCertVerify       bool
	openVPNCertVerifyCNOnly bool
	openVPNTLSAuth          bool
	openvpnTopology         OpenVPNTopology

	// Parameters for a WireGuard connection.
	wgUsePSK           bool
	wgClientKeyPair    wgKeyPair
	wgServerKeyPair    wgKeyPair
	wgClientIPv4       string
	wgClientIPv6       string
	wgServerListenPort int

	// IPType specifies the overlay IP type of the VPN service.
	// Currently VPNs except for WireGuard only supports IPv4.
	IPType IPType
	// CertVals contains necessary values to setup a cert-based VPN service. This
	// is only used by cert-based VPNs (e.g., L2TP/IPsec-cert, OpenVPN, etc.).
	CertVals *CertVals

	ipv4Subnet *subnet.IPv4Subnet
	ipv6Subnet *subnet.IPv6Subnet

	includedRoutesV4 []net.IPNet

	allowReachUnderlayIPFromVPN bool

	// Parameters for configure DNS, only one of them can take effect. These
	// parameters are set by the WithDNS*() options. Also see the comments there.
	dnsUseDefaultIPv4 bool
	dnsUseDefaultIPv6 bool
	dnsAddress        string

	autoConnect bool
}

// Type represents the VPN type.
type Type int

// VPN types.
const (
	TypeIKEv2 Type = iota
	TypeL2TPIPsec
	TypeOpenVPN
	TypeWireGuard
	TypeToyVPNServer // only for server setup, not for connection or client
)

func (t Type) String() string {
	return []string{"IKEv2", "L2TP/IPsec", "OpenVPN", "WireGuard", "ToyVPNServer"}[t]
}

// IPsecAuthType represent the authentication type for an IPsec-based VPN
// connection.
type IPsecAuthType int

// IPsec authentication types.
const (
	AuthTypePSK IPsecAuthType = iota
	AuthTypeCert
	AuthTypeEAP
)

func (t IPsecAuthType) String() string {
	return []string{"PSK", "cert", "EAP"}[t]
}

const (
	defaultIPv4SubnetCIDR     = "10.11.12.0/24"
	defaultIPv6SubnetCIDR     = "fdfd::/64"
	alternativeIPv4SubnetCIDR = "10.20.30.0/24"
	alternativeIPv6SubnetCIDR = "fdab::/64"
)

func getDefaultIPv4Subnet(cidr string) *subnet.IPv4Subnet {
	n, err := subnet.FromIPv4CIDR(cidr)
	if err != nil {
		// Construction from a const should never fail.
		panic(fmt.Sprintf("Invalid default IPv4 subnet: %v", err))
	}
	return n
}

func getDefaultIPv6Subnet(cidr string) *subnet.IPv6Subnet {
	n, err := subnet.FromIPv6CIDR(cidr)
	if err != nil {
		// Construction from a const should never fail.
		panic(fmt.Sprintf("Invalid default IPv6 subnet: %v", err))
	}
	return n
}

// Option is used in NewConfig() function to generate a VPN Config object
type Option = func(*Config)

// NewConfig creates a config object for a given VPN type. Note that currently
// it's possible to create an invalid Config (e.g., having an IPv6 DNS on IPv4
// VPN). We may want to have some validation somewhere.
func NewConfig(vpnType Type, opts ...Option) *Config {
	c := &Config{
		Type:               vpnType,
		IPsecAuthType:      AuthTypePSK,
		wgClientKeyPair:    wgDefaultClientKeyPair,
		wgServerKeyPair:    wgDefaultServerKeyPair,
		wgServerListenPort: 12345,
		ipv4Subnet:         getDefaultIPv4Subnet(defaultIPv4SubnetCIDR),
		ipv6Subnet:         getDefaultIPv6Subnet(defaultIPv6SubnetCIDR),
		dnsUseDefaultIPv4:  true,
		dnsUseDefaultIPv6:  false,
		dnsAddress:         "",
		autoConnect:        true,
	}
	for _, opt := range opts {
		opt(c)
	}
	if c.wgClientIPv4 == "" {
		c.wgClientIPv4 = c.ipv4Subnet.GetAddrEndWith(2).String()
	}
	if c.wgClientIPv6 == "" {
		c.wgClientIPv6 = c.ipv6Subnet.GetAddrEndWith(2).String()
	}
	return c
}

// WithMTU configures the MTU for this VPN connection.
func WithMTU(val int) Option {
	return func(c *Config) {
		c.MTU = val
	}
}

// WithMetered sets the Metered property to val on the shill service. False by default.
func WithMetered(val bool) Option {
	return func(c *Config) {
		c.Metered = val
	}
}

// WithSearchDomains configures the search domains for this VPN connection.
// Empty by default.
func WithSearchDomains(val []string) Option {
	return func(c *Config) {
		c.SearchDomains = val
	}
}

// WithProxyConfig configures a fake PAC config on the VPN service. Empty by
// default. The main purpose of this option is to make sure that the network
// validation mode of a VPN service won't be affected by the ProxyConfig
// property (b/344798084), and thus the value does not matter as long as the
// mode value is not "direct".
func WithProxyConfig() Option {
	return func(c *Config) {
		c.proxyConfig = "{\"mode\":\"pac_script\",\"pac_mandatory\":false,\"pac_url\":\"url\"}"
	}
}

// WithIPsecAuthType configures the authentication type for an IPsec-based VPN
// connection.
func WithIPsecAuthType(val IPsecAuthType) Option {
	return func(c *Config) {
		c.IPsecAuthType = val
	}
}

// WithL2TPIPsecXAuth enables Xauth for L2TP/IPsec.
func WithL2TPIPsecXAuth() Option {
	return func(c *Config) {
		c.IPsecUseXauth = true
	}
}

// WithOpenVPNUseUserPassword configures an OpenVPN connection with username and
// password.
func WithOpenVPNUseUserPassword() Option {
	return func(c *Config) {
		c.openVPNUseUserPassword = true
	}
}

// OpenVPNCertVerifyType represents how server certificate will be verified at
// the client side.
type OpenVPNCertVerifyType int

// Cert verify types.
const (
	OpenVPNCertVerifyNone    OpenVPNCertVerifyType = iota // no verification
	OpenVPNCertVerifySubject                              // verify the full subject
	OpenVPNCertVerifyCNOnly                               // verify only the common name
)

// WithOpenVPNCertVerify configures cert verify for OpenVPN.
func WithOpenVPNCertVerify(val OpenVPNCertVerifyType) Option {
	return func(c *Config) {
		c.openVPNCertVerify = (val != OpenVPNCertVerifyNone)
		c.openVPNCertVerifyCNOnly = (val == OpenVPNCertVerifyCNOnly)
	}
}

// WithWGUsePSK enables (or disables) PSK for WireGuard authentication. Default
// value is false.
func WithWGUsePSK(val bool) Option {
	return func(c *Config) {
		c.wgUsePSK = val
	}
}

// WithWGClientPublicKey lets the server use k as the client public key. This is
// a server-only option. If not set, a default client key will be used, which
// matches the default private key used in Connection.
func WithWGClientPublicKey(key string) Option {
	return func(c *Config) {
		c.wgClientKeyPair = wgKeyPair{
			private: "",
			public:  key,
		}
	}
}

// WithWGServerSecondKeyPair lets the server use the alternative key pair. This
// is a server-only option. It will only be useful the test need to start the
// second WireGuard server. Default value is false.
func WithWGServerSecondKeyPair(val bool) Option {
	return func(c *Config) {
		c.wgServerKeyPair = wgSecondServerKeyPair
	}
}

// WithWGClientIPv4 configures the connection to use ip as the client IPv4
// address. By default the address end with .2 in ipv4Subnet will be used.
func WithWGClientIPv4(ip string) Option {
	return func(c *Config) {
		c.wgClientIPv4 = ip
	}
}

// WithWGClientIPv6 configures the connection to use ip as the client IPv6
// address. By default the address end with ::2 in ipv6Subnet will be used.
func WithWGClientIPv6(ip string) Option {
	return func(c *Config) {
		c.wgClientIPv6 = ip
	}
}

// WithCertVals sets up the certificate value used by client.
// This is mandatory by connections using certificates for authentication.
func WithCertVals(val *CertVals) Option {
	return func(c *Config) {
		c.CertVals = val
	}
}

// WithOpenVPNTLSAuth enables TLSAuth for OpenVPN.
func WithOpenVPNTLSAuth() Option {
	return func(c *Config) {
		c.openVPNTLSAuth = true
	}
}

// OpenVPNTopology is mapped to the `--topology` option which indicates the
// virtual addressing topology used by the OpenVPN connection. See the manual of
// OpenVPN for more details. Note that this option only affect IPv4.
type OpenVPNTopology int

// OpenVPN topology types.
const (
	OpenVPNTopologyUnspecified OpenVPNTopology = iota
	OpenVPNTopologyNet30
	OpenVPNTopologyP2P
	OpenVPNTopologySubnet
)

func (t OpenVPNTopology) String() string {
	return []string{"", "net30", "p2p", "subnet"}[t]
}

// WithOpenVPNTopology configures the topology option in OpenVPN. Specifying
// this option will turn off the default route by default (note that default
// route and topology are two independent options in OpenVPN, we just do this in
// the tests).
func WithOpenVPNTopology(val OpenVPNTopology) Option {
	return func(c *Config) {
		c.openvpnTopology = val
	}
}

// WithIPType configures the IP family type of the overlay network. It’s
// IPv4-only by default. Note that not all VPN types support IPv6.
func WithIPType(val IPType) Option {
	return func(c *Config) {
		c.IPType = val
	}
}

// WithIPv4Subnet configures the IPv4 overlay used in the VPN. `.1` in the
// subnet will be used as the server address, `.2`-`.254` will be used as the
// pool for the client address (except for WireGuard, where `.2` will be used
// directly as the client address). Note that these two options are independent
// from the included routes option. The subnet will only affect the overlay IPs
// and the server-side routing setup, while the included routes option will
// affect the client-side routing setup.
func WithIPv4Subnet(n *subnet.IPv4Subnet) Option {
	return func(c *Config) {
		c.ipv4Subnet = n
	}
}

// WithIPv6Subnet configures the IPv4 overlay used in the VPN. See the comment
// above for WithIPv4Subnet for more details.
func WithIPv6Subnet(n *subnet.IPv6Subnet) Option {
	return func(c *Config) {
		c.ipv6Subnet = n
	}
}

// WithIPv4IncludedRoute sets up the VPN as split-routed. This option can be used multiple times to set up multiple included routes.
func WithIPv4IncludedRoute(route *net.IPNet) Option {
	return func(c *Config) {
		c.includedRoutesV4 = append(c.includedRoutesV4, *route)
	}
}

// WithAllowingReachUnderlayIP allows the client to reach the server listening
// only on the underlay IP (i.e., the same address which the VPN server is
// listening on) via the VPN connection. By default, those packets will be
// dropped. This option is not suggested in general, since in the test we
// usually want to set up an environment that resources available on physical
// networks and VPNs are different.
func WithAllowingReachUnderlayIP() Option {
	return func(c *Config) {
		c.allowReachUnderlayIPFromVPN = true
	}
}

// WithDNSUseDefaultIPv4 configures VPN to use the server overlay IPv4 as the
// DNS server. Currently only one DNS server is allowed, and the last WithDNS*()
// option will take effect. This is the default option.
func WithDNSUseDefaultIPv4() Option {
	return func(c *Config) {
		c.dnsUseDefaultIPv4 = true
		c.dnsUseDefaultIPv6 = false
		c.dnsAddress = ""
	}
}

// WithDNSUseDefaultIPv6 configures VPN to use the server overlay IPv6 as the
// DNS server. Currently only one DNS server is allowed, and the last WithDNS*()
// option will take effect.
func WithDNSUseDefaultIPv6() Option {
	return func(c *Config) {
		c.dnsUseDefaultIPv4 = false
		c.dnsUseDefaultIPv6 = true
		c.dnsAddress = ""
	}
}

// WithDNSAddress configures VPN to use the input ip as the DNS server.
// Currently only one DNS server is allowed, and the last WithDNS*() option will
// take effect. Call this function with an empty string will create an option to
// disable DNS on the VPN connection.
func WithDNSAddress(ip string) Option {
	return func(c *Config) {
		c.dnsUseDefaultIPv4 = false
		c.dnsUseDefaultIPv6 = false
		c.dnsAddress = ip
	}
}

// WithoutAutoConnect disables auto connecting in StartConnection(), i.e., the
// function will return with leaving the service disconnected.
func WithoutAutoConnect() Option {
	return func(c *Config) {
		c.autoConnect = false
	}
}

// WGSecondServerDefaultOptions provides a set of default options can be used to
// create the second WireGuard server, if the test doesn't care about the
// specific routing setup.
var WGSecondServerDefaultOptions = []Option{
	WithWGServerSecondKeyPair(true),
	WithIPv4Subnet(getDefaultIPv4Subnet(alternativeIPv4SubnetCIDR)),
	WithIPv6Subnet(getDefaultIPv6Subnet(alternativeIPv6SubnetCIDR)),

	// We need to reset the client IPs to make sure that the two servers use the
	// same client IP.
	WithWGClientIPv4(getDefaultIPv4Subnet(defaultIPv4SubnetCIDR).GetAddrEndWith(2).String()),
	WithWGClientIPv6(getDefaultIPv6Subnet(defaultIPv6SubnetCIDR).GetAddrEndWith(2).String()),
}

// IPType defines IP address type of overlay IP address.
type IPType int

const (
	// IPTypeIPv4 address is used for overlay IP address.
	IPTypeIPv4 IPType = iota
	// IPTypeIPv6 address is used for overlay IP address.
	IPTypeIPv6
	// IPTypeIPv4AndIPv6 is used for supporting the dual stack.
	IPTypeIPv4AndIPv6
)

// OverlayConfig contains a subset of properties of Config that can be used to
// config overlay datapath. This will mainly be used by the ARC VPN test app.
type OverlayConfig struct {
	ClientIPv4 string
}

// GetOverlayConfig returns OverlayConfig of this Config.
func (c *Config) GetOverlayConfig() *OverlayConfig {
	return &OverlayConfig{
		ClientIPv4: c.ipv4Subnet.GetAddrEndWith(2).String(),
	}
}

func (c *Config) getServerOverlayIPv4() string {
	return c.ipv4Subnet.GetAddrEndWith(1).String()
}

func (c *Config) getServerOverlayIPv6() string {
	return c.ipv6Subnet.GetAddrEndWith(1).String()
}

func (c *Config) getDNSAddress() string {
	if c.dnsUseDefaultIPv4 {
		return c.getServerOverlayIPv4()
	}
	if c.dnsUseDefaultIPv6 {
		return c.getServerOverlayIPv6()
	}
	return c.dnsAddress
}
