// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package capture

import (
	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
)

// Packet contains the different layers of a packet captured. Only the detected
// layers are set and multiple fields can be set at the same time.
type Packet struct {
	IPv4   *layers.IPv4
	IPv6   *layers.IPv6
	DHCPv4 *layers.DHCPv4
	TCP    *layers.TCP
	DNS    *layers.DNS
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

func parsePacket(p gopacket.Packet) *Packet {
	packet := &Packet{}

	if ip := p.Layer(layers.LayerTypeIPv4); ip != nil {
		packet.IPv4 = ip.(*layers.IPv4)
	}
	if ip := p.Layer(layers.LayerTypeIPv6); ip != nil {
		packet.IPv6 = ip.(*layers.IPv6)
	}
	if dhcp := p.Layer(layers.LayerTypeDHCPv4); dhcp != nil {
		packet.DHCPv4 = dhcp.(*layers.DHCPv4)
	}
	if tcp := p.Layer(layers.LayerTypeTCP); tcp != nil {
		packet.TCP = tcp.(*layers.TCP)
	}
	if dns := p.Layer(layers.LayerTypeDNS); dns != nil {
		packet.DNS = dns.(*layers.DNS)
	}

	return packet
}
