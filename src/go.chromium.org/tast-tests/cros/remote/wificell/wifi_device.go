// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"net"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	"go.chromium.org/tast/core/ssh"
)

// IfaceType ...
type IfaceType int

// Known IfaceType values.
const (
	// STA interface in infrastructure mode.
	StaIfaceType IfaceType = iota
	// P2P interface.
	P2PIfaceType
	// AP interface in infrastructure mode.
	APIfaceType
)

// WiFiDeviceType ...
type WiFiDeviceType int

// Known WiFiDevice types.
const (
	// CrOSDevice.
	CrOSDevice WiFiDeviceType = iota
	// AndroidDevice.
	AndroidDevice
)

// WiFiDevice is any Device that can perform certain WiFi-related operations.
// This can be Chromebook, Android, RPi, whatever.
type WiFiDevice interface {
	// Type returns WiFiDeviceType.
	Type() WiFiDeviceType

	// Conn returns SSH connection object for this device.
	Conn() *ssh.Conn

	// IPv4Addrs returns the IPv4 addresses for the network interface.
	IPv4Addrs(ctx context.Context, ifType IfaceType) ([]net.IP, error)

	// Ping sends ICMP pings from the device to a particular destination.
	Ping(ctx context.Context, addr string, ifType IfaceType, opts ...ping.Option) (*ping.Result, error)

	// PingDevice sends ICMP pings from the device to another WiFiDevice.
	PingDevice(ctx context.Context, dest WiFiDevice, srcIfType, dstIfType IfaceType, opts ...ping.Option) (*ping.Result, error)

	// IfName returns name of the interface of a particular type.
	IfName(ctx context.Context, ifType IfaceType) (string, error)
}

// P2PWiFiDevice is a WiFiDevice that can perform P2P (WiFi Direct)-related operations.
type P2PWiFiDevice interface {
	WiFiDevice
	// P2PSSID returns P2P SSID of the device.
	P2PSSID() string
	// P2PPassphrase returns passphrase for P2P group handled by the device.
	P2PPassphrase() string
	// P2PFrequency returns frequency for P2P group handled by the device.
	P2PFrequency() uint32
	// P2PMACAddress returns MAC Address for P2P group handled by the device.
	P2PMACAddress() string
	// P2PGroupCreate creates WiFi Direct Group and takes its ownership.
	P2PGroupCreate(ctx context.Context, ops ...p2p.GroupOption) error
	// P2PGroupDelete deletes the existing WiFi Direct Group.
	P2PGroupDelete(ctx context.Context) error
	// P2PGroupConnect handles connection to the existing WiFi Direct Group.
	P2PGroupConnect(ctx context.Context, device P2PWiFiDevice) error
	// P2PGroupDisconnect handles disconnection from the existing WiFi Direct Group.
	P2PGroupDisconnect(ctx context.Context) error
}
