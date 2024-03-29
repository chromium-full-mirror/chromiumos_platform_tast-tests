// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wificell

import (
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/network/ping"
	"go.chromium.org/tast-tests/cros/common/wifi/p2p"
	"go.chromium.org/tast-tests/cros/remote/wificell/hostapd"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

// androidDeviceData contains all information necessary to configure an Android Device.
type androidDeviceData struct {
	serialNumber string
	modelName    string
	labstation   labstationData
	p2p          p2pAndroidDeviceData
}

// labstastionData contains all information necessary to communicate with the Android devices.,
type labstationData struct {
	target string
	host   *ssh.Conn
}

// p2pAndroidDeviceData contains P2P (WiFi Direct)-related data.
type p2pAndroidDeviceData struct {
	ifName     string
	ssid       string
	passphrase string
	frequency  uint32
	iPv4       string
}

const (
	// P2PAndroidDevicePersistent is used for connecting to a group owner.
	P2PAndroidDevicePersistent = "true"
	// P2PAndroidDeviceNotPersistent is used for creating a group owner.
	P2PAndroidDeviceNotPersistent = "false"
)

var p2pAndroidDeviceNetworkSSID = hostapd.RandomSSID("DIRECT-")

// Type returns WiFiDeviceType, which in case of Android phone is AndroidDevice.
func (ad *androidDeviceData) Type() WiFiDeviceType {
	return AndroidDevice
}

// Conn returns pointer to SSH connection object.
func (ad *androidDeviceData) Conn() *ssh.Conn {
	return ad.labstation.host
}

// IfName returns interface name of a particular type (STA/AP/P2P)
func (ad *androidDeviceData) IfName(ctx context.Context, ifType IfaceType) (string, error) {
	if ifType == P2PIfaceType {
		return ad.P2PIfName(), nil
	}
	return "", errors.Errorf("interface type %v not supported", ifType)
}

// IPv4Addrs returns a slice of IPv4 Addresses configured on an interface of a particular type.
func (ad *androidDeviceData) IPv4Addrs(ctx context.Context, ifType IfaceType) ([]net.IP, error) {
	if ifType != P2PIfaceType {
		return nil, errors.Errorf("interface type %v not supported", ifType)
	}

	ret := make([]net.IP, 1)
	var err error

	ret[0], _, err = net.ParseCIDR(ad.p2p.iPv4)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to parse IP address %s", ad.p2p.iPv4)
	}

	return ret, nil
}

// PingDevice pings another WiFiDevice on a particular type of interface. It automatically detects necessary addresses.
func (ad *androidDeviceData) PingDevice(ctx context.Context, dest WiFiDevice, srcIfType, dstIfType IfaceType, opts ...ping.Option) (*ping.Result, error) {
	srcName, err := ad.IfName(ctx, srcIfType)
	if err != nil {
		return nil, err
	}

	ip, err := dest.IPv4Addrs(ctx, dstIfType)
	if err != nil {
		return nil, err
	}
	opts = append(opts, ping.Interval(0.1), ping.BindAddress(true), ping.SourceIface(srcName))

	testing.ContextLogf(ctx, "Ping %s:%s -> %s", ad.modelName, srcName, ip[0])
	return ad.Ping(ctx, ip[0].String(), dstIfType, opts...)
}

// Ping sends pings from the current device to an abstract IP Address.
func (ad *androidDeviceData) Ping(ctx context.Context, addr string, ifType IfaceType, opts ...ping.Option) (*ping.Result, error) {
	pingCmd, pingArgs, _, err := ping.Command(ctx, addr, opts...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create the ping command")
	}
	pingOutput, err := ad.labstation.host.CommandContext(ctx, "adb", append([]string{"-s", ad.serialNumber, "shell", pingCmd}, pingArgs...)...).Output(ssh.DumpLogOnError)

	// ping will return non-zero value when no reply received. It would
	// be convenient if the caller can distinguish the case from command
	// error. Always try to parse the output here.
	res, parseErr := ping.ParseOutput(string(pingOutput))
	if parseErr != nil {
		if err != nil {
			return nil, err
		}
		return nil, err
	}
	return res, nil
}

// P2PIfName returns P2P interface name of a particular
func (ad *androidDeviceData) P2PIfName() string {
	return ad.p2p.ifName
}

func (ad *androidDeviceData) P2PSSID() string {
	return ad.p2p.ssid
}

func (ad *androidDeviceData) P2PPassphrase() string {
	return ad.p2p.passphrase
}

func (ad *androidDeviceData) P2PFrequency() uint32 {
	// TODO(b/333957851): Check frequency actively each time when called to cover the case of a channel switch.
	return ad.p2p.frequency
}

// waitForP2PNetworkInfo waits for p2p network information and returns the p2p interface and IP address.
func (ad *androidDeviceData) waitForP2PNetworkInfo(ctx context.Context) (string, string, error) {
	var iface, ipAddr string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		// TODO(b/334194299): Use wifip2p commands instead of ip command to get the network information.
		ifaceOut, err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "ip", "route", "|", "grep", "p2p", "|", "awk", "'{print $3}'").Output(ssh.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to get the p2p interface name")
		}

		if len(ifaceOut) == 0 {
			return errors.New("failed to find the p2p IP interface")
		}

		ipAddrOut, err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "ip", "route", "|", "grep", "p2p", "|", "awk", "'{print $9}'").Output(ssh.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to get the p2p interface IP address")
		}

		if len(ipAddrOut) == 0 {
			return errors.New("failed to find the p2p IP address")
		}

		iface = strings.TrimSuffix(string(ifaceOut), "\n")
		ipAddr = strings.TrimSuffix(string(ipAddrOut), "\n") + "/24"
		testing.ContextLogf(ctx, "Android P2P GO Interface: %s", iface)
		testing.ContextLogf(ctx, "Android P2P GO IP Address: %s", ipAddr)

		return nil
	}, &testing.PollOptions{
		Timeout:  time.Second * 20,
		Interval: time.Millisecond * 500,
	}); err != nil {
		return "", "", err
	}
	return iface, ipAddr, nil
}

// P2PGroupCreate creates WiFi Direct Group and takes its ownership.
func (ad *androidDeviceData) P2PGroupCreate(ctx context.Context, ops ...p2p.GroupOption) error {
	if err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "cmd", "wifi", "force-country-code", "enabled", "US").Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to force country code to US")
	}

	if err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "cmd", "wifip2p", "init").Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to initialize wifip2p")
	}

	const hex = "1234567890ABCDEFabcdef"
	s := strings.Repeat(hex, 1+13/len(hex))
	p2pAndroidDevicePassphrase := s[:13]

	if err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "cmd", "wifip2p", "create-group-with-config", p2pAndroidDeviceNetworkSSID, p2pAndroidDevicePassphrase, strconv.Itoa(p2p.Freq(ops...)), P2PAndroidDeviceNotPersistent).Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to create group with parameters: Network name = %s, Passpharase = %s, Band of Frequency = %s, Persistent = %s", p2pAndroidDeviceNetworkSSID, p2pAndroidDevicePassphrase, strconv.Itoa(p2p.Freq(ops...)), P2PAndroidDeviceNotPersistent)
	}

	iface, ipAddr, err := ad.waitForP2PNetworkInfo(ctx)
	if err != nil {
		return err
	}

	ad.p2p.ifName = string(iface)
	ad.p2p.iPv4 = string(ipAddr)
	ad.p2p.ssid = p2pAndroidDeviceNetworkSSID
	ad.p2p.passphrase = p2pAndroidDevicePassphrase
	ad.p2p.frequency = uint32(p2p.Freq(ops...))

	testing.ContextLog(ctx, "P2P Group owner (GO): Configured")

	return nil
}

// P2PGroupDelete deletes the existing WiFi Direct Group.
func (ad *androidDeviceData) P2PGroupDelete(ctx context.Context) error {
	if err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "cmd", "wifip2p", "remove-group").Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to initialize wifip2p")
	}

	ad.p2p.ifName = ""
	ad.p2p.iPv4 = ""
	ad.p2p.ssid = ""
	ad.p2p.passphrase = ""
	ad.p2p.frequency = 0

	testing.ContextLog(ctx, "P2P Group owner (GO): Deconfigured")

	return nil
}

// P2PGroupConnect handles connection to the existing WiFi Direct Group.
func (ad *androidDeviceData) P2PGroupConnect(ctx context.Context, device P2PWiFiDevice) error {
	if err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "cmd", "wifip2p", "init").Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to initialize wifip2p")
	}

	if err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "cmd", "wifip2p", "connect-with-config", device.P2PSSID(), device.P2PPassphrase(), strconv.Itoa(int(device.P2PFrequency())), P2PAndroidDevicePersistent).Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to connect to p2p network with parameters: Network name = %s, Passpharase = %s, Band of Frequency = %d, Persistent = %s", device.P2PSSID(), device.P2PPassphrase(), device.P2PFrequency(), P2PAndroidDevicePersistent)
	}

	iface, ipAddr, err := ad.waitForP2PNetworkInfo(ctx)
	if err != nil {
		return err
	}

	ad.p2p.ifName = string(iface)
	ad.p2p.iPv4 = string(ipAddr)
	ad.p2p.ssid = device.P2PSSID()
	ad.p2p.passphrase = device.P2PPassphrase()
	ad.p2p.frequency = device.P2PFrequency()

	testing.ContextLog(ctx, "The p2p client connected to the p2p group owner (GO) network")

	return nil
}

// P2PGroupDisconnect handles disconnection from the existing WiFi Direct Group.
func (ad *androidDeviceData) P2PGroupDisconnect(ctx context.Context) error {
	if err := ad.labstation.host.CommandContext(ctx, "adb", "-s", ad.serialNumber, "shell", "cmd", "wifip2p", "remove-group").Run(ssh.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to disconnect the p2p client")
	}

	ad.p2p.ifName = ""
	ad.p2p.iPv4 = ""
	ad.p2p.ssid = ""
	ad.p2p.passphrase = ""
	ad.p2p.frequency = 0

	testing.ContextLog(ctx, "The p2p client disconnected from the p2p group owner (GO) network")

	return nil
}
