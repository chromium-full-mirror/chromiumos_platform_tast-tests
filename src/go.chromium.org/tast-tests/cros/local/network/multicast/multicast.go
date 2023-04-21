// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package multicast contains utilities for multicast related tests.
package multicast

import (
	"bufio"
	"bytes"
	"context"
	"net"
	"strconv"
	"strings"
	"time"

	"golang.org/x/net/ipv4"
	"golang.org/x/net/ipv6"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	patchpanel "go.chromium.org/tast-tests/cros/local/network/patchpanel_client"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	apk = "ArcMulticastForwarderTest.apk"
	pkg = "org.chromium.arc.testapp.multicast_forwarder"
	cls = "org.chromium.arc.testapp.multicast_forwarder.MulticastForwarderActivity"

	// MdnsButtonID is the ID for button to send mDNS packet.
	MdnsButtonID = "org.chromium.arc.testapp.multicast_forwarder:id/button_mdns"
	// SsdpButtonID is the ID for button to send SSDP packet.
	SsdpButtonID = "org.chromium.arc.testapp.multicast_forwarder:id/button_ssdp"
	// AcquireLockButtonID is the ID for button to acquire multicast lock.
	AcquireLockButtonID = "org.chromium.arc.testapp.multicast_forwarder:id/button_acquire"
	// DataID is the ID for text input box to set data.
	DataID = "org.chromium.arc.testapp.multicast_forwarder:id/data"
	// PortID is the ID for text input box to set port.
	PortID = "org.chromium.arc.testapp.multicast_forwarder:id/port"
	// Ipv6CheckboxID is the ID for check box to set whether this is IPv6 packet or not.
	Ipv6CheckboxID = "org.chromium.arc.testapp.multicast_forwarder:id/checkbox_ipv6"

	// MdnsHostnameIn is randomly generated inbound mDNS hostnames to verify if packet successfully forwarded.
	MdnsHostnameIn = "bff40af49dd97a3d1951f5af9c2b648099f15bbb.local"
	// MdnsHostnameInIPv6 is randomly generated inbound mDNS IPv6 hostnames to verify if packet successfully forwarded.
	MdnsHostnameInIPv6 = "da39a3ee5e6b4b0d3255bfef95601890afd80709.local"
	// LegacyMDNSHostnameIn is randomly generated inbound legacy mDNS hostnames to verify if packet successfully forwarded.
	LegacyMDNSHostnameIn = "a2ffec2cb85be5e7be43b2b0f4b7187379347e27.local"
	// LegacyMDNSHostnameInIPv6 is randomly generated inbound legacy mDNS IPv6 hostnames to verify if packet successfully forwarded.
	LegacyMDNSHostnameInIPv6 = "430c8b722af2d577a3d689ca6d82821385442907.local"
	// SsdpUserAgentIn is randomly generated inbound SSDP user agent to verify if packet successfully forwarded.
	SsdpUserAgentIn = "3d4a9db69bb32af3631825c556840656138cea3c"
	// SsdpUserAgentInIPv6 is randomly generated inbound SSDP IPv6 user agent to verify if packet successfully forwarded.
	SsdpUserAgentInIPv6 = "600c5082adf6270c25649bcdecf0584f203b7cb7"

	// MdnsHostnameOut is randomly generated outbound mDNS hostnames to verify if packet successfully forwarded.
	MdnsHostnameOut = "011f4b05bbe073f2ee322356159daa0a3ea5793f.local"
	// MdnsHostnameOutIPv6 is randomly generated outbound mDNS IPv6 hostnames to verify if packet successfully forwarded.
	MdnsHostnameOutIPv6 = "bed4eb698c6eeea7f1ddf5397d480d3f2c0fb938.local"
	// LegacyMDNSHostnameOut is randomly generated outbound legacy mDNS hostnames to verify if packet successfully forwarded.
	LegacyMDNSHostnameOut = "6c0596b8ac609191181a90517d51c0b486f23799.local"
	// LegacyMDNSHostnameOutIPv6 is randomly generated outbound legacy mDNS IPv6 hostnames to verify if packet successfully forwarded.
	LegacyMDNSHostnameOutIPv6 = "ce15802a8c5e8e9db0ffaf10130ef265296e9ea4.local"
	// SsdpUserAgentOut is randomly generated outbound SSDP user agent to verify if packet successfully forwarded.
	SsdpUserAgentOut = "e81a11db0ca4a137276eca2f189279f038219a23"
	// SsdpUserAgentOutIPv6 is randomly generated outbound SSDP IPv6 user agent to verify if packet successfully forwarded.
	SsdpUserAgentOutIPv6 = "d03754dadcd065da9063f9fb6c392e9f66880830"

	// MdnsPort is used as source ports to send mDNS packets.
	MdnsPort = 5353
	// LegacyMDNSPort is used as source ports to send legacy mDNS packets.
	LegacyMDNSPort = 10101
	// SsdpPort is used as source ports to send SSDP packets.
	SsdpPort = 9191

	// SsdpPrefix is used to search the correct SSDP packets from tcpdump.
	SsdpPrefix = "USER-AGENT: "
	// MdnsPrefix is used to search the correct mDNS packets from tcpdump.
	MdnsPrefix = "(QM)? "

	waitForPacketTimeout = 30 * time.Second
)

// SendMDNS creates an mDNS question query for hostname with a socket bound to port and ifname.
// This will call sendMulticast which intentionally set a flag to loopback the multicast packet.
// dstAddr is destination address of sending the query.
func SendMDNS(ctx context.Context, hostname, ifname string, port int, dstAddr *net.UDPAddr) error {
	// Craft mDNS message.
	b := bytes.NewBuffer(nil)
	// Add header to buffer.
	b.Write([]byte{0x0, 0x0})           // Transaction ID = 0
	b.Write([]byte{0x0, 0x0})           // Flags = 0
	b.Write([]byte{0x0, 0x1})           // Number of questions = 1
	b.Write([]byte{0x0, 0x0})           // Number of answers = 1
	b.Write([]byte{0x0, 0x0, 0x0, 0x0}) // Number of resource records = 0
	// Add hostname to buffer
	for _, data := range strings.Split(hostname, ".") {
		b.WriteByte(uint8(len(data)))
		b.Write([]byte(data))
	}
	b.WriteByte(0x0)          // Terminator
	b.Write([]byte{0x0, 0x1}) // QTYPE = A record
	b.Write([]byte{0x0, 0x1}) // QTYPE = IN class

	srcAddr := &net.UDPAddr{IP: dstAddr.IP, Port: port}

	if err := sendMulticast(ctx, b.Bytes(), srcAddr, dstAddr, ifname); err != nil {
		return err
	}
	return nil
}

// SendSSDP creates an SSDP search query with USER-AGENT ua with a socket bound to port and ifname.
// This will call sendMulticast which intentionally set a flag to loopback the multicast packet.
func SendSSDP(ctx context.Context, ua, ifname string, port int, dstAddr *net.UDPAddr) error {
	// Craft SSDP message.
	b := bytes.NewBuffer(nil)
	b.Write([]byte("M-SEARCH * HTTP/1.1\r\n"))
	b.Write([]byte("HOST: 239.255.255.250:1900\r\n"))
	b.Write([]byte("MAN: \"ssdp:discover\"\r\n"))
	b.Write([]byte("MX: 3\r\n"))
	b.Write([]byte("ST: ssdp:all\r\n"))
	b.Write([]byte("USER-AGENT: " + ua + "\r\n\r\n"))

	srcAddr := &net.UDPAddr{IP: dstAddr.IP, Port: port}

	if err := sendMulticast(ctx, b.Bytes(), srcAddr, dstAddr, ifname); err != nil {
		return err
	}
	return nil
}

// sendMulticast take data b and send it using a temporarily created socket bound to |srcAddr.port| and |ifname|.
// This function intentionally set multicast loopback to true.
func sendMulticast(ctx context.Context, b []byte, srcAddr, dstAddr *net.UDPAddr, ifname string) error {
	ifi, err := net.InterfaceByName(ifname)
	if err != nil {
		return errors.Wrap(err, "failed to get interface by name")
	}

	c, err := net.ListenMulticastUDP("udp", ifi, srcAddr)
	if err != nil {
		return errors.Wrap(err, "failed to open sending multicast socket")
	}
	defer c.Close()

	if dstAddr.IP.To4() != nil {
		pc := ipv4.NewPacketConn(c)
		if err := pc.SetMulticastLoopback(true); err != nil {
			return errors.Wrap(err, "failed to set multicast loopback")
		}
	} else {
		pc := ipv6.NewPacketConn(c)
		if err := pc.SetMulticastLoopback(true); err != nil {
			return errors.Wrap(err, "failed to set multicast loopback")
		}
	}

	//TODO(b/281926451): Secure this blocking call with context.
	if _, err := c.WriteTo(b, dstAddr); err != nil {
		return testing.PollBreak(errors.Wrap(err, "failed to send data"))
	}
	return nil
}

// SetTextsAndClick edits multicast sender parameter by setting EditTexts' text.
// This function shouldn't be called concurrently as it is closes over |d|.
func SetTextsAndClick(ctx context.Context, d *ui.Device, hostname, buttonID string, port int) error {
	if err := d.Object(ui.ID(DataID)).SetText(ctx, hostname); err != nil {
		return errors.Wrap(err, "failed setting hostname")
	}
	if err := d.Object(ui.ID(DataID), ui.Focused(true)).WaitForExists(ctx, 30*time.Second); err != nil {
		return errors.Wrap(err, "failed to focus on field "+DataID)
	}

	if err := d.Object(ui.ID(PortID)).SetText(ctx, strconv.Itoa(port)); err != nil {
		return errors.Wrap(err, "failed setting port")
	}
	if err := d.Object(ui.ID(PortID), ui.Focused(true)).WaitForExists(ctx, 30*time.Second); err != nil {
		return errors.Wrap(err, "failed to focus on field "+PortID)
	}
	if err := d.Object(ui.ID(buttonID)).Click(ctx); err != nil {
		return errors.Wrap(err, "failed to press button")
	}
	return nil
}

// InstallAndStartTestApp installs and start test app ArcMulticastForwarderTest.apk.
func InstallAndStartTestApp(ctx context.Context, d *ui.Device, a *arc.ARC) error {
	testing.ContextLog(ctx, "Installing app")
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		return errors.Wrap(err, "failed to install the APK")
	}

	testing.ContextLog(ctx, "Starting app")
	if err := a.Command(ctx, "am", "start", "-W", pkg+"/"+cls).Run(); err != nil {
		return errors.Wrap(err, "failed starting app")
	}

	if err := d.Object(ui.ID(MdnsButtonID)).WaitForExists(ctx, 30*time.Second); err != nil {
		return errors.Wrap(err, "failed to start app")
	}
	return nil
}

// SetIPv6Enabled checks and unchecks IPv6 check box in test app based on argument |b|.
func SetIPv6Enabled(ctx context.Context, d *ui.Device, b bool) error {
	if c, err := d.Object(ui.ID(Ipv6CheckboxID)).IsChecked(ctx); err != nil {
		return errors.Wrap(err, "failed to get IPv6 checkbox status")
	} else if c == b {
		return nil
	}
	if err := d.Object(ui.ID(Ipv6CheckboxID)).Click(ctx); err != nil {
		return errors.Wrap(err, "failed to toggle IPv6 checkbox")
	}
	return nil
}

// PhysicalInterfaces lists all available physical interfaces.
func PhysicalInterfaces(ctx context.Context) ([]string, error) {
	pc, err := patchpanel.New(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create patchpanel client")
	}

	response, err := pc.GetDevices(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get patchpanel devices")
	}

	var ifnames []string
	for _, device := range response.Devices {
		ifnames = append(ifnames, device.PhysIfname)
	}
	return ifnames, nil
}

// SupportedInterfaces gets all physical interfaces and checks the physical interfaces for multicast
// support. This is done by checking multicast flag followed by IPv4 existence.
// We don't check for IPv6 as kernel always provision an EUI 64 derived like local address in the fe80::/64 prefix.
func SupportedInterfaces(ctx context.Context) ([]string, error) {
	allIfnames, err := PhysicalInterfaces(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get physical interfaces")
	}
	var ifnames []string
	for _, ifname := range allIfnames {
		iface, err := net.InterfaceByName(ifname)
		if err != nil {
			return nil, errors.Wrap(err, "failed to get interface by name")
		}

		if iface.Flags&net.FlagMulticast == 0 {
			continue
		}

		addrs, err := iface.Addrs()
		if err != nil {
			return nil, errors.Wrap(err, "failed to get interface addresses")
		}

		for _, addr := range addrs {
			ip, _, err := net.ParseCIDR(addr.String())
			if err != nil {
				return nil, errors.Wrap(err, "failed to parse interface CIDR")
			}
			if ip.To4() != nil {
				ifnames = append(ifnames, ifname)
				break
			}
		}
	}
	return ifnames, nil
}

// StreamCmd takes a command cmd and stream its output. It search its output for every key in map
// |m|. This function will return an error that logs values of remaining key-value pairs if any key
// in |m| is not found before context is finished.
func StreamCmd(ctx context.Context, cmdList []string, m map[string]string) error {
	cmd := testexec.CommandContext(ctx, cmdList[0], cmdList[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start command")
	}

	// Signal channel used to notify error of exiting on context time out.
	exitChan := make(chan error, 1)
	// Signal channel used to terminate goroutine for cmd if unneeded anymore.
	completedChan := make(chan int, 1)
	go func(ctx context.Context) {
		var err error
		defer func() {
			exitChan <- err
			close(exitChan)
		}()
		select {
		case <-ctx.Done():
			err = ctx.Err()
		case <-completedChan:
		}
		// sc.Scan() below might block. Release bufio.Scanner by killing command if the
		// process execution time exceeds context deadline.
		cmd.Kill()
		cmd.Wait()
	}(ctx)

	// Copy expectation set for checking.
	expect := make(map[string]string)
	for k, v := range m {
		expect[k] = v
	}

	// Watch and wait until command have the expected outputs.
	sc := bufio.NewScanner(stdout)
	for {
		if !sc.Scan() {
			break
		}

		t := sc.Text()
		for a := range expect {
			if strings.Contains(t, a) {
				delete(expect, a)
			}
		}

		if len(expect) == 0 {
			break
		}
	}

	// Signal the goroutine that it's being completed and stops goroutine.
	close(completedChan)

	// Above goroutine exited when context is time out.
	if err := <-exitChan; err != nil {
		return errors.Wrap(err, "failed to scan for streaming output")
	}
	return nil
}

// StreamCmdExpectNotFound takes a command cmd and stream its output. It search its output for
// every key in map |m|. This function will return an error if any key in |s| is found within
// time window of waitForPacketTimeout.
func StreamCmdExpectNotFound(ctx context.Context, cmdList []string, m map[string]string) error {
	cmd := testexec.CommandContext(ctx, cmdList[0], cmdList[1:]...)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return err
	}

	if err := cmd.Start(); err != nil {
		return errors.Wrap(err, "failed to start command")
	}

	// Signal channel used to notify error of exiting on context time out.
	exitChan := make(chan error, 1)
	// Signal channel used to terminate goroutine for cmd if unneeded anymore.
	errChan := make(chan error, 1)
	// sc.Scan() below might block. Release bufio.Scanner by killing command if the
	// process execution time exceeds waitForPacketTimeout.
	go func(ctx context.Context) {
		var err error
		defer func() {
			exitChan <- err
			close(exitChan)
			close(errChan)
		}()
		select {
		// If we don't get error within waitForPacketTimeout, pass.
		case <-ctx.Done():
			err = ctx.Err()
		case err = <-errChan:
		case <-time.After(waitForPacketTimeout):
			err = nil
		}
		// sc.Scan() below might block. Release bufio.Scanner by killing command if the
		// process execution time exceeds context deadline.
		cmd.Kill()
		cmd.Wait()
	}(ctx)

	// Watch and wait until command have the expected outputs.
	sc := bufio.NewScanner(stdout)

scanloop:
	for {
		if !sc.Scan() {
			break
		}

		t := sc.Text()
		for a, name := range m {
			if strings.Contains(t, a) {
				// Signal the goroutine that it's being completed and stops goroutine.
				errChan <- errors.Wrap(err, "not expected but output line contains: "+name)
				break scanloop
			}
		}
	}

	if err := <-exitChan; err != nil {
		return errors.Wrap(err, "failed to scan for streaming output")
	}
	return nil
}
