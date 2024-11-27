// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package amber implements the Controller interface for Amber Robotics Arm.
package amber

import (
	"bytes"
	"encoding/binary"
	"net"
	"time"

	"go.chromium.org/tast-tests/cros/common/robotics/arm"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var armIPVar = testing.RegisterVarString(
	"amber.ip",
	"",
	"The IP of the control box of the Amber Robotics Arm",
)

const (
	defaultMoveDuration = 5 * time.Second
	// defaultGripperDuration is the default wait time for a gripper operation to complete.
	defaultGripperDuration = 3 * time.Second
	// DefaultReadWriteTimeout is the default timeout for connection read / write.
	DefaultReadWriteTimeout = 10 * time.Second
	// responseSuccess indicates a successful arm operation.
	responseSuccess uint8 = 1
)

// Arm implements the Controller interface for Amber Robotics Arm.
type Arm struct {
	conn             net.Conn
	counter          uint32
	readWriteTimeout time.Duration
}

var _ arm.Controller = (*Arm)(nil)

// NewArm returns an Arm object associated with a connection to the control box.
// |timeout| is the timeout for the connection read/write actions.
func NewArm(timeout time.Duration) (*Arm, error) {
	addr := armIPVar.Value()
	if addr == "" {
		return nil, errors.Errorf("the arm IP is not set with %q runtime variable", armIPVar.Name())
	}
	conn, err := net.Dial("udp", addr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to dial to the UDP server")
	}
	return &Arm{
		conn:             conn,
		readWriteTimeout: timeout,
	}, nil
}

// Close closes the connection associates with the object.
func (a *Arm) Close() error {
	return a.conn.Close()
}

// getNextCounter returns the current counter and increases the counter by 1.
// The counter field is used in the communication between robotic arm to ensure the
// response is corresponding to the request. Increment the counter each time to ensure
// the next request will have a different counter.
func (a *Arm) getNextCounter() uint32 {
	counter := a.counter
	a.counter++
	return counter
}

// sendRequest parses the request into bytes and sends to the robotics arm.
func (a *Arm) sendRequest(req []*armData) error {
	// Parse the request into bytes.
	buf := new(bytes.Buffer)
	for _, data := range req {
		binary.Write(buf, binary.LittleEndian, data.value)
	}

	if err := a.conn.SetWriteDeadline(time.Now().Add(a.readWriteTimeout)); err != nil {
		return errors.Wrap(err, "failed to set connection write deadline")
	}
	_, err := a.conn.Write(buf.Bytes())
	return err
}

// readResponse reads the response with the given message length from the robotic arm.
func (a *Arm) readResponse(resLength int, res []*armData) error {
	resBytes := make([]byte, resLength)
	if err := a.conn.SetReadDeadline(time.Now().Add(a.readWriteTimeout)); err != nil {
		return errors.Wrap(err, "failed to set read deadline")
	}
	if _, err := a.conn.Read(resBytes); err != nil {
		return errors.Wrap(err, "failed to collect response")
	}
	if err := parseResponseFromBytes(resBytes, res); err != nil {
		return errors.Wrap(err, "failed to parse response from bytes")
	}
	return nil
}

// parseResponseFromBytes parses the data from responded bytes into |res|.
func parseResponseFromBytes(bytes []byte, res []*armData) error {
	var start int
	for _, data := range res {
		lastEnd, err := data.readFromBytes(bytes, start)
		if err != nil {
			return errors.Wrapf(err, "failed to read data %s", data.name)
		}
		// Update the start point for the next data.
		start = lastEnd
	}
	return nil
}
