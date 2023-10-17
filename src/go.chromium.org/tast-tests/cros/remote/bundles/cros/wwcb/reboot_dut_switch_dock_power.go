// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RebootDUTSwitchDockPower,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "In clamshell and tablet modes, verifies the connection of the peripherals after power off and on the docking station during DUT reboot",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		ServiceDeps:  []string{"tast.cros.browser.ChromeService"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "EthernetID", "USBTypeAIDArray", "wwcbIPPowerIp"},
		Timeout:      5 * time.Minute,
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  false,
		}, {
			Name: "tablet_mode",
			Val:  true,
		}},
	})
}

func RebootDUTSwitchDockPower(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	if s.Param().(bool) {
		testing.ContextLog(ctx, "Enable tablet mode")
		if err := pxy.Servo().RunECCommand(ctx, "tabletmode on"); err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer pxy.Servo().RunECCommand(cleanupCtx, "tabletmode off")
	}

	// Cutting off the servo power supply.
	if err := pxy.Servo().SetPDRole(ctx, servo.PDRoleSnk); err != nil {
		s.Fatal("Failed to cut-off servo power supply: ", err)
	}
	defer pxy.Servo().SetPDRole(cleanupCtx, servo.PDRoleSrc)

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

	dockingID := s.RequiredVar("DockingID")
	extDispID := s.RequiredVar("ExtDispID1")
	ethernetID := s.RequiredVar("EthernetID")
	USBTypeAIDArray := strings.Split(s.RequiredVar("USBTypeAIDArray"), ",")

	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to plug in the docking station: ", err)
	}

	USBDevices, err := utils.ConnectPeripheralsViaDock(ctx, dut, extDispID, ethernetID, USBTypeAIDArray)
	if err != nil {
		s.Fatal("Failed to connect peripherals via Dock: ", err)
	}
	defer func(context.Context) {
		if s.HasError() {
			USBDevicesFailed, err := utils.GetUSBDevice(ctx, dut)
			if err != nil {
				s.Log("Failed to get the USB device when fail the test: ", err)
			}
			testing.ContextLog(ctx, strings.Join(USBDevicesFailed, "\n"))
		}
	}(cleanupCtx)

	if err := powerOffOn(ctx, dut, ippowerPorts, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after power off/on the docking station: ", err)
	}

	if err := rebootDUT(ctx, dut, ippowerPorts, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after reboot DUT: ", err)
	}

	if err := powerOffRebootOn(ctx, dut, ippowerPorts, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after power off dock, reboot DUT, power on dock: ", err)
	}
}

// powerOffOn powers off and on the docking station and verifies the connection of peripherals.
func powerOffOn(ctx context.Context, dut *dut.DUT, ippowerPorts []int, USBDevices []string) error {
	if err := utils.CloseIppower(ctx, ippowerPorts); err != nil {
		return errors.Wrap(err, "failed to power off the docking station")
	}

	// GoBigSleepLint: Prevent Dock cached
	testing.Sleep(ctx, 5*time.Second)

	if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
		return errors.Wrap(err, "failed to power on the docking station")
	}

	if err := utils.VerifyPeripheralsConnection(ctx, dut, true, USBDevices); err != nil {
		return errors.Wrap(err, "failed to verify connection of peripherals")
	}
	return nil
}

// rebootDUT reboots the DUT then verifies the connection of peripherals.
func rebootDUT(ctx context.Context, dut *dut.DUT, ippowerPorts []int, USBDevices []string) error {
	if err := dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	if err := utils.VerifyPeripheralsConnection(ctx, dut, true, USBDevices); err != nil {
		return errors.Wrap(err, "failed to verify connection of peripherals")
	}
	return nil
}

// powerOffRebootOn powers off the docking station, reboots the DUT, powers on the docking station then verifies the connection of peripherals.
func powerOffRebootOn(ctx context.Context, dut *dut.DUT, ippowerPorts []int, USBDevices []string) error {
	if err := utils.CloseIppower(ctx, ippowerPorts); err != nil {
		return errors.Wrap(err, "failed to power off the docking station")
	}

	if err := dut.Reboot(ctx); err != nil {
		return errors.Wrap(err, "failed to reboot DUT")
	}

	if err := utils.OpenIppower(ctx, ippowerPorts); err != nil {
		return errors.Wrap(err, "failed to power on the docking station")
	}

	if err := utils.VerifyPeripheralsConnection(ctx, dut, true, USBDevices); err != nil {
		return errors.Wrap(err, "failed to verify connection of peripherals")
	}
	return nil
}
