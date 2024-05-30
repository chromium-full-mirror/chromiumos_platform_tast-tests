// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dhcp

import (
	"context"
	"fmt"
	"net"
	"syscall"
	"time"

	"golang.org/x/sync/errgroup"
	"golang.org/x/sys/unix"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// testServer is a simple DHCP server you can program with expectations of
// future packets and responses to those packets.  The server is basically a
// thin wrapper around a server socket with some utility logic to make setting
// up tests easier. To write a test, you start a server, construct a sequence of
// handling rules, and write a test function.
//
// Handling rules let you set up expectations of future packets of certain
// types. Handling rules are processed in order, and only the first remaining
// handler handles a given packet. In theory you could write the entire test
// into a single handling rule and keep an internal state machine for how far
// that handler has gotten through the test. This would be poor style however.
// Correct style is to write (or reuse) a handler for each packet the server
// should see, leading us to a happy land where any conceivable packet handler
// has already been written for us.
type testServer struct {
	iface     string
	inAddr    net.IP
	bcastAddr net.IP

	// The source port that the server uses to send out the DHCP packet. Note that
	// RFC 2131 makes no requirements or recommendations on this port.
	sendPort int

	listenConn *net.UDPConn
	sendConn   *net.UDPConn
	packets    []*dhcpPacket
}

type testFunction func(context.Context) error

// The standard DHCP ports.
const (
	ServerPort = 67
	ClientPort = 68
)

func newTestServer(iface string, inAddr, bcastAddr net.IP, sendPort int) *testServer {
	return &testServer{
		iface:     iface,
		inAddr:    inAddr,
		bcastAddr: bcastAddr,
		sendPort:  sendPort,
	}
}

// setupAndBindSocket creates, sets the appropriate socket options for, and
// binds to the server socket.
func (s *testServer) setupAndBindSocket(ctx context.Context) (retErr error) {
	controlFunc := func(network, address string, c syscall.RawConn) error {
		var err error
		if cerr := c.Control(func(fd uintptr) {
			if err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_REUSEADDR, 1); err != nil {
				return
			}
			if err = unix.SetsockoptInt(int(fd), unix.SOL_SOCKET, unix.SO_BROADCAST, 1); err != nil {
				return
			}
			if len(s.iface) > 0 {
				if err = unix.SetsockoptString(int(fd), unix.SOL_SOCKET, unix.SO_BINDTODEVICE, s.iface); err != nil {
					return
				}
			}
		}); cerr != nil {
			return cerr
		}
		return err
	}

	defer func() {
		if retErr != nil {
			s.cleanUp(ctx)
		}
	}()

	// Create the listen socket.
	lc := net.ListenConfig{Control: controlFunc}
	listenAddr := fmt.Sprintf("%s:%d", s.inAddr.String(), ServerPort)
	listenConn, err := lc.ListenPacket(ctx, "udp", listenAddr)
	if err != nil {
		listenConn.Close()
		return errors.Wrapf(err, "failed to listen on %s", listenAddr)
	}
	var ok bool
	s.listenConn, ok = listenConn.(*net.UDPConn)
	if !ok {
		return errors.New("incorrect socket type, expected UDP")
	}

	// Create the connect socket. We may use a different source port to send out
	// the UDP packet, so use a different UDP.conn object here. Note: In the case
	// that the two port are same, theoretically we should use SO_REUSEPORT to
	// create the sockets, but seems that the current code also works. We should
	// revisit this part if there is any problem in the future.
	dialer := net.Dialer{
		LocalAddr: &net.UDPAddr{IP: s.inAddr, Port: s.sendPort},
		Control:   controlFunc,
	}
	bcastAddr := fmt.Sprintf("%s:%d", s.bcastAddr, ClientPort)
	sendConn, err := dialer.Dial("udp", bcastAddr)
	if err != nil {
		return errors.Wrapf(err, "failed to connect to %s", bcastAddr)
	}
	s.sendConn, ok = sendConn.(*net.UDPConn)
	if !ok {
		return errors.New("incorrect socket type, expected UDP")
	}

	return nil
}

func (s *testServer) sendResponse(packet *dhcpPacket) error {
	if packet == nil {
		return errors.New("handling rule failed to return a packet")
	}
	binaryStr, err := packet.marshal()
	if err != nil {
		return errors.Wrap(err, "packet failed to serialize to binary string")
	}
	if err = s.sendConn.SetWriteDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
		return errors.Wrap(err, "unable to set deadline")
	}
	_, err = s.sendConn.Write([]byte(binaryStr))
	return err
}

// runLoop is the loop body of the test server. It receives and handles DHCP
// packets coming from the client and responds to them according to the given
// handling rules.
func (s *testServer) runLoop(ctx context.Context, rules []HandlingRule) error {
	buffer := make([]byte, 2048)
	for {
		if len(rules) < 1 {
			return errors.New("no handling rules left")
		}
		if err := ctx.Err(); err != nil {
			return err
		}
		if err := s.listenConn.SetReadDeadline(time.Now().Add(100 * time.Millisecond)); err != nil {
			return errors.Wrap(err, "unable to set deadline")
		}
		n, _, err := s.listenConn.ReadFromUDP(buffer)
		if opErr, ok := err.(*net.OpError); ok && opErr.Timeout() {
			continue
		} else if err != nil {
			return errors.Wrap(err, "read failed")
		} else if n == 0 {
			testing.ContextLog(ctx, "Read returned 0 bytes")
			continue
		}
		packet, err := newDHCPPacket(buffer[:n])
		if err != nil {
			testing.ContextLog(ctx, "Unable to create DHCP packet: ", err)
			continue
		}
		if err = packet.isValid(); err != nil {
			testing.ContextLog(ctx, "Invalid DHCP packet: ", err)
			continue
		}

		s.packets = append(s.packets, packet)

		rule := rules[0]
		code := rule.handle(packet)

		testing.ContextLogf(ctx, "Test DHCP server received a DHCP packet %s, action %s", packet, code)

		if code&popHandler != 0 {
			rules = rules[1:]
		}

		if code&haveResponse != 0 {
			for i := 0; i < rule.respPktCnt; i++ {
				response, err := rule.respond(packet)
				if err != nil {
					return errors.Wrap(err, "failed to generate response")
				}
				if err = s.sendResponse(response); err != nil {
					return errors.Wrap(err, "failed to send packet")
				}
			}
		}

		if code&testFailed > 0 {
			return errors.New("handling rule rejected packet")
		}

		if code&testSucceeded > 0 {
			return nil
		}
	}
}

func (s *testServer) cleanUp(ctx context.Context) {
	if s.listenConn != nil {
		if err := s.listenConn.Close(); err != nil {
			testing.ContextLog(ctx, "Failed to close listen socket: ", err)
		}
	}
	if s.sendConn != nil {
		if err := s.sendConn.Close(); err != nil {
			testing.ContextLog(ctx, "Failed to close send socket: ", err)
		}
	}
}

// runTest runs testFunc against a server with the given handling rules.
func (s *testServer) runTest(ctx context.Context, rules []HandlingRule, testFunc testFunction) error {
	if err := s.setupAndBindSocket(ctx); err != nil {
		return err
	}
	defer s.cleanUp(ctx)
	g, ctx := errgroup.WithContext(ctx)
	g.Go(func() error {
		return s.runLoop(ctx, rules)
	})
	g.Go(func() error {
		return testFunc(ctx)
	})
	return g.Wait()
}

// Packets returns all the DHCP packets received by the server.
func (s *testServer) Packets() []*dhcpPacket {
	return s.packets
}
