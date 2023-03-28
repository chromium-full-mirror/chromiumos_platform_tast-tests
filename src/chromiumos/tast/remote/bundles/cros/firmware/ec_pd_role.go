// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/errors"
	"chromiumos/tast/remote/dutfs"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/remote/firmware/fixture"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ECPDRole,
		Desc:         "Verify USB-C/PD source role policy",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr and add linto@chromium.org to gerrit review.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Lid()),
		Timeout:      20 * time.Minute,
	})
}

func ECPDRole(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	s.Log("Rebooting the DUT with hard reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to EC reset DUT: ", err)
	}

	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 8*time.Minute)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx); err != nil {
		s.Fatal("Failed to reconnect DUT: ", err)
	}

	// Stainless reported that some DUTs weren't able to reach S5 or G3 while
	// lid closed. Adding some delay here, after a cold reset, helped make
	// the test more stable.
	s.Log("Sleeping for one minute")
	if err := testing.Sleep(ctx, 1*time.Minute); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	// Parse the usb pd port list.
	usbcPorts, err := listUSBPdPorts(ctx, h)
	if err != nil {
		s.Fatal("Failed to list USB-C ports: ", err)
	}

	for _, step := range []struct {
		testAction   func(context.Context, *firmware.Helper) error
		expectStatus servo.USBPdDualRoleValue
	}{
		{
			testAction:   usbPdCloseLid,
			expectStatus: servo.USBPdDualRoleSink,
		},
		{
			testAction:   usbPdOpenLid,
			expectStatus: servo.USBPdDualRoleOn,
		},
		{
			testAction:   usbPdSuspend,
			expectStatus: servo.USBPdDualRoleOff,
		},
	} {
		if step.expectStatus == servo.USBPdDualRoleSink {
			// When powered off, we found two duts on Stainless with their pd dual-role status
			// reported as "off", rather than "force sink".
			modelsWithPDOffDurG3 := []string{"elm", "hana"}
			if func(modelName string, modelPool []string) bool {
				for _, m := range modelPool {
					if modelName == m {
						return true
					}
				}
				return false
			}(h.Model, modelsWithPDOffDurG3) {
				step.expectStatus = servo.USBPdDualRoleOff
			}
		}
		if err := step.testAction(ctx, h); err != nil {
			s.Fatal("Action failed: ", err)
		}
		// Verify status of all usb-c port.
		for _, portID := range usbcPorts {
			if err := h.Servo.CheckUSBPdStatus(ctx, portID, step.expectStatus); err != nil {
				s.Fatal("Failed to check for USB PD: ", err)
			}
		}
	}
}

func listUSBPdPorts(ctx context.Context, h *firmware.Helper) ([]int, error) {
	bout, err := h.DUT.Conn().CommandContext(ctx, "ectool", "usbpdpower").Output()
	if err != nil {
		return nil, errors.Wrap(err, "failed to run usbpdpower")
	}
	r := regexp.MustCompile(`Port (\d)`)
	matches := r.FindAllStringSubmatch(string(bout), -1)
	if len(matches) == 0 {
		return nil, errors.New("could not find any usb pd ports")
	}

	var usbPdPorts []int
	for _, port := range matches {
		p, err := strconv.Atoi(port[1])
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse port ID")
		}
		usbPdPorts = append(usbPdPorts, p)
	}
	return usbPdPorts, nil
}

func usbPdCloseLid(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.CloseLid(ctx); err != nil {
		return errors.Wrap(err, "failed to close lid")
	}
	// Similar to ticket b:268492022, setting lid open while DUT at S5 failed
	// on some machines. But, waiting for G3 before opening lid worked.
	testing.ContextLog(ctx, "Checking for G3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "G3"); err != nil {
		// On some models, such as kracko, the sequence to G3 following a power-off request got blocked
		// because modelfwd.lock files were never cleared from '/run/lock/power_override/'. Check for
		// files that still exist under this directory.
		var lockFiles []string
		checkLockFiles := func() error {
			lockFilesPath := "/run/lock/power_override/"
			if err := h.RequireRPCClient(ctx); err != nil {
				return errors.Wrap(err, "failed to open RPC client")
			}
			fs := dutfs.NewClient(h.RPCClient.Conn)
			out, err := fs.ReadDir(ctx, lockFilesPath)
			if err != nil {
				return errors.Wrapf(err, "failed to read from %s", lockFilesPath)
			}
			if len(out) == 0 {
				return errors.Errorf("found %s empty", lockFilesPath)
			}
			for _, file := range out {
				lockFiles = append(lockFiles, file.Name())
			}
			return nil
		}
		if err := checkLockFiles(); err != nil {
			testing.ContextLog(ctx, "Unexpected error in checking for lock files: ", err)
		}
		return errors.Wrapf(err, "failed to get G3 powerstate, found lock files: %s", lockFiles)
	}
	return nil
}

func usbPdOpenLid(ctx context.Context, h *firmware.Helper) error {
	if err := h.Servo.OpenLid(ctx); err != nil {
		return errors.Wrap(err, "failed to open lid")
	}
	// During boot-up, dut would reach S0 first before getting reconnected.
	testing.ContextLog(ctx, "Checking for S0 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
		return errors.Wrap(err, "failed to get power state at S0")
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 8*time.Minute)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	// Allowing some delay makes the test more stable.
	testing.ContextLog(ctx, "Sleeping for one minute")
	if err := testing.Sleep(ctx, 1*time.Minute); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}
	return nil
}

func usbPdSuspend(ctx context.Context, h *firmware.Helper) error {
	testing.ContextLog(ctx, "Suspending DUT")
	if err := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend").Start(); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}

	testing.ContextLog(ctx, "Checking for S0ix, S3, S5, or G3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0ix", "S3", "S5", "G3"); err != nil {
		return errors.Wrap(err, "failed to get power state at S0ix, S3, S5, or G3")
	}
	return nil
}
