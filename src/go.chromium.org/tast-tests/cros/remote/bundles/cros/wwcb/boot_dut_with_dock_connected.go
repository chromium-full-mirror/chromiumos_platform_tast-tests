// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         BootDUTWithDockConnected,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "In clamshell and tablet modes, verify connection of peripherals after cold-boot DUT with the dock station connected",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb", "group:pasit"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "EthernetID", "USBTypeAIDArray", "wwcbIPPowerIp", "newTestItem"},
		Data:         []string{"Capabilities.json"},
		Timeout:      10 * time.Minute,
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
	})
}

func BootDUTWithDockConnected(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	extDispID := s.RequiredVar("ExtDispID1")
	ethernetID := s.RequiredVar("EthernetID")
	USBDeviceIDs := strings.Split(s.RequiredVar("USBTypeAIDArray"), ",")

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
	defer func(context.Context) {
		if !dut.Connected(ctx) {
			if err := utils.PowerOnDUT(ctx, pxy, dut); err != nil {
				s.Error("Failed to power on DUT at clean-up: ", err)
			}
		}
	}(cleanupCtx)

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize the fixture: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Open IP power to supply docking power.
	ippowerPorts := []int{1}
	if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
		s.Fatal("Failed to power on the docking station: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, ippowerPorts)

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	// Connect docking station, ext-display, Ethernet, USB devices.
	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to connect to the docking station: ", err)
	}
	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}
	if err := utils.ControlFixture(ctx, ethernetID, "on"); err != nil {
		s.Fatal("Failed to connect to the Ethernet: ", err)
	}
	for _, usbID := range USBDeviceIDs {
		if err := utils.ControlFixture(ctx, usbID, "on"); err != nil {
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
	if err := utils.VerifyDisplayCount(ctx, dut, 2); err != nil {
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
		if err := utils.VerifyDockingInterface(ctx, s.DUT(), dockingID, s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}

		if err := utils.VerifyUSBTypeADeviceSpeed(ctx, dut, s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the usb devices speed: ", err)
		}
	}
}
