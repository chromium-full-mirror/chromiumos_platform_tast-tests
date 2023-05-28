// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package eap contains utilities for manipulating EAP access points
// during tests.
package eap

import (
	"bytes"
	"context"
	"fmt"
	"io/ioutil"
	"path/filepath"
	"text/template"

	"go.chromium.org/tast-tests/cros/common/crypto/certificate"
	"go.chromium.org/tast-tests/cros/local/hostapd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// TestUser is the username used for setting up EAP credential for AP.
	TestUser = "test-user"
	// TestPassword is password used for setting up EAP credential for AP.
	TestPassword = "test-password"
)

// Auth is the authentication method the access point will expose.
type Auth int

const (
	// AuthTLS represents the EAP-TLS authentication method.
	AuthTLS Auth = iota
	// AuthTTLS represents the EAP-TTLS with MSCHAPv2 authentication method.
	AuthTTLS
)

// AccessPoint describes a EAP WiFi compatible access point with its match criteria.
type AccessPoint struct {
	// SSID is the name of the network.
	SSID string
	// Auth is the EAP network authentication.
	Auth
}

// ToServer transforms the Passpoint access point descriptions into a valid hostapd service instance.
func (ap *AccessPoint) ToServer(iface, outDir string, certs certificate.CertStore) *hostapd.Server {
	return hostapd.NewServer(
		iface,
		filepath.Join(outDir, iface),
		NewAPConf(
			ap.SSID,
			ap.Auth,
			TestUser,
			TestPassword,
			&certs,
		),
	)
}

// APConf contains the parameters required to setup a EAP WiFi
// compatible access point.
type APConf struct {
	// ssid is the name of the access point.
	ssid string
	// auth is the authentication method exposed by the access point.
	auth Auth
	// identity is the username of the EAP user (EAP-TTLS).
	identity string
	// password is the secret of the EAP user (EAP-TTLS).
	password string
	// cert is the set of certificates used by the radius server to prove its
	// identity and authenticate the user (EAP-TLS).
	cert *certificate.CertStore
}

// NewAPConf creates a new EAP WiFi compatible access point configuration from the parameters.
func NewAPConf(ssid string, auth Auth, identity, password string, cert *certificate.CertStore) *APConf {
	return &APConf{
		ssid:     ssid,
		auth:     auth,
		identity: identity,
		password: password,
		cert:     cert,
	}
}

// Generate transforms the configuration parameters in a set of configuration
// files suitable for hostapd.
func (c APConf) Generate(ctx context.Context, dir, ctrlPath string) (string, error) {
	serverCertPath := filepath.Join(dir, "cert")
	privateKeyPath := filepath.Join(dir, "private_key")
	eapUserFilePath := filepath.Join(dir, "eap_user")
	caCertPath := filepath.Join(dir, "ca_cert")
	confPath := filepath.Join(dir, "hostapd.conf")

	// Create the radius users configuration.
	eapUsers, err := c.prepareEAPUsers()
	testing.ContextLog(ctx, "eapUsers")
	testing.ContextLog(ctx, eapUsers)
	if err != nil {
		return "", errors.Wrap(err, "failed to prepare EAP users file")
	}

	confContents, err := c.prepareConf(ctrlPath, caCertPath, serverCertPath, privateKeyPath, eapUserFilePath)
	testing.ContextLog(ctx, "confContents")
	testing.ContextLog(ctx, confContents)
	if err != nil {
		return "", errors.Wrap(err, "failed to prepare configuration file")
	}

	for _, p := range []struct {
		path     string
		contents string
	}{
		{confPath, confContents},
		{serverCertPath, c.cert.ServerCred.Cert},
		{privateKeyPath, c.cert.ServerCred.PrivateKey},
		{eapUserFilePath, eapUsers},
		{caCertPath, c.cert.CACred.Cert},
	} {
		if err := ioutil.WriteFile(p.path, []byte(p.contents), 0644); err != nil {
			return "", errors.Wrapf(err, "failed to write file %q", p.path)
		}
	}

	return confPath, nil
}

// prepareEAPUsers creates the content of the radius users file that describes
// how to authenticate users.
func (c APConf) prepareEAPUsers() (string, error) {
	switch c.auth {
	case AuthTLS:
		// TLS auth only requires an outer authentication
		return `# Outer authentication
* TLS`, nil
	case AuthTTLS:
		// TTLS requires outer and inner authentication
		return fmt.Sprintf(`# Outer authentication
* TTLS
# Inner authentication
"%s" TTLS-MSCHAPV2 "%s" [2]`, c.identity, c.password), nil
	default:
		return "", errors.Errorf("unsupported authentication method: %v", c.auth)
	}
}

// prepareConf generates the content of hostapd configuration file.
func (c APConf) prepareConf(socketPath, caPath, certPath, keyPath, eapUsers string) (string, error) {
	tmpl, err := template.New("").Parse(`
ctrl_interface={{.CtrlSocket}}
# Wireless configuration
ssid={{.SSID}}
hw_mode=g
channel=1
# Enable EAP authentication and server
ieee8021x=1
eapol_version=2
eap_server=1
ca_cert={{.CaCert}}
server_cert={{.ServerCert}}
private_key={{.PrivateKey}}
eap_user_file={{.EapUsers}}
# Security
wpa=2
wpa_key_mgmt=WPA-EAP WPA-EAP-SHA256
wpa_pairwise=CCMP
ieee80211w=1
`)
	if err != nil {
		return "", errors.Wrap(err, "failed to parse configuration template")
	}

	var buf bytes.Buffer
	if err := tmpl.Execute(&buf, struct {
		CtrlSocket string
		SSID       string
		CaCert     string
		ServerCert string
		PrivateKey string
		EapUsers   string
	}{
		CtrlSocket: socketPath,
		SSID:       c.ssid,
		CaCert:     caPath,
		ServerCert: certPath,
		PrivateKey: keyPath,
		EapUsers:   eapUsers,
	}); err != nil {
		return "", errors.Wrap(err, "failed to execute hostapd configuration template")
	}
	return buf.String(), nil
}

func (c APConf) String() string {
	return fmt.Sprintf("%q", c.ssid)
}
