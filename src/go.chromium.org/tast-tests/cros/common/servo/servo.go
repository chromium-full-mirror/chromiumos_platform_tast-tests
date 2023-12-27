// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package servo is used to communicate with servo devices connected to DUTs.
// It communicates with servod over XML-RPC.
// More details on servo: https://www.chromium.org/chromium-os/servo
//
// Caution: If you reboot the ChromeOS EC:
// - If using a CCD servo, you should call RemoveCCDWatchdogs(ctx) or servod will fail.
// - Use Helper.WaitConnect instead of DUT.WaitConnect.
package servo

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/xmlrpc"

	"go.chromium.org/tast/core/errors"
)

// Servo holds the servod connection information.
type Servo struct {
	xmlrpc *xmlrpc.XMLRpc

	// Cache queried attributes that won't change.
	version            string
	dutConnType        DUTConnTypeValue
	dutCCDController   DUTController
	dutDebugController DUTController
	servoType          string
	hasCCD             bool
	hasServoMicro      bool
	hasC2D2            bool
	isDualV4           bool
	isPDTester         bool
	// TODO(b/194310192): Unify the servo UART regexp controls in servod.
	uartRegexp StringControl

	// For PD tests, this caches the information about the PD port on the DUT.
	dutPDInfo *DUTPDInfo
}

const (
	// servodDefaultHost is the default host for servod.
	servodDefaultHost = "localhost"
	// servodDefaultPort is the default port for servod.
	servodDefaultPort = 9999
)

// New creates a new Servo object for communicating with a servod instance.
// connSpec holds servod's location, either as "host:port" or just "host"
// (to use the default port).
func New(ctx context.Context, host string, port int) (*Servo, error) {
	s := &Servo{xmlrpc: xmlrpc.New(host, port)}

	// Ensure Servo is set up properly before returning.
	return s, s.verifyConnectivity(ctx)
}

// Default creates a Servo object for communicating with a local servod
// instance using the default port.
func Default(ctx context.Context) (*Servo, error) {
	return New(ctx, servodDefaultHost, servodDefaultPort)
}

// NewDirect returns a servo object for communication with a servod instance running at spec.
// During local tests, instead of establishing a ssh proxy with servo host, creating a direct
// connection is preferred. Param spec is either "host:port" or just "host" to use default port.
// Please make sure "host" is the address reachable from DUT.
func NewDirect(ctx context.Context, spec string) (*Servo, error) {
	info, err := splitHostPort(spec)
	if err != nil {
		return nil, err
	}
	return New(ctx, info.Hostname, info.ServoPort)
}

func (s *Servo) reconnect(ctx context.Context, host string, port int) error {
	s.xmlrpc = xmlrpc.New(host, port)
	return s.verifyConnectivity(ctx)
}

// verifyConnectivity sends and verifies an echo request to make sure
// everything is set up properly.
func (s *Servo) verifyConnectivity(ctx context.Context) error {
	const msg = "hello from servo"
	actualMessage, err := s.Echo(ctx, "hello from servo")
	if err != nil {
		return err
	}

	const expectedMessage = "ECH0ING: " + msg
	if actualMessage != expectedMessage {
		return errors.Errorf("echo verification request returned %q; expected %q", actualMessage, expectedMessage)
	}

	return nil
}

// Close performs Servo cleanup.
func (s *Servo) Close(ctx context.Context) error {
	return nil
}
