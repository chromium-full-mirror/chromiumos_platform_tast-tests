// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package usbip

import (
	"context"
	"net"
	"strings"
	"testing"
)

type TestHandler struct {
	input  string
	output string
}

func bufferToString(buffer []byte) string {
	return strings.Trim(string(buffer), "\x00")
}

func (h *TestHandler) HandleConnection(conn *net.Conn) error {
	buffer := make([]byte, 32)
	(*conn).Read(buffer)
	(*h).input = bufferToString(buffer)
	(*conn).Write([]byte((*h).output))
	return nil
}

const (
	network = "tcp"
	address = ":12345"
)

func TestServeAndStop(t *testing.T) {
	input := "Input message"
	h := &TestHandler{
		output: "Output message",
	}
	ctx := context.Background()
	s := NewBaseServer(h)
	s.Start(ctx, network, address, func(err error) {})
	defer func() { s.Stop(ctx) }()

	conn, _ := net.Dial(network, address)
	conn.Write([]byte(input))
	buffer := make([]byte, 32)
	conn.Read(buffer)
	conn.Close()

	output := bufferToString(buffer)
	if input != h.input {
		t.Errorf("Expected handler to receive %q; received: %q", input, h.input)
	}
	if output != h.output {
		t.Errorf("Expected to receive %q; received: %q", h.output, output)
	}
}

func TestStartStopMultipleTimes(t *testing.T) {
	h := &TestHandler{}
	ctx := context.Background()
	s := NewBaseServer(h)
	if err := s.Start(ctx, network, address, func(err error) {}); err != nil {
		t.Errorf("Unable to start server #1: %s", err)
	}
	defer func() {
		if err := s.Stop(ctx); err != nil {
			t.Errorf("Unable to stop server #1: %s", err)
		}
	}()
	if err := s.Start(ctx, network, address, func(err error) {}); err != nil {
		t.Errorf("Unable to start server #2: %s", err)
	}
	defer func() {
		if err := s.Stop(ctx); err != nil {
			t.Errorf("Unable to stop server #2: %s", err)
		}
	}()

	conn, err := net.Dial(network, address)
	if err != nil {
		t.Error("Unable to connect to the server")
	}
	if err := conn.Close(); err != nil {
		t.Error("Unable to disconnect from the server")
	}
}

func TestSecondStarFailsWithDifferentArgs(t *testing.T) {
	h := &TestHandler{}
	ctx := context.Background()
	s := NewBaseServer(h)
	s.Start(ctx, network, address, func(err error) {})
	defer func() { s.Stop(ctx) }()

	otherNetwork := "udp"
	otherAddress := ":11111"

	checkErrorPresence := func(err error) {
		expectedError := "server has already been started at"
		if err == nil {
			t.Error("Unable to find an error")
		}
		if !strings.Contains(err.Error(), expectedError) {
			t.Errorf("Unable to find expected error, want: %s; got: %s", expectedError, err.Error())
		}
	}
	checkErrorPresence(s.Start(ctx, otherNetwork, address, func(err error) {}))
	checkErrorPresence(s.Start(ctx, network, otherAddress, func(err error) {}))
	checkErrorPresence(s.Start(ctx, otherNetwork, otherAddress, func(err error) {}))
}
