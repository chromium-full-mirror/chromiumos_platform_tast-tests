// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package l4server provides a simple TCP/UDP server running inside a virtualnet.Env.
package l4server

import (
	"context"
	"fmt"
	"io"
	"net"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Option is the function type to configure an l4server.
type Option = func(*server)

const defaultBufSize = 4096

// WithBufSize sets the internal receive buffer size (in bytes) of the server.
// For UDP servers, this value must be no smaller than the maximum packet size
// to avoid the remaining part being discarded. The default value is 4096, which
// should be a good enough size in most cases.
func WithBufSize(size int) Option {
	return func(s *server) {
		s.bufSize = size
	}
}

// WithAddr configures the listening address of the server. INADDR_ANY will be
// used by default.
func WithAddr(addr string) Option {
	return func(s *server) {
		s.addr = addr
	}
}

// WithMsgHandler controls the behavior when the server receives any data. The
// server will do nothing by default.
func WithMsgHandler(handler MsgHandler) Option {
	return func(s *server) {
		s.handler = handler
	}
}

// New creates a new TCP/UDP server. The returned object should be passed to
// Env.StartServer; its lifetime will be managed by the environment. Note that
// the TCP server can only accept one connection at most now.
func New(fam Family, port int, opts ...Option) *server {
	s := &server{
		fam:     fam,
		port:    port,
		bufSize: defaultBufSize,
	}
	for _, opt := range opts {
		opt(s)
	}
	return s
}

// MsgHandler defines a function for processing incoming messages and emitting replies.
// If this function returns nil, no reply will be sent.
type MsgHandler func(in []byte) []byte

// Reflector is a type of MsgHandler that simply replies to any message with the same.
func Reflector() MsgHandler {
	return func(in []byte) []byte {
		out := make([]byte, len(in))
		copy(out, in)
		return out
	}
}

// Family describes the IP family for the server.
type Family int

func (f Family) String() string {
	return [...]string{"tcp", "tcp4", "tcp6", "udp", "udp4", "udp6"}[f]
}

const (
	// TCP corresponds to "tcp" as documented in the net package.
	TCP Family = iota
	// TCP4 corresponds to "tcp4" as documented in the net package.
	TCP4
	// TCP6 corresponds to "tcp6" as documented in the net package.
	TCP6
	// UDP corresponds to "udp" as documented in the net package.
	UDP
	// UDP4 corresponds to "udp4" as documented in the net package.
	UDP4
	// UDP6 corresponds to "udp6" as documented in the net package.
	UDP6
)

type closeHandler interface {
	Close() error
}

type server struct {
	fam     Family
	port    int
	bufSize int
	conns   []closeHandler
	handler MsgHandler
	addr    string
	run     bool
}

func (s *server) String() string {
	return fmt.Sprintf("%s:%s", s.fam, s.addrPort())
}

func (s *server) addrPort() string {
	switch s.fam {
	case TCP4, UDP4:
		return fmt.Sprintf("%s:%d", s.addr, s.port)
	case TCP6, UDP6:
		return fmt.Sprintf("[%s]:%d", s.addr, s.port)
	default:
		return fmt.Sprintf(":%d", s.port)
	}
}

// Start enters the netns of the virtualnet environment, starts listening on the desired port,
// and runs the handling loop until Stop is called.
func (s *server) Start(ctx context.Context, env *env.Env) error {
	if s.run {
		return errors.Errorf("%s server already running", s)
	}
	if (s.fam == TCP || s.fam == UDP) && len(s.addr) > 0 {
		return errors.New("cannot specify binding addr without IP family")
	}
	s.run = true
	ec := make(chan error)

	go func() {
		cleanup, err := env.EnterNetNS(ctx)
		if err != nil {
			ec <- errors.Wrapf(err, "failed to enter ns %s", env.NetNSName)
			return
		}
		defer cleanup()

		switch s.fam {
		case TCP, TCP4, TCP6:
			s.handleTCP(ctx, ec)
		case UDP, UDP4, UDP6:
			s.handleUDP(ctx, ec)
		}
	}()

	if err := <-ec; err != nil {
		return err
	}
	testing.ContextLogf(ctx, "%s server running", s)
	return nil
}

func (s *server) handleTCP(ctx context.Context, ec chan error) {
	addr, err := net.ResolveTCPAddr(s.fam.String(), s.addrPort())
	if err != nil {
		ec <- errors.Wrapf(err, "failed to resolve %s addr", s)
		return
	}
	// bind() call may fail here, perhaps because the interface is still not
	// ready. Use Poll() to retry. See b/259179849#comment13.
	var listener *net.TCPListener
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		listener, err = net.ListenTCP(s.fam.String(), addr)
		return err
	}, &testing.PollOptions{Timeout: 2 * time.Second}); err != nil {
		ec <- errors.Wrapf(err, "failed to listen on %s network", s)
		return
	}
	s.conns = append(s.conns, listener)
	ec <- nil

	var conn *net.TCPConn
	in := make([]byte, s.bufSize)

	// Only accept one connection at the same time.
	for {
		if !s.run {
			return
		}

		if conn == nil {
			conn, err = listener.AcceptTCP()
			if err != nil {
				testing.ContextLogf(ctx, "Failed to accept on %s network", s)
				return
			}
		}

		n, err := conn.Read(in)
		if err != nil && err == io.EOF {
			// This connection has ended. Reset conn to accept the next one.
			conn = nil
			continue
		}
		if err != nil {
			if s.run {
				testing.ContextLogf(ctx, "%s read failed: %v", s, err)
			}
			continue
		}
		out := s.handler(in[:n])
		if out == nil {
			continue
		}
		if _, err := conn.Write(out); err != nil {
			if s.run {
				testing.ContextLogf(ctx, "%s write failed: %v", s, err)
			}
			continue
		}
	}
}

func (s *server) handleUDP(ctx context.Context, ec chan<- error) {
	addr, err := net.ResolveUDPAddr(s.fam.String(), s.addrPort())
	if err != nil {
		ec <- errors.Wrapf(err, "failed to resolve %s addr", s)
		return
	}
	// bind() call may fail here, perhaps because the interface is still not
	// ready. Use Poll() to retry. See b/259179849#comment13.
	var conn *net.UDPConn
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		conn, err = net.ListenUDP(s.fam.String(), addr)
		return err
	}, &testing.PollOptions{Timeout: 2 * time.Second}); err != nil {
		ec <- errors.Wrapf(err, "failed to listen on %s network", s)
		return
	}
	s.conns = append(s.conns, conn)
	ec <- nil

	in := make([]byte, s.bufSize)
	for {
		if !s.run {
			return
		}
		n, addr, err := conn.ReadFromUDP(in)
		if err != nil {
			if s.run {
				testing.ContextLogf(ctx, "%s read failed: %v", s, err)
			}
			continue
		}
		out := s.handler(in[:n])
		if out == nil {
			continue
		}
		if _, err := conn.WriteToUDP(out, addr); err != nil {
			if s.run {
				testing.ContextLogf(ctx, "%s write failed: %v", s, err)
			}
			continue
		}
	}
}

// Stops shuts down the handling loop.
func (s *server) Stop(ctx context.Context) error {
	s.run = false
	var lastErr error
	for _, c := range s.conns {
		if err := c.Close(); err != nil {
			lastErr = err
			testing.ContextLogf(ctx, "Failed to close %s server: %v", s, err)
		}
	}
	return lastErr
}

// WriteLogs is not implemented as this server generates no logs.
func (s *server) WriteLogs(_ context.Context, f *os.File) error {
	return nil
}
