// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dhcp

import (
	"context"
	"net"

	"go.chromium.org/tast-tests/cros/local/network/virtualnet/env"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testFunc func(ctx context.Context) error

// RunTestWithEnv does the following steps:
//  1. start the DHCP test server with rules in env;
//  2. run testFunc to execute the test logic;
//  3. stop the DHCP test server and check the rules.
//
// Returns the server after the test which can be used to verify the
// interaction, e.g., get packets received by the server.
func RunTestWithEnv(ctx context.Context, env *env.Env, rules []HandlingRule, testFunc testFunc) (*testServer, []error) {
	listenAddr := net.IPv4(0, 0, 0, 0)
	broadcast := net.IPv4(255, 255, 255, 255)

	ec := make(chan error)

	s := newTestServer(env.VethInName, listenAddr, broadcast, serverPort)
	serverCtx, cancel := context.WithCancel(ctx)

	go func(ctx context.Context) {
		cleanup, err := env.EnterNetNS(ctx)
		if err != nil {
			ec <- errors.Wrapf(err, "failed to enter ns %s", env.NetNSName)
			return
		}
		defer cleanup()
		if err := s.setupAndBindSocket(ctx); err != nil {
			ec <- err
			return
		}
		defer s.listenConn.Close()
		defer s.sendConn.Close()
		testing.ContextLog(ctx, "Test DHCP server started")
		ec <- s.runLoop(ctx, rules)
		testing.ContextLog(ctx, "Test DHCP server stopped")
	}(serverCtx)

	// Run the test function at first, and then stop the server if it is still
	// running and get the result.
	var errs []error
	if err := testFunc(ctx); err != nil {
		errs = append(errs, err)
	}
	testing.ContextLog(ctx, "Verify rules for DHCP server")
	cancel()
	if err := <-ec; err != nil {
		errs = append(errs, errors.Wrap(err, "failed to verify DHCP rules in server"))
	}
	return s, errs
}

// OptionMapOpt is the function type to configure a OptionMap. OptionMap is of
// type map so this function type takes its value instead of pointer as the
// argument.
type OptionMapOpt func(OptionMap)

// WithOptionLeaseTime specifies the lease time in second (DHCP option 51).
func WithOptionLeaseTime(t uint32) OptionMapOpt {
	return func(m OptionMap) {
		m[ipLeaseTime] = t
	}
}

// WithOptionT1 specifies the renewal time in second (DHCP option 58).
func WithOptionT1(t uint32) OptionMapOpt {
	return func(m OptionMap) {
		m[renewalT1TimeValue] = t
	}
}

// WithOptionT2 specifies the rebinding time in second (DHCP option 59).
func WithOptionT2(t uint32) OptionMapOpt {
	return func(m OptionMap) {
		m[rebindingT2TimeValue] = t
	}
}

// NewOptionMap returns a workable OptionMap for DHCP. Lease time is 24 hours by
// default.
func NewOptionMap(gatewayIP, clientIP net.IP, opts ...OptionMapOpt) OptionMap {
	m := OptionMap{
		serverID:    gatewayIP.String(),
		subnetMask:  "255.255.255.0",
		ipLeaseTime: uint32(86400), // 86400 seconds
		requestedIP: clientIP.String(),
		routers:     []string{gatewayIP.String()},
	}
	for _, opt := range opts {
		opt(m)
	}
	return m
}

// NewFieldMap returns a FieldMap for DHCP.
func NewFieldMap(serverName string) FieldMap {
	return FieldMap{
		legacyServerName: serverName,
	}
}
