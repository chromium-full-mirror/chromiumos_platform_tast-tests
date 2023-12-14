// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package vpn

import (
	"context"
	"crypto/sha1"
	"crypto/x509"
	"encoding/pem"
	"fmt"
	"net"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/local/network/ping"
	"go.chromium.org/tast-tests/cros/local/network/routing"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet/subnet"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Config contains the parameters (for both client and server) to configure a
// VPN connection.
type Config struct {
	Type          Type
	MTU           int
	Metered       bool
	SearchDomains []string

	IPsecAuthType IPsecAuthType

	// Parameters for an L2TP/IPsec VPN connection.
	IPsecUseXauth         bool
	IPsecXauthMissingUser bool
	IPsecXauthWrongUser   bool
	UnderlayIPIsOverlayIP bool

	// Parameters for an OpenVPN connection.
	OpenVPNUseUserPassword        bool
	OpenVPNCertVerify             bool
	OpenVPNCertVerifyWrongHash    bool
	OpenVPNCertVerifyWrongSubject bool
	OpenVPNCertVerifyWrongCN      bool
	OpenVPNCertVerifyCNOnly       bool
	OpenVPNTLSAuth                bool
	openvpnTopology               OpenVPNTopology

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
	CertVals CertVals

	ipv4Subnet *subnet.IPv4Subnet
	ipv6Subnet *subnet.IPv6Subnet

	includedRoutesV4 []net.IPNet

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
)

func (t Type) String() string {
	return []string{"IKEv2", "L2TP/IPsec", "OpenVPN", "WireGuard"}[t]
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

// NewConfig creates a config object for a given VPN type
func NewConfig(vpnType Type, opts ...Option) *Config {
	c := &Config{
		Type:               vpnType,
		IPsecAuthType:      AuthTypePSK,
		wgClientKeyPair:    wgDefaultClientKeyPair,
		wgServerKeyPair:    wgDefaultServerKeyPair,
		wgServerListenPort: 12345,
		ipv4Subnet:         getDefaultIPv4Subnet(defaultIPv4SubnetCIDR),
		ipv6Subnet:         getDefaultIPv6Subnet(defaultIPv6SubnetCIDR),
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

// WithIPsecAuthType configures the authentication type for an IPsec-based VPN
// connection.
func WithIPsecAuthType(val IPsecAuthType) Option {
	return func(c *Config) {
		c.IPsecAuthType = val
	}
}

// L2TPIPSecXauthType represents how Xauth is configured for L2TP/IPsec.
// TODO(b/257379393): Remove this type after we move the related code into
// VPNConnect.
type L2TPIPSecXauthType int

// Xauth types.
const (
	L2TPIPsecXauthNone L2TPIPSecXauthType = iota
	L2TPIPsecXauthCorrect
	L2TPIPsecXauthWrongUser
	L2TPIPsecXauthMissingUser
)

// WithL2TPIPsecXAuth configures Xauth for L2TP/IPsec.
func WithL2TPIPsecXAuth(val L2TPIPSecXauthType) Option {
	return func(c *Config) {
		c.IPsecUseXauth = (val != L2TPIPsecXauthNone)
		switch val {
		case L2TPIPsecXauthNone, L2TPIPsecXauthCorrect:
			break
		case L2TPIPsecXauthWrongUser:
			c.IPsecXauthWrongUser = true
		case L2TPIPsecXauthMissingUser:
			c.IPsecXauthMissingUser = true
		}
	}
}

// WithOpenVPNUseUserPassword configures an OpenVPN connection with username and
// password.
func WithOpenVPNUseUserPassword() Option {
	return func(c *Config) {
		c.OpenVPNUseUserPassword = true
	}
}

// OpenVPNCertVerifyType represents how cert verify is configured for OpenVPN.
// TODO(b/257379393): Remove this type after we move the related code into
// VPNConnect.
type OpenVPNCertVerifyType int

// Cert verify types.
const (
	OpenVPNCertVerifyNone OpenVPNCertVerifyType = iota
	OpenVPNCertVerifyCorrect
	OpenVPNCertVerifyWrongHash
	OpenVPNCertVerifyWrongSubject
	OpenVPNCertVerifyWrongCN
	OpenVPNCertVerifyCNOnly
)

// WithOpenVPNCertVerify configures cert verify for OpenVPN.
func WithOpenVPNCertVerify(val OpenVPNCertVerifyType) Option {
	return func(c *Config) {
		c.OpenVPNCertVerify = (val != OpenVPNCertVerifyNone)
		switch val {
		case OpenVPNCertVerifyNone, OpenVPNCertVerifyCorrect:
			break
		case OpenVPNCertVerifyWrongHash:
			c.OpenVPNCertVerifyWrongHash = true
		case OpenVPNCertVerifyWrongSubject:
			c.OpenVPNCertVerifyWrongSubject = true
		case OpenVPNCertVerifyWrongCN:
			c.OpenVPNCertVerifyWrongCN = true
		case OpenVPNCertVerifyCNOnly:
			c.OpenVPNCertVerifyCNOnly = true
		}
	}
}

// WithWGUsePSK enables (or disables) PSK for WireGuard authentication. Default
// value is false.
func WithWGUsePSK(val bool) Option {
	return func(c *Config) {
		c.wgUsePSK = val
	}
}

// WithWGAutoGenKey lets shill generate a key pair for WireGuard instead of
// using a given one.
// TODO(b/257379393): Remove this function after we the related code into
// VPNConnect.
func WithWGAutoGenKey() Option {
	return func(c *Config) {
		c.wgClientKeyPair = wgKeyPair{}
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
func WithCertVals(val CertVals) Option {
	return func(c *Config) {
		c.CertVals = val
	}
}

// WithOpenVPNTLSAuth enables TLSAuth for OpenVPN.
func WithOpenVPNTLSAuth() Option {
	return func(c *Config) {
		c.OpenVPNTLSAuth = true
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

// WithUnderlayIPIsOverlayIP makes the VPN connection use the same IP address for
// overlay network and underlay network.
func WithUnderlayIPIsOverlayIP() Option {
	return func(c *Config) {
		c.UnderlayIPIsOverlayIP = true
	}
}

// WithIPv4IncludedRoute sets up the VPN as split-routed. This option can be used multiple times to set up multiple included routes.
func WithIPv4IncludedRoute(route *net.IPNet) Option {
	return func(c *Config) {
		c.includedRoutesV4 = append(c.includedRoutesV4, *route)
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

// Connection represents a VPN connection can be used in the test.
type Connection struct {
	// routingEnv will only be set and used when StartConnection() is called with
	// a nil env.
	routingEnv *routing.TestEnv

	Server       *Server
	SecondServer *Server

	config  Config
	service *shill.Service
}

// StartConnection creates a VPN connection on DUT, including the following steps:
//   - If env is nil, create a virtualnet env on DUT;
//   - Set up the VPN server in env;
//   - Create and configure a VPN service profile in shill;
//   - Connect the VPN service and wait for the service connected.
//
// On success, the caller should call Cleanup() on the returned Connection to
// release the resources after the test is done.
func StartConnection(ctx context.Context, env *virtualnet.Env, vpnType Type, opts ...Option) (*Connection, error) {
	config := NewConfig(vpnType, opts...)
	conn := &Connection{config: *config}
	cleanupCtx, _ := ctxutil.Shorten(ctx, 5*time.Second)
	success := false
	defer func() {
		if !success {
			conn.Cleanup(cleanupCtx)
		}
	}()

	if err := conn.startServer(ctx, env); err != nil {
		return nil, errors.Wrap(err, "failed to start VPN server")
	}

	svc, err := ConfigureService(ctx, conn.Server, nil, &conn.config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to configure shill service")
	}
	conn.service = svc

	if config.autoConnect {
		if err := conn.Connect(ctx); err != nil {
			return nil, errors.Wrap(err, "failed to connect the VPN service")
		}
	}

	success = true
	return conn, nil
}

// Connect lets shill connect to the VPN service, waits for it connected, and
// then verifies the routing layer setup before return.
func (c *Connection) Connect(ctx context.Context) error {
	testing.ContextLog(ctx, "Waiting for VPN service connected")
	if err := c.service.Connect(ctx); err != nil {
		return errors.Wrap(err, "failed to call Connect on service")
	}
	if err := c.service.WaitForConnectedOrError(ctx); err != nil {
		return errors.Wrap(err, "failed to wait for service connected")
	}
	if err := ping.ExpectPingSuccessWithTimeout(ctx, c.Server.OverlayIPv4, "chronos", 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to verify VPN routing")
	}

	var overlayIPArray []string
	if c.config.IPType == IPTypeIPv4 || c.config.IPType == IPTypeIPv4AndIPv6 {
		overlayIPArray = append(overlayIPArray, c.Server.OverlayIPv4)
	}
	if c.config.IPType == IPTypeIPv6 || c.config.IPType == IPTypeIPv4AndIPv6 {
		overlayIPArray = append(overlayIPArray, c.Server.OverlayIPv6)
	}
	testing.ContextLogf(ctx, "VPN connected, underlay_ip is %s, overlay_ip list is %s", c.Server.UnderlayIP, strings.Join(overlayIPArray, ","))

	return nil
}

// Disconnect will disconnect the shill service. This does not clean up the VPN server
// and callers should still call Cleanup().
func (c *Connection) Disconnect(ctx context.Context) error {
	testing.ContextLog(ctx, "Disconnecting service: ", c.service)
	return c.service.Disconnect(ctx)
}

// Cleanup removes the service from shill, and releases other resources used for
// the connection. Callers don't necessarily need to call Disconnect() before this.
func (c *Connection) Cleanup(ctx context.Context) error {
	var lastErr error

	// Removes service from the profile.
	if c.service != nil {
		if err := c.service.Remove(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to remove service from profile: ", err)
			lastErr = err
		}
	}

	// Shuts down server.
	if c.Server != nil {
		if err := c.Server.Exit(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop VPN server: ", err)
			lastErr = err
		}
	}

	// Stops routing env.
	if c.routingEnv != nil {
		if err := c.routingEnv.TearDown(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop routing env: ", err)
			lastErr = err
		}
	}

	// Makes sure charon is stopped for IPsec-based VPNs.
	if err := waitForCharonExitOrKill(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to wait for charon to stop: ", err)
		lastErr = err
	}

	return lastErr
}

func (c *Connection) startServer(ctx context.Context, env *virtualnet.Env) error {
	if env == nil {
		c.routingEnv = routing.NewTestEnvWithoutResetProfile()
		if err := c.routingEnv.SetUp(ctx); err != nil {
			return errors.Wrap(err, "failed to setup routing env")
		}
		env = c.routingEnv.BaseServer
	}

	var err error
	if c.config.Type != TypeWireGuard {
		c.Server, err = StartServerWithConfig(ctx, env, &c.config)
		return err
	}

	if c.config.wgClientKeyPair.public == "" {
		c.config.wgClientKeyPair.public, err = c.generateWireGuardKey(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get public key")
		}
	}
	c.Server, err = StartServerWithConfig(ctx, env, &c.config)
	return err
}

// ConfigureService create a VPN service to server profile in shill based on
// config. It's caller's responsibility to remove the service from the profile
// after test is done.
func ConfigureService(ctx context.Context, server, secondServer *Server, config *Config) (*shill.Service, error) {
	m, err := shill.NewManager(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed creating shill manager proxy")
	}

	props, err := CreateProperties(server, secondServer, config)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create shill properties for VPN")
	}

	servicePath, err := m.ConfigureService(ctx, props)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to configure the service for the VPN properties %v", props)
	}

	return shill.NewService(ctx, servicePath)
}

// generateWireGuardKey calls configureService() to create an "empty" WireGuard
// service in shill, and then reads and returns the generated public key from
// the service properties. The service created in the profile in this step will
// be overwritten with the full properties after the server is created.
func (c *Connection) generateWireGuardKey(ctx context.Context) (string, error) {
	var err error
	c.service, err = ConfigureService(ctx, c.Server, c.SecondServer, &c.config)
	if err != nil {
		return "", err
	}
	properties, err := c.service.GetProperties(ctx)
	if err != nil {
		return "", err
	}
	provider, err := properties.Get(shillconst.ServicePropertyProvider)
	if err != nil {
		return "", err
	}
	providerMap, ok := provider.(map[string]interface{})
	if !ok {
		return "", errors.New("failed to read Provider property as map")
	}
	publicKey, ok := providerMap["WireGuard.PublicKey"].(string)
	if !ok {
		return "", errors.New("failed to read WireGuard.PublicKey property as string")
	}
	if len(publicKey) != len(wgDefaultClientKeyPair.public) {
		return "", errors.Errorf("generated key is not valid: %s", publicKey)
	}
	return publicKey, nil
}

// CreateProperties returns a dict which contains the D-Bus property values of a
// VPN service for the given config. secondServer is only for WireGuard.
func CreateProperties(server, secondServer *Server, config *Config) (map[string]interface{}, error) {
	if config.Type != TypeWireGuard && secondServer != nil {
		return nil, errors.New("second server should only be set for wireguard")
	}

	properties, err := func() (map[string]interface{}, error) {
		switch config.Type {
		case TypeIKEv2:
			return createIKEv2Properties(server, config)
		case TypeL2TPIPsec:
			return createL2TPIPsecProperties(server, config)
		case TypeOpenVPN:
			return createOpenVPNProperties(server, config)
		case TypeWireGuard:
			return createWireGuardProperties(server, secondServer, config)
		default:
			return nil, errors.Errorf("unexpected server type: got %s", config.Type)
		}
	}()
	if err != nil {
		return nil, err
	}

	properties["Metered"] = config.Metered
	staticIPConfig, ok := properties["StaticIPConfig"].(map[string]interface{})
	if !ok {
		staticIPConfig = make(map[string]interface{})
		properties["StaticIPConfig"] = staticIPConfig
	}
	staticIPConfig["Mtu"] = config.MTU
	staticIPConfig["SearchDomains"] = config.SearchDomains

	return properties, nil
}

func createL2TPIPsecProperties(server *Server, config *Config) (map[string]interface{}, error) {
	var serverAddress string
	if config.UnderlayIPIsOverlayIP {
		serverAddress = server.OverlayIPv4
	} else {
		serverAddress = server.UnderlayIP
	}

	properties := map[string]interface{}{
		"Provider.Host":      serverAddress,
		"Provider.Type":      "l2tpipsec",
		"Type":               "vpn",
		"L2TPIPsec.User":     chapUser,
		"L2TPIPsec.Password": chapSecret,
		"SaveCredentials":    true,
	}

	if config.IPsecAuthType == AuthTypePSK {
		properties["Name"] = "test-vpn-l2tp-psk"
		properties["L2TPIPsec.PSK"] = ipsecPresharedKey
	} else if config.IPsecAuthType == AuthTypeCert {
		properties["Name"] = "test-vpn-l2tp-cert"
		properties["L2TPIPsec.CACertPEM"] = []string{certificate.TestCert1().CACred.Cert}
		properties["L2TPIPsec.ClientCertID"] = config.CertVals.id
		properties["L2TPIPsec.ClientCertSlot"] = config.CertVals.slot
		properties["L2TPIPsec.PIN"] = config.CertVals.pin
	} else {
		return nil, errors.Errorf("unexpected auth type %s for L2TP/IPsec", config.IPsecAuthType)
	}

	if config.IPsecUseXauth && !config.IPsecXauthMissingUser {
		if config.IPsecXauthWrongUser {
			properties["L2TPIPsec.XauthUser"] = "wrong-user"
			properties["L2TPIPsec.XauthPassword"] = "wrong-password"
		} else {
			properties["L2TPIPsec.XauthUser"] = xauthUser
			properties["L2TPIPsec.XauthPassword"] = xauthPassword
		}
	}

	return properties, nil
}

func createIKEv2Properties(server *Server, config *Config) (map[string]interface{}, error) {
	properties := map[string]interface{}{
		"Name":          "test-ikev2-vpn",
		"Provider.Host": server.UnderlayIP,
		"Provider.Type": "ikev2",
		"Type":          "vpn",
	}

	switch config.IPsecAuthType {
	case AuthTypePSK:
		properties["IKEv2.AuthenticationType"] = "PSK"
		properties["IKEv2.LocalIdentity"] = ikeClientIdentity
		properties["IKEv2.RemoteIdentity"] = ikeServerIdentity
		properties["IKEv2.PSK"] = ipsecPresharedKey
	case AuthTypeCert:
		properties["IKEv2.AuthenticationType"] = "Cert"
		properties["IKEv2.CACertPEM"] = []string{certificate.TestCert1().CACred.Cert}
		properties["IKEv2.ClientCertID"] = config.CertVals.id
		properties["IKEv2.ClientCertSlot"] = config.CertVals.slot
		properties["IKEv2.RemoteIdentity"] = ikeServerIdentity
	case AuthTypeEAP:
		properties["IKEv2.AuthenticationType"] = "EAP"
		properties["IKEv2.CACertPEM"] = []string{certificate.TestCert1().CACred.Cert}
		properties["EAP.EAP"] = "MSCHAPV2"
		properties["EAP.Identity"] = xauthUser
		properties["EAP.Password"] = xauthPassword
	default:
		return nil, errors.Errorf("unexpected auth type %s for IKEv2", config.IPsecAuthType)
	}

	return properties, nil
}

func createOpenVPNProperties(server *Server, config *Config) (map[string]interface{}, error) {
	properties := map[string]interface{}{
		"Name":                  "test-vpn-openvpn",
		"Provider.Host":         server.UnderlayIP,
		"Provider.Type":         "openvpn",
		"Type":                  "vpn",
		"OpenVPN.CACertPEM":     []string{certificate.TestCert1().CACred.Cert},
		"OpenVPN.Pkcs11.ID":     config.CertVals.id,
		"OpenVPN.Pkcs11.PIN":    config.CertVals.pin,
		"OpenVPN.RemoteCertEKU": "TLS Web Server Authentication",
		"OpenVPN.Verb":          "5",
		"SaveCredentials":       true,
	}

	if config.OpenVPNUseUserPassword {
		properties["OpenVPN.User"] = openvpnUsername
		properties["OpenVPN.Password"] = openvpnPassword
	}

	if config.OpenVPNTLSAuth {
		properties["OpenVPN.TLSAuthContents"] = openvpnTLSAuthKey
	}

	if config.OpenVPNCertVerify {
		if config.OpenVPNCertVerifyWrongHash {
			properties["OpenVPN.VerifyHash"] = "00" + strings.Repeat(":00", 19)
		} else {
			certBlock, _ := pem.Decode([]byte(certificate.TestCert1().CACred.Cert))
			caCert, err := x509.ParseCertificate(certBlock.Bytes)
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse CA cert")
			}
			// Translates the form of SHA-1 hash from []byte to "xx:xx:...:xx".
			properties["OpenVPN.VerifyHash"] = strings.ReplaceAll(fmt.Sprintf("% 02x", sha1.Sum(caCert.Raw)), " ", ":")
		}

		if config.OpenVPNCertVerifyWrongSubject {
			properties["OpenVPN.VerifyX509Name"] = "bogus subject name"
		} else if config.OpenVPNCertVerifyWrongCN {
			properties["OpenVPN.VerifyX509Name"] = "bogus cn"
			properties["OpenVPN.VerifyX509Type"] = "name"
		} else if config.OpenVPNCertVerifyCNOnly {
			// This can be parsed from certificate.TestCert1().ServerCred.Cert .
			properties["OpenVPN.VerifyX509Name"] = "chromelab-wifi-testbed-server.mtv.google.com"
			properties["OpenVPN.VerifyX509Type"] = "name"
		} else {
			// This can be parsed from certificate.TestCert1().ServerCred.Cert, but
			// the output format of String() function of the parsed result does not
			// match the format that OpenVPN expects.
			properties["OpenVPN.VerifyX509Name"] = "C=US, ST=California, L=Mountain View, CN=chromelab-wifi-testbed-server.mtv.google.com"
		}
	}

	return properties, nil
}

func createWireGuardProperties(server, secondServer *Server, config *Config) (map[string]interface{}, error) {
	var peers []map[string]string

	// Helper function to generate AllowedIPs string according to IPType.
	genAllowedIPs := func(v4, v6 string) string {
		switch config.IPType {
		case IPTypeIPv4:
			return v4
		case IPTypeIPv6:
			return v6
		case IPTypeIPv4AndIPv6:
			return v4 + "," + v6
		}
		return ""
	}

	getEndpoint := func(ip string, port int) string {
		return fmt.Sprintf("%s:%d", ip, port)
	}

	// TODO(b/257379393): Generate the props from server.Config instead of config.
	var nameServers []string
	if server != nil {
		peer := map[string]string{
			"PublicKey":  server.Config.wgServerKeyPair.public,
			"Endpoint":   getEndpoint(server.UnderlayIP, server.Config.wgServerListenPort),
			"AllowedIPs": genAllowedIPs("0.0.0.0/0", "::/0"),
		}
		if config.wgUsePSK {
			peer["PresharedKey"] = wgPresharedKey
		}
		if secondServer != nil {
			// Do not set "default route" if we have two peers.
			peer["AllowedIPs"] = genAllowedIPs(
				server.Config.ipv4Subnet.String(),
				server.Config.ipv6Subnet.String(),
			)
		}
		peers = append(peers, peer)

		if config.IPType != IPTypeIPv6 {
			nameServers = append(nameServers, server.OverlayIPv4)
		}
		if config.IPType != IPTypeIPv4 {
			nameServers = append(nameServers, server.OverlayIPv6)
		}
	}

	if secondServer != nil {
		peers = append(peers, map[string]string{
			"PublicKey": secondServer.Config.wgServerKeyPair.public,
			"Endpoint":  getEndpoint(secondServer.UnderlayIP, secondServer.Config.wgServerListenPort),
			"AllowedIPs": genAllowedIPs(
				secondServer.Config.ipv4Subnet.String(),
				secondServer.Config.ipv6Subnet.String(),
			),
			"PresharedKey": wgPresharedKey,
		})
	}

	staticIPConfig := map[string]interface{}{
		"NameServers": nameServers,
	}
	properties := map[string]interface{}{
		"Name":            "test-vpn-wg",
		"Provider.Host":   "wireguard",
		"Provider.Type":   "wireguard",
		"Type":            "vpn",
		"WireGuard.Peers": peers,
		"StaticIPConfig":  staticIPConfig,
		"SaveCredentials": true, // Not required, just to avoid a WARNING log in shill
	}
	if config.wgClientKeyPair.private != "" {
		properties["WireGuard.PrivateKey"] = config.wgClientKeyPair.private
	}

	// Note that server can be nil in this function (for letting shill generate the key pair
	// before the server starts). We can just pass in some default values in this case.
	clientIPv4 := "0.0.0.0"
	clientIPv6 := "::"
	if server != nil {
		// TODO(b/257379393): Check if the configs from the two server provides the
		// same client IP.
		clientIPv4 = server.Config.wgClientIPv4
		clientIPv6 = server.Config.wgClientIPv6
	}
	switch config.IPType {
	case IPTypeIPv4:
		properties["WireGuard.IPAddress"] = []string{clientIPv4}
	case IPTypeIPv6:
		properties["WireGuard.IPAddress"] = []string{clientIPv6}
	case IPTypeIPv4AndIPv6:
		properties["WireGuard.IPAddress"] = []string{clientIPv4, clientIPv6}
	}
	return properties, nil
}

// Service gets service of this connection.
func (c *Connection) Service() *shill.Service {
	return c.service
}
