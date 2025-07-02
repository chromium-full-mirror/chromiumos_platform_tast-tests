// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BootDUTWithDockConnected,
		Desc:         "In clamshell and tablet modes, verify connection of peripherals after cold-boot DUT with the dock station connected",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr: []string{
			"group:wwcb",
			"group:pasit",
			"group:release-health",
			"release-health_usb",
			"pasit_dock",
		},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Vars:         []string{"servo", "newTestItem"},
		TestBedDeps:  []string{tbdep.ServoStateWorking},
		Data:         []string{"Capabilities.json"},
		Fixture:      "wwcb.dock",
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  false,
		}, {
			Name: "tablet_mode",
			Val:  true,
		}, {
			Name:      "fast",
			ExtraAttr: []string{"pasit_fast"},
			Val:       true,
		}},
		Timeout: 5 * time.Minute,
	})
}

func BootDUTWithDockConnected(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 60*time.Second)
	defer cancel()

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	// Cutting off the servo power supply.
	if err := utils.DisableServoPower(ctx, dut, pxy.Servo()); err != nil {
		s.Fatal("Failed to cut-off servo power supply: ", err)
	}
	defer utils.EnableServoPower(ctx, dut, pxy.Servo())

	// Enable tablet mode.
	if s.Param().(bool) {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode off")
	}

	beforeShutdwonDUT, err := utils.GetUSBDevice(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get USB device: ", err)
	}

	if err := utils.ShutdownDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to shut down DUT: ", err)
	}

	defer func(ctx context.Context) {
		// power on dut should precede any logs collection from the DUT
		if !dut.Connected(ctx) {
			if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
				s.Error("Failed to power on DUT at clean-up: ", err)
				return
			}
		}

		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	// Connect docking station, ext-display, Ethernet, USB devices.
	tf := s.FixtValue().(*topology.TestFixture)
	_, dockID, err := tf.Helper.ActivateDeviceByTypeVia(ctx, topology.DeviceTypeMonitor, topology.DeviceTypeDockingStation)
	if err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}
	if _, err := tf.Helper.ActivateDeviceByTypeViaId(ctx, topology.DeviceTypeNetwork, dockID); err != nil {
		s.Fatal("Failed to connect to the Ethernet: ", err)
	}
	// Enable any auxiliary USB devices that pass through the dock, these devices are categorized as "HID"
	for _, usbID := range tf.Helper.DevicesByTypeViaId(topology.DeviceTypeHID, dockID) {
		if err := tf.Helper.ActivateDeviceByID(ctx, usbID); err != nil {
			s.Fatal("Failed to connect to the USB device: ", err)
		}
	}

	// Power on DUT.
	if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to power on DUT: ", err)
	}

	// Do the verification on power, display, Ethernet, USB audio.
	if err := utils.VerifyPowerStatus(ctx, dut, true); err != nil {
		s.Fatal("Failed to verify power is charging: ", err)
	}
	if err := utils.VerifyEthernetState(ctx, dut, true); err != nil {
		s.Fatal("Failed to verify Ethernet is connected: ", err)
	}
	if err := utils.VerifyDisplayCountEquals(ctx, dut, 2); err != nil {
		s.Fatal("Failed to verify external display is connected: ", err)
	}
	afterConnectPeripherals, err := utils.GetUSBDevice(ctx, s.DUT())
	if err != nil {
		s.Fatal("Failed to get USB device: ", err)
	}
	if len(afterConnectPeripherals) <= len(beforeShutdwonDUT) {
		s.Fatalf("Unexpect USB device; before shutdwon DUT: %v, after connect peripherals: %v", strings.Join(beforeShutdwonDUT, "\n"), strings.Join(afterConnectPeripherals, "\n"))
	}

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}

		if err := utils.VerifyUSBTypeADeviceSpeed(ctx, dut, s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the usb devices speed: ", err)
		}
	}
}
