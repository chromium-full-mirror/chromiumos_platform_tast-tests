// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package hostapd

// Iface represents an interface used by hostapd.
type Iface struct {
	name string
	conf *Config
}

// Name returns the name of the interface.
func (i *Iface) Name() string {
	return i.name
}

// Config returns the config of the interface.
func (i *Iface) Config() *Config {
	return i.conf
}

// FormatConfig composes a hostapd.conf based on the given Config and ctrlPath.
// iface is the network interface for the hostapd to run. ctrlPath is the control
// file path for hostapd to communicate with hostapd_cli.
func (i *Iface) FormatConfig(ctrlPath string) (string, error) {
	return i.conf.Format(i.name, ctrlPath)
}

// NewIface creates an interface
func NewIface(name string, conf *Config) *Iface {
	return &Iface{name, conf}
}
