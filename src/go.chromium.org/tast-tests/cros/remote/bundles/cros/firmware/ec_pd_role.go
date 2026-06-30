// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"golang.org/x/exp/slices"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ECPDRole,
		Desc: "Verify USB-C/PD source role policy",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_pd", "firmware_meets_kpi", "firmware_stressed", "firmware_ec_ro", "firmware_ec_rw", "firmware_bios_pdc"},
		Fixture:      fixture.NormalMode,
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.Lid()),
		Timeout:      20 * time.Minute,
	})
}

type dutState struct {
	lidOpen bool
	suspend bool
}

func ECPDRole(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	testParams := firmware.PDTestParams{
		DTS: firmware.DTSModeOff,
	}

	if err := firmware.SetupPDTester(ctx, h, testParams, s.OutDir()); err != nil {
		s.Fatal("Failed to configure Servo for PD testing: ", err)
	}

	s.Log("Rebooting the DUT with hard reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to EC reset DUT: ", err)
	}

	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect DUT: ", err)
	}

	// Parse the usb pd port list.
	usbcPorts, err := listUSBPdPorts(ctx, h)
	if err != nil {
		s.Fatal("Failed to list USB-C ports: ", err)
	}

	currentDutState := dutState{
		lidOpen: true,
		suspend: false,
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 30*time.Second)
	defer cancel()

	defer func(ctx context.Context) {
		if !currentDutState.lidOpen {
			if err := h.Servo.OpenLid(ctx); err != nil {
				s.Fatal("Failed to open lid: ", err)
			}
		}
	}(cleanupCtx)

	var chromeService pb.ChromeServiceClient
	for _, step := range []struct {
		needLogin    bool
		testAction   func(context.Context, *firmware.Helper, *dutState) error
		expectStatus servo.USBPdDualRoleValue
	}{
		{
			needLogin:    false,
			testAction:   usbPdCloseLid,
			expectStatus: servo.USBPdDualRoleSink,
		},
		{
			needLogin:    false,
			testAction:   usbPdOpenLid,
			expectStatus: servo.USBPdDualRoleOn,
		},
		{
			needLogin:    true,
			testAction:   usbPdSuspend,
			expectStatus: servo.USBPdDualRoleOff,
		},
	} {
		if step.needLogin {
			if err := h.RequireRPCClient(ctx); err != nil {
				s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
			}
			s.Log("Starting a new Chrome")
			chromeService = pb.NewChromeServiceClient(h.RPCClient.Conn)
			if _, err := chromeService.New(ctx, &pb.NewRequest{
				LoginMode: pb.LoginMode_LOGIN_MODE_GUEST_LOGIN,
			}); err != nil {
				s.Fatal("Failed to create new Chrome at login: ", err)
			}
			defer func(ctx context.Context) {
				if currentDutState.suspend {
					wakeupKey := servo.Enter
					if h.Config.ModeSwitcherType == firmware.TabletDetachableSwitcher {
						wakeupKey = servo.PowerKey
					}
					testing.ContextLogf(ctx, "Waking DUT from suspend by %s", wakeupKey)
					if err := h.Servo.KeypressWithDuration(ctx, wakeupKey, servo.DurPress); err != nil {
						s.Fatal("Failed to wake DUT from suspend: ", err)
					}
				}
				chromeService.Close(ctx, &empty.Empty{})
			}(cleanupCtx)
		}

		if step.expectStatus == servo.USBPdDualRoleSink {
			// When powered off, we found two duts on Stainless with their pd dual-role status
			// reported as "off", rather than "force sink".
			modelsWithPDOffDurG3 := []string{"elm", "hana"}
			if slices.Contains(modelsWithPDOffDurG3, h.Model) {
				step.expectStatus = servo.USBPdDualRoleOff
			}
		}
		if err := step.testAction(ctx, h, &currentDutState); err != nil {
			s.Fatal("Action failed: ", err)
		}

		// Verify status of all usb-c port.
		for _, portID := range usbcPorts {
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				drpState, err := h.Servo.GetDUTDualRoleState(ctx, portID)
				if err != nil {
					return errors.Wrap(err, "failed to check for USB PD")
				}
				if drpState != step.expectStatus {
					return errors.Errorf(
						"got DRP state %q but expected %q (port %d)",
						drpState, step.expectStatus, portID,
					)
				}
				return nil
				// Poll fails if Timeout <= 10s as commands inside need longer to run.
			}, &testing.PollOptions{Timeout: 15 * time.Second}); err != nil {
				s.Fatal("Failed to poll for DRP state: ", err)
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

func usbPdCloseLid(ctx context.Context, h *firmware.Helper, dut *dutState) error {
	if err := h.Servo.CloseLid(ctx); err != nil {
		return errors.Wrap(err, "failed to close lid")
	}
	dut.lidOpen = false
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

func usbPdOpenLid(ctx context.Context, h *firmware.Helper, dut *dutState) error {
	if err := h.Servo.OpenLid(ctx); err != nil {
		return errors.Wrap(err, "failed to open lid")
	}
	dut.lidOpen = true
	// During boot-up, dut would reach S0 first before getting reconnected.
	testing.ContextLog(ctx, "Checking for S0 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0"); err != nil {
		return errors.Wrap(err, "failed to get power state at S0")
	}
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to DUT")
	}
	return nil
}

func usbPdSuspend(ctx context.Context, h *firmware.Helper, dut *dutState) error {
	testing.ContextLog(ctx, "Suspending DUT")
	if err := h.DUT.Conn().CommandContext(ctx, "powerd_dbus_suspend").Start(); err != nil {
		return errors.Wrap(err, "failed to suspend DUT")
	}
	dut.suspend = true
	testing.ContextLog(ctx, "Checking for S0ix or S3 powerstate")
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, firmware.PowerStateTimeout, "S0ix", "S3"); err != nil {
		return errors.Wrap(err, "failed to get power state at S0ix or S3")
	}
	return nil
}
