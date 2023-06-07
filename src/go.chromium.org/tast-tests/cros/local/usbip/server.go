// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"golang.org/x/exp/slices"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/usbdevice"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Server represents a USBIP server.
type Server struct {
	server  *BaseServer
	handler *Handler
}

// server is a singletone Server instance.
var server *Server

// NewServer creates and returns a new USBIP server.
func NewServer() *Server {
	if server != nil {
		return server
	}
	handler := NewHandler()
	server = &Server{
		server:  NewBaseServer(handler),
		handler: handler,
	}
	return server
}

// Start starts a server asynchronously.
func (s *Server) Start(ctx context.Context, onError func(err error)) error {
	network := "tcp"
	address := fmt.Sprintf(":%d", serverPort)
	return s.server.Start(ctx, network, address, onError)
}

// Stop synchornously stops the server.
func (s *Server) Stop(ctx context.Context) error {
	return s.server.Stop(ctx)
}

// DetachFn is a function to detach an attached emulated device.
type DetachFn func(context.Context) error

// AttachFn is a function to attach an emulated device.
// Returns a function to detach the attached device.
type AttachFn func(context.Context) (DetachFn, error)

// AddDevice adds a new emulated device to the USBIP server.
// Returns a function to attach the device.
func (s *Server) AddDevice(device usbdevice.Device) AttachFn {
	bus, _, _, _ := s.handler.AddDevice(device)
	attach := func(ctx context.Context) (DetachFn, error) {
		attachCmd := testexec.CommandContext(ctx, "usbip", "attach", "-r", "localhost", "-b", bus)
		if err := attachCmd.Run(); err != nil {
			return nil, errors.Wrap(err, "unable to attach a usbip client")
		}
		// "usbip attach" doesn't return information for detach. For that we have to parse the output of "usbip port".
		// Looking for the last entry containing expected bus and getting the mapped port. Sample input:
		// Imported USB devices
		// ====================
		// Port 00: <Port in Use> at Full Speed(12Mbps)
		// 	   unknown vendor : unknown product (18d1:0123)
		// 	   3-1 -> usbip://localhost:3240/1-1.0
		// 		   -> remote bus/dev 000/002
		var assignedPort string
		re := regexp.MustCompile("(?sm)^Port (\\d+).*?-> usbip:.*?(\\d+-\\d+\\.\\d+)$")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			portCmd := testexec.CommandContext(ctx, "usbip", "port")
			output, err := portCmd.CombinedOutput()
			if err != nil {
				return errors.Wrap(err, "unable to list usbip ports")
			}
			result := re.FindAllStringSubmatch(string(output), -1)
			idx := slices.IndexFunc(result, func(matches []string) bool { return len(matches) == 3 && matches[2] == bus })
			if idx == -1 {
				return errors.Errorf("unable to find attached device in usbip port response: %s", string(output))
			}
			assignedPort = result[idx][1]
			return nil
		}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 5 * time.Second}); err != nil {
			return nil, err
		}
		detach := func(ctx context.Context) error {
			cmd := testexec.CommandContext(ctx, "usbip", "detach", "-p", assignedPort)
			if err := cmd.Run(); err != nil {
				return errors.Wrapf(err, "unable to detach a usbip client for port %s", assignedPort)
			}
			return nil
		}
		return detach, nil
	}
	return attach
}

// RemoveDevices removes all the devices from the USBIP server.
func (s *Server) RemoveDevices() {
	s.handler.RemoveDevices()
}
