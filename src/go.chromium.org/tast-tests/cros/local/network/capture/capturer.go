// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package capture

import (
	"context"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"github.com/google/gopacket/pcapgo"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Capturer is a helper to capture packets on a given interface using pcapgo.
type Capturer struct {
	iface   string
	packets chan *Packet
	cancel  context.CancelFunc
}

// NewCapturer creates a capturer instance on the provided interface.
func NewCapturer(iface string) *Capturer {
	return &Capturer{
		iface:   iface,
		packets: make(chan *Packet),
	}
}

// Start launches the packet capture on the capturer interface.
func (c *Capturer) Start(ctx context.Context) error {
	if c.cancel != nil {
		return errors.Errorf("capturer already started on %s", c.iface)
	}
	cancelableCtx, cancel := context.WithCancel(ctx)
	c.cancel = cancel
	go c.capture(cancelableCtx)
	return nil
}

// Stop ends the capture on the interface by cancelling the go routine context.
// This is an asynchronous method.
func (c *Capturer) Stop() {
	if c.cancel != nil {
		c.cancel()
	}
}

// Packets returns the channel that delivers the packets captured and parsed.
func (c *Capturer) Packets() chan *Packet {
	return c.packets
}

func (c *Capturer) capture(ctx context.Context) {
	h, err := pcapgo.NewEthernetHandle(c.iface)
	if err != nil {
		testing.ContextLogf(ctx, "Failed to create capture handle on %s: %v", c.iface, err)
		c.cancel = nil
		return
	}
	defer h.Close()

	// Close the packet channel on return.
	defer close(c.packets)

	src := gopacket.NewPacketSource(h, layers.LayerTypeEthernet)
	for {
		select {
		case p := <-src.Packets():
			c.packets <- parsePacket(p)
		case <-ctx.Done():
			return
		}
	}
}
