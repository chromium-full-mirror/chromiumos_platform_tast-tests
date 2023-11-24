// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package owe provides a Config type for an OWE network.
package owe

import (
	"context"
	"net"
	"strconv"

	"go.chromium.org/tast-tests/cros/common/pkcs11/netcertstore"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/hostap"
	"go.chromium.org/tast-tests/cros/common/wifi/security"
	"go.chromium.org/tast-tests/cros/common/wifi/security/wpa"
	"go.chromium.org/tast/core/ssh"
)

// ModeEnum is a type of BSS in AP using OWE - see also the constants below.
type ModeEnum int

// Types of OWE BSSes: pure OWE AP and transitional mode one (this is
// pair of APs one open and one hidden using OWE).
const (
	ModePureOWE ModeEnum = 1 << iota
	ModeTransOpen
	ModeTransOWE
)

// Config implements security.Config interface for an OWE network.
type Config struct {
	mode  ModeEnum
	ssid  string
	bssid string
}

// TransOption is the function signature used to specify options of Config.
type TransOption func(*Config)

// SSID returns an Option which sets the "owe" companion ssid in hostapd config.
func SSID(ssid string) TransOption {
	return func(c *Config) {
		c.ssid = ssid
	}
}

// BSSID returns an Option which sets the "owe" companion bssid in hostapd config.
func BSSID(bssid net.HardwareAddr) TransOption {
	return func(c *Config) {
		c.bssid = bssid.String()
	}
}

// Static check: Config implements security.Config interface.
var _ security.Config = (*Config)(nil)

// ConfigFactory provides Gen method to build a new Config.
type ConfigFactory struct {
	mode ModeEnum
	ops  []TransOption
}

// Gen builds a Config.
func (cf *ConfigFactory) Gen() (security.Config, error) {
	conf := &Config{mode: cf.mode}
	if conf.mode == ModePureOWE {
		return conf, nil
	}
	for _, op := range cf.ops {
		op(conf)
	}
	return conf, nil
}

// NewConfigFactory builds a ConfigFactory.
func NewConfigFactory(m ModeEnum, ops ...TransOption) *ConfigFactory {
	return &ConfigFactory{
		mode: m,
		ops:  ops,
	}
}

// Static check: ConfigFactory implements security.ConfigFactory interface.
var _ security.ConfigFactory = (*ConfigFactory)(nil)

// Security returns security of OWE network.
func (c *Config) Security() (string, error) {
	if c.mode == ModePureOWE {
		return shillconst.SecurityOWE, nil
	}
	return shillconst.SecurityTransOWE, nil
}

// Class returns the security class of an open network - there is no
// pre-shared secret, OWE network behaves like open network.
func (*Config) Class() string {
	return shillconst.SecurityClassNone
}

// HostapdConfig returns hostapd config options specific for an OWE network.
func (c *Config) HostapdConfig() (map[string]string, error) {
	var ret = make(map[string]string)
	if c.mode == ModePureOWE || c.mode == ModeTransOWE {
		ret["wpa"] = strconv.Itoa(int(wpa.ModePureWPA2))
		ret["wpa_key_mgmt"] = "OWE"
		ret["rsn_pairwise"] = string(wpa.CipherCCMP)
		ret["ieee80211w"] = "2"
	}
	if c.mode == ModeTransOWE {
		ret["ignore_broadcast_ssid"] = "1"
	}
	if c.mode != ModePureOWE {
		ret["owe_transition_ssid"] = hostap.EncodeSSID(c.ssid)
		ret["owe_transition_bssid"] = c.bssid
	}

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
