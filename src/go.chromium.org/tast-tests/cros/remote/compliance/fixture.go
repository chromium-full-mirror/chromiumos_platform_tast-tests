// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package compliance

import (
	"context"
	"fmt"
	"net"
	"strings"
	"time"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Timeout for methods of Tast fixture.
const (
	complianceUser  = "crosecminion"
	setUpTimeout    = 6 * time.Minute
	tearDownTimeout = 3 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "complianceHostFixture",
		Desc:            "Fixture for communicating with compliance measurement devices",
		Contacts:        []string{},
		Impl:            &TestFixture{},
		SetUpTimeout:    setUpTimeout,
		TearDownTimeout: tearDownTimeout,
		Vars:            []string{"compliance.HostName", "compliance.User"},
	})
}

// TestFixture is the compliance test fixture.
type TestFixture struct {
	Host *WindowsHost
}

// SetUp sets up the test fixture and connects to the CallboxManager.
func (tf *TestFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	dut := s.DUT()

	// Get host connected to the compliance measurement device.
	cHost, ok := s.Var("compliance.HostName")
	if !ok || cHost == "" {
		testing.ContextLog(ctx, "No host specified, deducing from DUT name")
		var err error
		if cHost, err = complianceHostName(dut); err != nil {
			s.Fatal("Failed to determine compliance hostname: ", err)
		}
	}

	user, ok := s.Var("compliance.User")
	if !ok || user == "" {
		testing.ContextLogf(ctx, "No username provided, using default: %q", complianceUser)
		user = complianceUser
	}

	testing.ContextLogf(ctx, "Using compliance host: %q", cHost)
	host, err := NewWindowsHost(ctx, cHost, dut, user)
	if err != nil {
		s.Fatal("Failed to initialize compliance host: ", err)
	}
	tf.Host = host
	return tf
}

// complianceHostName derives the hostname of the compliance tester from the DUTs hostname.
//
// Compliance Hosts follow the convention: <compliance_host>-host<host_number>
// e.g. a compliance host with the name "chromeos1-sinclair-compliance1" may support the following DUTs:
// "chromeos1-sinclair-compliance1-host1", "chromeos1-sinclair-compliance1-host2", ...
func complianceHostName(dut *dut.DUT) (string, error) {
	dutHost := dut.HostName()
	if host, _, err := net.SplitHostPort(dutHost); err == nil {
		dutHost = host
	}

	dutHost = strings.TrimSuffix(dutHost, ".cros")
	if dutHost == "localhost" {
		return "", errors.Errorf("unable to parse hostname from: %q, localhost not supported", dutHost)
	}

	if ip := net.ParseIP(dutHost); ip != nil {
		return "", errors.Errorf("unable to parse hostname from: %q, ip:port format not supported", dutHost)
	}

	hostname := strings.Split(dutHost, "-")
	if len(hostname) < 2 {
		return "", errors.Errorf("unable to parse hostname from: %q, unknown name format", dutHost)
	}

	// Strip off final part of hostname.
	hostname = hostname[0 : len(hostname)-1]
	return fmt.Sprintf("%s:22", strings.Join(hostname, "-")), nil
}

// Reset does nothing currently, but is required for the test fixture.
func (tf *TestFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest initializes the test fixture before each test run.
func (tf *TestFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

// PostTest cleans up the test fixture after each test run.
func (tf TestFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

// TearDown releases resources held open by the test fixture.
func (tf *TestFixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if tf.Host == nil {
		return
	}
	if err := tf.Host.Close(ctx); err != nil {
		s.Log("Failed to close tunnel to compliance host: ", err)
	}
}
