// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package toyserver provides a toy VPN server implementation.
//
// Protocol:
//
// The client should initiate the connection by starting a TCP connection to the
// server. This TCP connection will be used and only be used to tunnel IP
// packets. Each IP packet will be sent to the peer by one message. Each message
// contains two part: the length of the packet in network order (header), and
// the packet itself (payload). The packet will be sent in plain text -- there
// is no encryption.
//
// Server implementation:
//
// This implementation opens a tun device in the given virtualnet environment
// and forwards the traffic between it and the incoming TCP connection. This
// server will only accept one TCP connection in each run. If a new client wants
// to connect to this server, the previous connection needs to be closed and the
// server needs to be restarted. On any failure or unexpected situation, the
// server will exit itself. Since the main purpose is to support ARC VPN test,
// only IPv4 is supported for overlay. The implementation also assumes the
// underlay is IPv4 for simplicity.
//
// Example usage:
//
// s := toyserver.New()
// defer s.TearDown()
// s.SetUp()
// go s.RunLoop()
package toyserver

import (
	"context"
	"encoding/binary"
	"fmt"
	"io"
	"net"
	"os"
	"strconv"
	"sync"
	"unsafe"

	"github.com/google/gopacket"
	"github.com/google/gopacket/layers"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/network/virtualnet"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"golang.org/x/sys/unix"
)

// Server represents a toy VPN server.
type Server struct {
	mtu      int
	tunFD    *os.File
	listener net.Listener
}

// New creates a toy VPN server. TearDown should be deferred after this function
// returned.
func New() *Server {
	return &Server{}
}

// SetUp does the following setups: 1) create a tun device in e, configure
// serverOverlayIPv4 on it and bring it up, and 2) listen to incoming tcp
// connection at `0.0.0.0:serverPort`.
func (s *Server) SetUp(ctx context.Context, e *virtualnet.Env, serverPort int, tunIfname, serverOverlayIPv4 string, mtu int) error {
	// We only need to enter netns in this function. After we get the fd for the
	// tun device and create the socket for TCP connection, we can go back to the
	// root netns.
	exitNetNS, err := e.EnterNetNS(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to enter netns")
	}
	defer exitNetNS()

	tunFD, err := openTunDevice(ctx, tunIfname)
	if err != nil {
		return errors.Wrap(err, "failed to open tun device")
	}
	s.tunFD = tunFD

	if err := configureTunDevice(ctx, tunIfname, serverOverlayIPv4, mtu); err != nil {
		return errors.Wrap(err, "failed to configure tun device")
	}

	listener, err := net.Listen("tcp", fmt.Sprintf("0.0.0.0:%d", serverPort))
	if err != nil {
		return errors.Wrapf(err, "failed to listen on %d for TCP connection", serverPort)
	}
	s.listener = listener
	s.mtu = mtu

	return nil
}

// TearDown stops the server if it's still running and releases the resources.
func (s *Server) TearDown(ctx context.Context) error {
	var errs []error
	if s.tunFD != nil {
		if err := s.tunFD.Close(); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to close tun device"))
		}
		s.tunFD = nil
	}
	if s.listener != nil {
		if err := s.listener.Close(); err != nil {
			errs = append(errs, errors.Wrap(err, "failed to close TCP listener"))
		}
		s.listener = nil
	}

	return errors.Join(errs...)
}

// RunLoop accepts the TCP connection and starts forwarding between the tun
// interface and the TCP connection. This function won't return until failure
// (or TearDown is called), so the caller may want to run this in a goroutine.
func (s *Server) RunLoop(ctx context.Context) {
	const tag = "ToyVPNServer"

	testing.ContextLogf(ctx, "%s: waiting for TCP connection", tag)
	conn, err := s.listener.Accept()
	if err != nil {
		testing.ContextLogf(ctx, "%s: failed to accept TCP connection: %v", tag, err)
		return
	}
	defer conn.Close()

	testing.ContextLogf(ctx, "%s: accepted TCP connection from %s", tag, conn.RemoteAddr().String())

	var wg sync.WaitGroup
	wg.Add(2)

	// Goroutine to read from TUN and write to TCP.
	go func() {
		defer func() {
			wg.Done()
			testing.ContextLogf(ctx, "%s: tun-to-tcp forwarder finished", tag)
		}()

		buf := make([]byte, s.mtu*2) // make a large enough buffer
		for {
			n, err := s.tunFD.Read(buf)
			if err != nil {
				testing.ContextLogf(ctx, "%s: failed to read from tun device: %v", tag, err)
				return
			}

			packet := buf[:n]
			header := make([]byte, 4)
			binary.BigEndian.PutUint32(header, uint32(len(packet)))
			if _, err := conn.Write(header); err != nil {
				testing.ContextLogf(ctx, "%s: failed to write header to tcp connection: %v", tag, err)
				return
			}
			if _, err := conn.Write(packet); err != nil {
				testing.ContextLogf(ctx, "%s: failed to write payload to tcp connection: %v", tag, err)
				return
			}
		}
	}()

	// Goroutine to read from TCP and write to TUN.
	go func() {
		defer func() {
			wg.Done()
			testing.ContextLogf(ctx, "%s: tcp-to-tun forwarder finished", tag)
		}()

		for {
			header := make([]byte, 4)
			if _, err := io.ReadFull(conn, header); err != nil {
				testing.ContextLogf(ctx, "%s: failed to read length from TCP connection: %v", tag, err)
				return
			}

			length := binary.BigEndian.Uint32(header)
			if length == 0 {
				testing.ContextLogf(ctx, "%s: got a packet with length of 0", tag)
				continue
			}

			payload := make([]byte, length)
			if _, err = io.ReadFull(conn, payload); err != nil {
				testing.ContextLogf(ctx, "%s: failed to read payload from TCP connection: %v", tag, err)
				return
			}

			p := gopacket.NewPacket(payload, layers.LayerTypeIPv4, gopacket.Default)
			if p.Layer(layers.LayerTypeIPv4) == nil {
				// Not a valid IPv4 packet. Skip it.
				continue
			}

			if _, err := s.tunFD.Write(payload); err != nil {
				testing.ContextLogf(ctx, "%s: failed to write to tun device: %v", tag, err)
				return
			}
		}
	}()

	wg.Wait()
}

func openTunDevice(ctx context.Context, name string) (*os.File, error) {
	fd, err := unix.Open("/dev/net/tun", os.O_RDWR, 0)
	if err != nil {
		return nil, errors.Wrap(err, "failed to open /dev/net/tun")
	}

	req, err := unix.NewIfreq(name)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create ifreq")
	}
	req.SetUint16(unix.IFF_TUN | unix.IFF_NO_PI)

	_, _, errno := unix.Syscall(unix.SYS_IOCTL, uintptr(fd), unix.TUNSETIFF, uintptr(unsafe.Pointer(req)))
	if errno != 0 {
		return nil, errors.Wrap(err, "failed to call ioctl with TUNSETIFF")
	}

	return os.NewFile(uintptr(fd), "/dev/net/tun"), nil
}

func configureTunDevice(ctx context.Context, name, ipv4Addr string, mtu int) error {
	// Install ip address.
	if err := testexec.CommandContext(ctx, "ip", "addr", "add", ipv4Addr+"/32", "dev", name).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to configure ip address")
	}

	// Disable IPv6.
	if err := testexec.CommandContext(ctx, "sysctl", "-w", "net.ipv6.conf."+name+".disable_ipv6=1").Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to disable IPv6")
	}

	// Bring the interface up.
	if err := testexec.CommandContext(ctx, "ip", "link", "set", "mtu", strconv.Itoa(mtu), "up", "dev", name).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrap(err, "failed to bring the interface up")
	}

	return nil
}
