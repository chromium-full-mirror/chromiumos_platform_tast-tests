// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package capture

import (
	"fmt"
	"strings"
	"time"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// Packet contains the different layers of a packet captured. Only the detected
// layers and payload are set and multiple fields can be set at the same time.
type Packet struct {
	Timestamp   time.Time
	Ethernet    *layers.Ethernet
	IPv4        *layers.IPv4
	IPv6        *layers.IPv6
	ICMPv4      *layers.ICMPv4
	ICMPv6      *layers.ICMPv6
	DHCPv4      *layers.DHCPv4
	TCP         *layers.TCP
	UDP         *layers.UDP
	DNS         *layers.DNS
	Application gopacket.ApplicationLayer
}

// DSCP returns the DSCP value present in the packet, 0 if no applicable.
func (p *Packet) DSCP() uint8 {
	if p.IPv4 != nil {
		return p.IPv4.TOS >> 2
	}
	if p.IPv6 != nil {
		return p.IPv6.TrafficClass >> 2
	}
	return 0
}

func (p *Packet) String() string {
	var lines []string
	lines = append(lines, fmt.Sprintf("Packet{timestamp=%v,", p.Timestamp))
	if p.Ethernet != nil {
		lines = append(lines, fmt.Sprintf("  Ethernet{src=%s, dst=%s},", p.Ethernet.SrcMAC, p.Ethernet.DstMAC))
	}
	if p.IPv4 != nil {
		lines = append(lines, fmt.Sprintf("  IPv4{src=%s, dst=%s, dscp=%d},", p.IPv4.SrcIP, p.IPv4.DstIP, p.DSCP()))
	}
	if p.IPv6 != nil {
		lines = append(lines, fmt.Sprintf("  IPv6{src=%s, dst=%s, dscp=%d},", p.IPv6.SrcIP, p.IPv6.DstIP, p.DSCP()))
	}
	if p.ICMPv4 != nil {
		lines = append(lines, fmt.Sprintf("  ICMPv4{type=%s},", p.ICMPv4.TypeCode))
	}
	if p.ICMPv6 != nil {
		lines = append(lines, fmt.Sprintf("  ICMPv6{type=%s},", p.ICMPv6.TypeCode))
	}
	if p.UDP != nil {
		lines = append(lines, fmt.Sprintf("  UDP{src=%d, dst=%d},", p.UDP.SrcPort, p.UDP.DstPort))
	}
	if p.TCP != nil {
		lines = append(lines, fmt.Sprintf("  TCP{src=%d, dst=%d},", p.TCP.SrcPort, p.TCP.DstPort))
	}
	lines = append(lines, "}")
	return strings.Join(lines, "\n")
}

func parsePacket(p gopacket.Packet) *Packet {
	packet := &Packet{}

	packet.Timestamp = p.Metadata().CaptureInfo.Timestamp

	if eth := p.Layer(layers.LayerTypeEthernet); eth != nil {
		packet.Ethernet = eth.(*layers.Ethernet)
	}
	if ip := p.Layer(layers.LayerTypeIPv4); ip != nil {
		packet.IPv4 = ip.(*layers.IPv4)
	}
	if ip := p.Layer(layers.LayerTypeIPv6); ip != nil {
		packet.IPv6 = ip.(*layers.IPv6)
	}
	if icmp := p.Layer(layers.LayerTypeICMPv4); icmp != nil {
		packet.ICMPv4 = icmp.(*layers.ICMPv4)
	}
	if icmp := p.Layer(layers.LayerTypeICMPv6); icmp != nil {
		packet.ICMPv6 = icmp.(*layers.ICMPv6)
	}
	if dhcp := p.Layer(layers.LayerTypeDHCPv4); dhcp != nil {
		packet.DHCPv4 = dhcp.(*layers.DHCPv4)
	}
	if tcp := p.Layer(layers.LayerTypeTCP); tcp != nil {
		packet.TCP = tcp.(*layers.TCP)
	}
	if udp := p.Layer(layers.LayerTypeUDP); udp != nil {
		packet.UDP = udp.(*layers.UDP)
	}
	if dns := p.Layer(layers.LayerTypeDNS); dns != nil {
		packet.DNS = dns.(*layers.DNS)
	}
	if app := p.ApplicationLayer(); app != nil {
		packet.Application = app
	}

	return packet
}
