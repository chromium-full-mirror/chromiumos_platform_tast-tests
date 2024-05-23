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
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"

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
		Attr:         []string{"group:wwcb", "group:pasit"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		ServiceDeps:  []string{"tast.cros.browser.ChromeService"},
		Vars:         []string{"servo", "newTestItem"},
		Data:         []string{"Capabilities.json"},
		Fixture:      "wwcbPasitDock",
		Timeout:      utils.TestingTimeout,
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
	if err := utils.DisableServoPower(ctx, dut, pxy.Servo()); err != nil {
		s.Fatal("Failed to cut-off servo power supply: ", err)
	}
	defer utils.EnableServoPower(ctx, dut, pxy.Servo())

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

	// Verify connection to DUT.
	if err := testing.Poll(ctx, dut.Connect, &testing.PollOptions{Timeout: 60 * time.Second}); err != nil {
		s.Fatal("Failed to connect to DUT: ", err)
	}

	tf := s.FixtValue().(*topology.TestFixture)
	_, USBDevices, err := tf.ConnectPeripheralsViaDock(ctx, dut)
	if err != nil {
		s.Fatal("Failed to connect the peripherals via Dock: ", err)
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

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}

		if err := utils.VerifyUSBTypeADeviceSpeed(ctx, dut, s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the usb devices speed: ", err)
		}
	}

	ippowerPorts := []int{1}
	if err := powerOffOn(ctx, dut, ippowerPorts, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after power off/on the docking station: ", err)
	}

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}

		if err := utils.VerifyUSBTypeADeviceSpeed(ctx, dut, s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the usb devices speed: ", err)
		}
	}

	if err := rebootDUT(ctx, dut, ippowerPorts, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after reboot DUT: ", err)
	}

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}

		if err := utils.VerifyUSBTypeADeviceSpeed(ctx, dut, s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the usb devices speed: ", err)
		}
	}

	if err := powerOffRebootOn(ctx, dut, ippowerPorts, USBDevices); err != nil {
		s.Fatal("Failed to verify peripherals after power off dock, reboot DUT, power on dock: ", err)
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
