// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package owe provides a Config type for an OWE network.
package owe

import (
	"context"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/pkcs11/netcertstore"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast/core/ssh"
)

// Config implements security.Config interface for an OWE network
// - there is nothing included because it behaves as an open network
// although the communication is encrypted.
type Config struct{}

// Static check: Config implements security.Config interface.
var _ security.Config = (*Config)(nil)

// ConfigFactory provides Gen method to build a new Config.
type ConfigFactory struct{}

// Gen builds a Config.
func (*ConfigFactory) Gen() (security.Config, error) {
	return &Config{}, nil
}

// NewConfigFactory builds a ConfigFactory.
func NewConfigFactory() *ConfigFactory {
	return &ConfigFactory{}
}

// Static check: ConfigFactory implements security.ConfigFactory interface.
var _ security.ConfigFactory = (*ConfigFactory)(nil)

// Class returns the security class of an open network - there is no
// pre-shared secret, OWE network behaves like open network.
func (*Config) Class() string {
	return shillconst.SecurityClassNone
}

// HostapdConfig returns hostapd config options specific for an OWE network.
func (*Config) HostapdConfig() (map[string]string, error) {
	var ret = make(map[string]string)
	ret["wpa"] = strconv.Itoa(int(wpa.ModePureWPA2))
	ret["wpa_key_mgmt"] = "OWE"
	ret["rsn_pairwise"] = string(wpa.CipherCCMP)

	return ret, nil
}

// ShillServiceProperties returns shill properties of an open network.
func (*Config) ShillServiceProperties() (map[string]interface{}, error) {
	return nil, nil
}

// NeedsNetCertStore tells that netcert store is not necessary for this configuration.
func (*Config) NeedsNetCertStore() bool {
	return false
}

// InstallRouterCredentials installs the necessary credentials onto router.
func (*Config) InstallRouterCredentials(context.Context, *ssh.Conn, string) error {
	return nil
}

// InstallClientCredentials installs the necessary credentials onto DUT.
func (*Config) InstallClientCredentials(context.Context, *netcertstore.Store) error {
	return nil
}
