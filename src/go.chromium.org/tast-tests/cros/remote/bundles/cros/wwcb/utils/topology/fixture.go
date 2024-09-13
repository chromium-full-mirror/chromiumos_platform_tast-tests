// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package topology

import (
	"context"
	"net"
	"strings"
	"time"

	labapi "go.chromium.org/chromiumos/config/go/test/lab/api"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
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
	testing.AddFixture(&testing.Fixture{
		Name:            "wwcbCamera",
		Desc:            "PASIT fixture that initializes camera topology for camera tests",
		Contacts:        []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent:    "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Impl:            &TestFixture{defaultTopology: defaultCameraTopology},
		SetUpTimeout:    setupTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"ExtCameraID"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "wwcbDisplay",
		Desc:            "PASIT fixture that initializes display topology for display tests",
		Contacts:        []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent:    "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Impl:            &TestFixture{defaultTopology: defaultDisplayTopology},
		SetUpTimeout:    setupTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"DockingID", "ExtDispID1", "ExtDispID2"},
	})
	testing.AddFixture(&testing.Fixture{
		Name:            "wwcbPasitDock",
		Desc:            "PASIT fixture that initializes pasit full topology for tests",
		Contacts:        []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent:    "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Impl:            &TestFixture{defaultTopology: defaultFullTopology},
		SetUpTimeout:    setupTimeout,
		PreTestTimeout:  preTestTimeout,
		PostTestTimeout: postTestTimeout,
		Vars:            []string{"DockingID", "ExtDispID1", "ExtDispID2", "EthernetID", "USBTypeAIDArray"},
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

func defaultCameraTopology(s *testing.FixtState, hostname string) *labapi.PasitHost {
	cameraID := varOrDefault(s, "ExtCameraID", "2001903")
	return DefaultCameraTopology(hostname, cameraID)
}

func defaultDisplayTopology(s *testing.FixtState, hostname string) *labapi.PasitHost {
	disp1ID := varOrDefault(s, "ExtDispID1", "2109001")
	disp2ID := varOrDefault(s, "ExtDispID2", "2109002")
	return DefaultDisplayTopology(hostname, disp1ID, disp2ID)
}

func defaultFullTopology(s *testing.FixtState, hostname string) *labapi.PasitHost {
	dockingID := varOrDefault(s, "DockingID", "1912901")
	disp1ID := varOrDefault(s, "ExtDispID1", "2007901")
	disp2ID := varOrDefault(s, "ExtDispID2", "2007902")
	ethID := varOrDefault(s, "EthernetID", "j45sw01")
	usbID := varOrDefault(s, "USBTypeAIDArray", "2001901")

	var usbs []string
	if usbID != "" {
		usbs = strings.Split(usbID, ",")
	}

	return DefaultFullTopology(hostname, dockingID, disp1ID, disp2ID, ethID, usbs...)
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

// ConnectPeripheralsViaDock connects the peripherals via the dock, verifies each connection and returns the list of USB devices.
// Peripherals devices such as external display, ethernet, USB (audio) devices.
func (tf *TestFixture) ConnectPeripheralsViaDock(ctx context.Context, dut *dut.DUT) (string, []string, error) {
	docks := tf.Helper.DevicesByType(DeviceTypeDockingStation)
	if len(docks) != 1 {
		return "", nil, errors.Errorf("failed to find dock, expected 1 docks got %d", len(docks))
	}

	dockID := docks[0]
	if _, err := tf.Helper.ActivateDeviceByTypeViaId(ctx, DeviceTypeMonitor, dockID); err != nil {
		return "", nil, errors.Wrap(err, "failed to connect monitor")
	}

	if _, err := tf.Helper.ActivateDeviceByTypeViaId(ctx, DeviceTypeNetwork, dockID); err != nil {
		return "", nil, errors.Wrap(err, "failed to connect ethernet")
	}

	usbTypeADeviceIDs := tf.Helper.DevicesByTypeViaId(DeviceTypeHID, dockID)
	for _, deviceID := range usbTypeADeviceIDs {
		if err := tf.Helper.ActivateDeviceByID(ctx, deviceID); err != nil {
			return "", nil, errors.Wrapf(err, "failed to connect to the USB Type-A device: %s", deviceID)
		}
	}

	usbDevices, err := utils.GetStableUSBDevices(ctx, dut)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to get a list of USB devices")
	}

	if err := utils.VerifyPeripheralsConnection(ctx, dut, true, usbDevices); err != nil {
		return "", nil, errors.Wrap(err, "failed to verify connections to the peripherals")
	}

	return dockID, usbDevices, nil
}

// VerifyDockingInterface verifies the docking interface is the same as the one in the capabilities.json file for meta tests.
func (tf *TestFixture) VerifyDockingInterface(ctx context.Context, dut *dut.DUT, capFile string) error {
	dockIDs := tf.Helper.DevicesByType(DeviceTypeDockingStation)
	if len(dockIDs) != 1 {
		return errors.Errorf("failed to determine dock ID, expected 1 docks, got %d", len(dockIDs))
	}

	dockingID := dockIDs[0]
	disable := func(ctx context.Context) error {
		if err := tf.Helper.DeactivateDeviceByID(ctx, dockingID); err != nil {
			return errors.Wrap(err, "failed to connect to the external storage")
		}
		return nil
	}

	enable := func(ctx context.Context) error {
		if err := tf.Helper.ActivateDeviceByID(ctx, dockingID); err != nil {
			return errors.Wrap(err, "failed to connect to the external storage")
		}
		return nil
	}

	if err := utils.VerifyDockingInterface(ctx, dut, capFile, disable, enable); err != nil {
		return errors.Wrap(err, "failed to verify the docking station interface")
	}
	return nil
}
