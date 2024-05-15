// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cellular

import (
	"context"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/services/cros/cellular"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:         "cellularRebootSetupRemote",
		Desc:         "A remote fixture that reboots the device as part of the setup to help enforce isolation between tests",
		Contacts:     []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Impl:         newFixture(rebootOnSetup),
		SetUpTimeout: 3 * time.Minute,
		Vars:         []string{"skipReboot"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularSuspendRemote",
		Desc:            "Remote cellular suspend-resume test fixture that reboots in pre and post test to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Impl:            newFixture(rebootOnSetup, rebootOnTeardown),
		SetUpTimeout:    3 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		Vars:            []string{"skipReboot"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularStressRemote",
		Desc:            "Remote cellular stress test fixture that reboots in setup and teardown to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Impl:            newFixture(rebootOnSetup, rebootOnTeardown),
		SetUpTimeout:    3 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		Vars:            []string{"skipReboot"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularE2ERemote",
		Desc:            "Remote cellular e2e test fixture that reboots in setup and teardown to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Impl:            newFixture(rebootOnSetup, rebootOnTeardown),
		SetUpTimeout:    3 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		Vars:            []string{"skipReboot"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularHotspotRemote",
		Desc:            "Remote cellular hotspot test fixture that reboots in setup and teardown to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Impl:            newFixture(rebootOnSetup, rebootOnTeardown),
		SetUpTimeout:    3 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		Vars:            []string{"skipReboot"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:         "cellularDUTCheckRemote",
		Desc:         "Remote cellular dut check test fixture that reboots during setup",
		Contacts:     []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent: "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		// Just reboot in SetUp since there shouldn't be any side effects to these tests.
		Impl:         newFixture(rebootOnSetup),
		SetUpTimeout: 3 * time.Minute,
		Vars:         []string{"skipReboot"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularAutoconnectRemote",
		Desc:            "Remote cellular autoconnect test fixture that reboots in setup and teardown to help enforce isolation",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Impl:            newFixture(rebootOnSetup, rebootOnTeardown),
		SetUpTimeout:    3 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		Vars:            []string{"skipReboot"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "cellularEnforceConnectionRemote",
		Desc:            "Remote cellular fixture that ensures the device is connected as part of reset",
		Contacts:        []string{"chromeos-cellular-team@google.com", "jstanko@google.com"},
		BugComponent:    "b:167157", // ChromeOS > Platform > Connectivity > Cellular
		Impl:            newFixture(rebootOnSetup, rebootOnTeardown, enforceConnected),
		SetUpTimeout:    3 * time.Minute,
		TearDownTimeout: 3 * time.Minute,
		ResetTimeout:    5 * time.Minute,
		ServiceDeps:     []string{"tast.cros.cellular.RemoteCellularService"},
		Vars:            []string{"skipReboot"},
	})
}

type fixtureOption func(*fixture)

var rebootOnSetup = func(f *fixture) {
	f.rebootOnSetup = true
}

var rebootOnTeardown = func(f *fixture) {
	f.rebootOnTeardown = true
}

var enforceConnected = func(f *fixture) {
	f.enforceConnected = true
}

func newFixture(options ...fixtureOption) *fixture {
	fixture := &fixture{}
	for _, opt := range options {
		opt(fixture)
	}
	return fixture
}

type fixture struct {
	rebootOnSetup    bool
	rebootOnTeardown bool
	enforceConnected bool
	dut              *dut.DUT
	hint             *testing.RPCHint
}

// If the uptime is less than skipStartupRebootUptime, then skip rebooting in startup since the device
// has been rebooted recently (likely in post-test of previous test group or by control file).
const skipStartupRebootUptime = 60 * time.Second

func (tf *fixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	// If skipReboot var was provided by user, then don't reboot.
	if _, ok := s.Var("skipReboot"); ok {
		tf.rebootOnSetup = false
		tf.rebootOnTeardown = false
	}

	d := s.DUT()
	if !d.Connected(ctx) {
		s.Log("Reconnecting to DUT")
		if err := d.Connect(ctx); err != nil {
			s.Fatal("Failed to connect to DUT: ", err)
		}
	}
	tf.dut = d
	tf.hint = s.RPCHint()

	if tf.rebootOnSetup && hasStartupRebootUptime(ctx, s) {
		s.Log("Rebooting DUT")
		if err := d.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
	}
	return tf
}

func (tf *fixture) Reset(ctx context.Context) error {
	if !tf.enforceConnected {
		return nil
	}

	// Reconnect to DUT if needed.
	if !tf.dut.Connected(ctx) {
		testing.ContextLog(ctx, "Reconnecting to DUT")
		if err := tf.dut.Connect(ctx); err != nil {
			return errors.Wrap(err, "failed to connect to dut")
		}
	}

	cl, err := rpc.Dial(ctx, tf.dut, tf.hint)
	if err != nil {
		return errors.Wrap(err, "failed to connect to the RPC service on the DUT")
	}
	defer cl.Close(ctx)

	remoteCellularClient := cellular.NewRemoteCellularServiceClient(cl.Conn)
	if _, err := remoteCellularClient.SetUp(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to initialize cellular shill service on DUT")
	}

	// If we fail to connect to the cellular network, we should return an error.
	// On error tast will call SetUp again which will cause our DUT to reboot (if that rebootOnSetup is true).
	if _, err := remoteCellularClient.Connect(ctx, &empty.Empty{}); err != nil {
		return errors.Wrap(err, "failed to connect to cellular service")
	}
	return nil
}

func (tf *fixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
}

func (tf *fixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
}

func (tf *fixture) TearDown(ctx context.Context, s *testing.FixtState) {
	if tf.rebootOnTeardown {
		d := s.DUT()

		// DUT may be disconnected prior to TearDown even if there are more tests
		// to run. Reconnect so we can reboot.
		if !d.Connected(ctx) {
			s.Log("Reconnecting to DUT")
			if err := d.Connect(ctx); err != nil {
				s.Fatal("Failed to connect to DUT: ", err)
			}
		}

		s.Log("Rebooting DUT")
		if err := d.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot: ", err)
		}
	}
}

// hasStartupRebootUptime returns true if the device has been up long enough to reboot on startup.
func hasStartupRebootUptime(ctx context.Context, s *testing.FixtState) bool {
	uptimeStr, err := s.DUT().Conn().CommandContext(ctx, "cat", "/proc/uptime").Output(ssh.DumpLogOnError)
	if err != nil {
		// Failed to read uptime, reboot to be safe.
		s.Error("Failed to read system uptime: ", err)
		return true
	}

	uptimeFloat, err := strconv.ParseFloat(strings.Fields(string(uptimeStr))[0], 64)
	if err != nil {
		s.Errorf("Failed to parse system uptime %q", string(uptimeStr))
		return true
	}

	s.Logf("Current uptime: %f seconds", uptimeFloat)
	return time.Duration(uptimeFloat)*time.Second >= skipStartupRebootUptime
}
