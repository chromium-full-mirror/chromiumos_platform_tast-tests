// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"context"
	"io"
	"net"
	"sync"

	"golang.org/x/sys/unix"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// BaseHandler represents a handler for a connection.
type BaseHandler interface {
	HandleConnection(*net.Conn) error
}

// BaseServer represents a server that runs asynchronously.
type BaseServer struct {
	listener net.Listener
	network  string
	address  string
	quit     chan interface{}
	wg       sync.WaitGroup
	handler  BaseHandler
}

// NewBaseServer creates and returns a new server.
func NewBaseServer(handler BaseHandler) *BaseServer {
	s := &BaseServer{
		handler: handler,
	}
	return s
}

func (s *BaseServer) isListening() bool {
	return s.listener != nil
}

// Start starts a server asynchronously.
func (s *BaseServer) Start(ctx context.Context, network, address string, onError func(err error)) error {
	if s.isListening() {
		if address != s.address || network != s.network {
			return errors.Errorf("server has already been started at (%s) %s, unable to start it for different network or address",
				s.network,
				s.address)

		}
		return nil
	}
	testing.ContextLogf(ctx, "Starting the %s server on %s", network, address)
	s.quit = make(chan interface{})
	var err error
	s.listener, err = net.Listen(network, address)
	if err != nil {
		return errors.Wrapf(err, "failed to listen on %v", address)
	}
	s.network = network
	s.address = address
	s.wg.Add(1)

	wrappedOnError := func(err error) {
		if errors.Is(err, io.EOF) || errors.Is(err, unix.EPIPE) {
			// EOF and EPIPE errors are thrown when a client drops the connection and we interact with it.
			testing.ContextLog(ctx, "A client has dropped its connection to the server")
			return
		}
		onError(err)
	}

	go s.serve(wrappedOnError)
	return nil
}

// Stop synchornously stops the server.
func (s *BaseServer) Stop(ctx context.Context) error {
	if !s.isListening() {
		return nil
	}
	testing.ContextLog(ctx, "Stopping the server")
	close(s.quit)
	if err := s.listener.Close(); err != nil {
		return errors.Wrap(err, "failed to stop the server")
	}
	s.wg.Wait()
	s.listener = nil
	return nil
}

// serve listens for incoming connections and sends them to the handler until the server is stopped.
func (s *BaseServer) serve(onError func(err error)) {
	defer s.wg.Done()
	for {
		conn, err := s.listener.Accept()
		if err != nil {
			select {
			case <-s.quit:
				return
			default:
				onError(errors.Wrap(err, "failed to accept the connection"))
			}
		} else {
			s.wg.Add(1)
			go func() {
				defer s.wg.Done()
				defer func() {
					if err := conn.Close(); err != nil {
						onError(errors.Wrap(err, "error closing the connection"))
					}
				}()
				if err := s.handler.HandleConnection(&conn); err != nil {
					onError(errors.Wrap(err, "error handling the connection"))
				}
			}()
		}
	}
}
