// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package topology

import (
	"context"
	"net"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/testing"
)

const (
	setupTimeout    = 2 * time.Minute
	preTestTimeout  = 2 * time.Minute
	postTestTimeout = 2 * time.Minute
)

func init() {
	testing.AddFixture(&testing.Fixture{
		Name:            "wwcbStorage",
		Desc:            "PASIT fixture that initializes storage topology for storage tests",
		Contacts:        []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent:    "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Impl:            &TestFixture{defaultTopology: defaultStorageTopology},
		SetUpTimeout:    setupTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"USBID"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "wwcbStorageEnableServoAndDisableTabletMode",
		Desc:            "PASIT fixture that initializes storage topology for storage tests",
		Contacts:        []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent:    "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Impl:            &TestFixture{defaultTopology: defaultStorageTopology},
		SetUpTimeout:    setupTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          "enableServoAndDisableTabletMode",
		Vars:            []string{"USBID"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "wwcbStorageEnableServoAndTabletMode",
		Desc:            "PASIT fixture that initializes storage topology for storage tests",
		Contacts:        []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent:    "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Impl:            &TestFixture{defaultTopology: defaultStorageTopology},
		SetUpTimeout:    setupTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Parent:          "enableServoAndTabletMode",
		Vars:            []string{"USBID"},
	})
}

var ipPowerPorts = []int{1}

func varOrDefault(s *testing.FixtState, varName, defaultValue string) string {
	if val, ok := s.Var(varName); ok {
		return val
	}
	return defaultValue
}

func defaultStorageTopology(s *testing.FixtState, hostname string) *labapi.PasitHost {
	usbID := varOrDefault(s, "USBID", "2001902")
	return DefaultStorageTopology(hostname, usbID)
}

// TestFixture is the PASIT test fixture.
type TestFixture struct {
	Helper          *Helper
	defaultTopology func(*testing.FixtState, string) *labapi.PasitHost
}

// SetUp configures the fixture.
func (tf *TestFixture) SetUp(ctx context.Context, s *testing.FixtState) interface{} {
	hostname := s.DUT().HostName()
	if host, _, err := net.SplitHostPort(hostname); err == nil {
		hostname = host
	}

	var pasitTopology *labapi.PasitHost
	if dutConfig, err := s.ChromeOSDUTLabConfig(""); err == nil {
		if dutConfig.GetChromeos().GetPasitHost() != nil {
			pasitTopology = dutConfig.GetChromeos().GetPasitHost()
			s.Log("Loaded DUT info from lab config")
		}
	}

	// No dut topology defined, use default.
	if pasitTopology == nil {
		pasitTopology = tf.defaultTopology(s, hostname)
		s.Log("Loaded DUT info from CLI args")
	}

	tf.Helper = NewHelper(pasitTopology, hostname)
	return tf
}

// Reset does nothing currently, but is required for the test fixture.
func (tf *TestFixture) Reset(ctx context.Context) error {
	return nil
}

// PreTest initializes the test fixture before each test run.
func (tf *TestFixture) PreTest(ctx context.Context, s *testing.FixtTestState) {
	// Initialize fixtures to find the connected devices.
	if err := tf.Helper.InitializeFixtures(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}

	// Try to power cycle IP power for DUTs that have it.
	if err := utils.OpenIppower(ctx, ipPowerPorts); err != nil {
		// Just log errors here since this may not always be provided.
		// Later this should be moved into the proto.
		s.Log("Failed to power on the docking station: ", err)
	}
}

// PostTest cleans up the test fixture after each test run.
func (tf TestFixture) PostTest(ctx context.Context, s *testing.FixtTestState) {
	tf.Helper.ResetAll(ctx)
	utils.CloseIppower(ctx, ipPowerPorts)
}

// TearDown releases resources held open by the test fixture.
func (tf *TestFixture) TearDown(ctx context.Context, s *testing.FixtState) {
}
