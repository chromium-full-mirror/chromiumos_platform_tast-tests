// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"net"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/pkcs11/netcertstore"
	"go.chromium.org/tast-tests/cros/common/shillconst"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	remoteping "go.chromium.org/tast-tests/cros/remote/network/ping"
	"go.chromium.org/tast-tests/cros/services/cros/bluetooth"
	"go.chromium.org/tast-tests/cros/services/cros/cellular"
	"go.chromium.org/tast-tests/cros/services/cros/wifi"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/wlan"
)

// defaultRPCInvokeMethod is a default method to invoke commands sent by RPC.
var defaultRPCInvokeMethod wifi.InvokeMethodEnum = wifi.InvokeMethodEnum_SHILL_API

// dutData contains all information necessary to configure ChromeOS DUT (Chromebook/Chromebox).
type dutData struct {
	dut              *dut.DUT
	rpcHint          *testing.RPCHint
	rpc              *rpc.Client
	wifiClient       *WifiClient
	chipset          wlan.DeviceID
	bluetoothClient  bluetooth.BluetoothServiceClient
	cellularClient   cellular.RemoteCellularServiceClient
	originalLogLevel int
	originalLogTags  []string
	p2p              p2pDutData

	// netCertStore is initialized lazily in ConnectWifi() when needed because it takes about 7 seconds to set up
	// and only a few tests need it.
	netCertStore *netcertstore.Store
}

// p2pDutData contains P2P (WiFi Direct)-related data.
type p2pDutData struct {
	id         string
	ifName     string
	netID      int32
	ssid       string
	passphrase string
}

// The below is WiFiDevice interface's implementation:

// Conn returns pointer to SSH connection object.
func (dd *dutData) Conn() *ssh.Conn {
	return dd.dut.Conn()
}

// IfName returns interface name of a particular type (STA/AP/P2P)
func (dd *dutData) IfName(ctx context.Context, ifType IfaceType) (string, error) {
	switch ifType {
	case StaIfaceType:
		iface, err := dd.wifiClient.Interface(ctx)
		if err != nil {
			return "", errors.Wrap(err, "DUT: failed to get the client WiFi interface")
		}
		return iface, nil

	case P2PIfaceType:
		return dd.p2p.ifName, nil

	case APIfaceType:
		return shillconst.ApInterfaceName, nil

	default:
		return "", errors.Errorf("Interface type %v not implemented yet", ifType)
	}
}

// IPv4Addrs returns a slice of IPv4 Addresses configured on an interface of a particular type.
func (dd *dutData) IPv4Addrs(ctx context.Context, ifType IfaceType) ([]net.IP, error) {
	name, err := dd.IfName(ctx, ifType)
	if err != nil {
		return nil, err
	}
	req := &wifi.GetIPv4AddrsRequest{
		InterfaceName: name,
	}
	addrs, err := dd.wifiClient.GetIPv4Addrs(ctx, req)
	if err != nil {
		return nil, err
	}
	ret := make([]net.IP, len(addrs.Ipv4))
	for i, a := range addrs.Ipv4 {
		ret[i], _, err = net.ParseCIDR(a)
		if err != nil {
			return nil, errors.Wrapf(err, "failed to parse IP address %s", a)
		}
	}

	return ret, nil
}

// PingDevice pings another WiFiDevice on a particular type of interface. It automatically detects necessary addresses.
func (dd *dutData) PingDevice(ctx context.Context, dest WiFiDevice,
	srcIfType, dstIfType IfaceType, opts ...ping.Option) (*ping.Result, error) {
	srcName, err := dd.IfName(ctx, srcIfType)
	if err != nil {
		return nil, err
	}

	ip, err := dest.IPv4Addrs(ctx, dstIfType)
	if err != nil {
		return nil, err
	}
	opts = append(opts, ping.Interval(0.1), ping.BindAddress(true), ping.SourceIface(srcName))

	testing.ContextLogf(ctx, "Ping %s:%s -> %s", dd.dut.HostName(), srcName, ip[0])
	return dd.Ping(ctx, ip[0].String(), dstIfType, opts...)
}

// Ping sends pings from the current device to an abstract IP Address.
func (dd *dutData) Ping(ctx context.Context, addr string, ifType IfaceType, opts ...ping.Option) (*ping.Result, error) {
	pr := remoteping.NewRemoteRunner(dd.Conn())
	return pr.Ping(ctx, addr, opts...)
}

// The below is P2PWiFiDevice interface's implementation:

// P2PIfName returns P2P interface name of a particular
func (dd *dutData) P2PIfName() string {
	return dd.p2p.ifName
}

func (dd *dutData) P2PSSID() string {
	return dd.p2p.ssid
}

func (dd *dutData) P2PPassphrase() string {
	return dd.p2p.passphrase
}

// P2PGroupCreate creates WiFi Direct Group and takes its ownership.
func (dd *dutData) P2PGroupCreate(ctx context.Context, ops ...p2p.GroupOption) error {
	request := &wifi.P2PGroupCreateRequest{
		Method: defaultRPCInvokeMethod,
		Data: &wifi.P2PData{
			Freq: uint32(p2p.Freq(ops...)),
		},
	}
	ret, err := dd.wifiClient.P2PGroupCreate(ctx, request)
	if err != nil {
		return err
	}
	dd.p2p.id = ret.Id
	dd.p2p.ifName = ret.IfName
	dd.p2p.ssid = ret.Data.Ssid
	dd.p2p.passphrase = ret.Data.Key
	testing.ContextLogf(ctx, "P2P Group owner (GO): Configured in %vms",
		ret.ExecutionTime.AsDuration().Milliseconds())
	return err
}

// P2PGroupDelete deletes the existing WiFi Direct Group.
func (dd *dutData) P2PGroupDelete(ctx context.Context) error {
	request := &wifi.P2PGroupDeleteRequest{
		Id: dd.p2p.id,
	}
	ret, err := dd.wifiClient.P2PGroupDelete(ctx, request)
	if err != nil {
		return err
	}
	testing.ContextLogf(ctx, "P2P Group owner (GO): Deconfigured in %vms",
		ret.ExecutionTime.AsDuration().Milliseconds())
	dd.p2p.id = ""
	dd.p2p.ifName = ""
	return nil
}

// P2PGroupConnect handles connection to the existing WiFi Direct Group.
func (dd *dutData) P2PGroupConnect(ctx context.Context, device P2PWiFiDevice, ops ...p2p.GroupOption) error {
	request := &wifi.P2PGroupConnectRequest{
		Method: defaultRPCInvokeMethod,
		Data: &wifi.P2PData{
			Freq: uint32(p2p.Freq(ops...)),
			Ssid: device.P2PSSID(),
			Key:  device.P2PPassphrase(),
		},
	}

	ret, err := dd.wifiClient.P2PGroupConnect(ctx, request)
	if err != nil {
		return err
	}
	dd.p2p.id = ret.Id
	dd.p2p.ifName = ret.IfName
	dd.p2p.netID = ret.NetworkId
	testing.ContextLogf(ctx, "The p2p client connected to the p2p group owner (GO) network in %vms",
		ret.ExecutionTime.AsDuration().Milliseconds())
	return err
}

// P2PGroupDisconnect handles disconnection from the existing WiFi Direct Group.
func (dd *dutData) P2PGroupDisconnect(ctx context.Context) error {
	request := &wifi.P2PGroupDisconnectRequest{
		Id:        dd.p2p.id,
		NetworkId: dd.p2p.netID,
	}

	ret, err := dd.wifiClient.P2PGroupDisconnect(ctx, request)
	if err != nil {
		return err
	}
	testing.ContextLogf(ctx, "The p2p client disconnected from the p2p group owner (GO) network in %vms",
		ret.ExecutionTime.AsDuration().Milliseconds())
	dd.p2p.id = ""
	dd.p2p.ifName = ""
	dd.p2p.netID = -1
	return nil
}
