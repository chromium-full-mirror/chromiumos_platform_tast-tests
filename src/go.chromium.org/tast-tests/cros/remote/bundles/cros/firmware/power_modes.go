// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/remote/tabletmode"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type powerModeTestParams struct {
	powermode  firmware.ResetType
	tabletmode bool
}

const (
	coldReset firmware.ResetType = "coldreset"
	shutDown  firmware.ResetType = "shutdown"
	warmReset firmware.ResetType = "warmreset"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerModes,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that system comes back after shutdown and coldreset",
		Contacts: []string{
			"chromeos-faft@google.com",
			"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		ServiceDeps:  []string{"tast.cros.ui.ScreenLockService"},
		SoftwareDeps: []string{"chrome", "reboot"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.FormFactor(hwdep.Convertible, hwdep.Chromeslate, hwdep.Detachable)),
		Vars:         []string{"servo"},
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:    []string{"group:firmware", "firmware_unstable"},
		Fixture: fixture.NormalMode,
		Params: []testing.Param{{
			Name:      "coldreset",
			Val:       powerModeTestParams{powermode: coldReset},
			ExtraAttr: []string{"group:intel-nda"},
		}, {
			Name:      "coldreset_tablet_mode",
			Val:       powerModeTestParams{powermode: coldReset, tabletmode: true},
			ExtraAttr: []string{"group:intel-convertible"},
		}, {
			Name:      "shutdown",
			Val:       powerModeTestParams{powermode: shutDown},
			ExtraAttr: []string{"group:intel-nda"},
		}, {
			Name:      "warmreset",
			Val:       powerModeTestParams{powermode: warmReset},
			ExtraAttr: []string{"group:intel-nda"},
		},
		},
	})
}

// PowerModes verifies that system comes back after shutdown and coldreset.
func PowerModes(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	dut := s.DUT()
	testOpt := s.Param().(powerModeTestParams)

	// Servo setup.
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed opening servo: ", err)
	}

	tmc := &tabletmode.ConvertibleModeControl{}
	if err := tmc.InitControl(ctx, dut); err != nil {
		s.Fatal("Failed to init TabletModeControl: ", err)
	}
	if testOpt.tabletmode {
		testing.ContextLog(ctx, "Put DUT into tablet mode")
		if err := tmc.ForceTabletMode(ctx); err != nil {
			s.Fatal("Failed to set DUT into tablet mode: ", err)
		}
	}

	defer func(ctx context.Context) {
		s.Log("Performing Cleanup")
		if !dut.Connected(ctx) {
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
				s.Fatal("Failed to set powerstate to ON at cleanup: ", err)
			}
		}
	}(ctx)

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	screenLockService := ui.NewScreenLockServiceClient(cl.Conn)
	if _, err := screenLockService.NewChrome(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to login chrome: ", err)
	}
	defer screenLockService.CloseChrome(ctx, &empty.Empty{})

	if testOpt.powermode == "coldreset" {
		s.Log("Performing cold reset")
		if err := dut.Conn().CommandContext(ctx, "ectool", "reboot_ec", "cold", "at-shutdown").Run(); err != nil {
			s.Fatal("Failed to execute ectool reboot_ec cmd: ", err)
		}

		if err := dut.Conn().CommandContext(ctx, "shutdown", "-h", "now").Run(); err != nil {
			s.Fatal("Failed to execute shutdown command: ", err)
		}
		if err := dut.WaitConnect(ctx); err != nil {
			s.Fatal("Failed to wake up DUT: ", err)
		}

		if err := powercontrol.ValidatePrevSleepState(ctx, dut, 5); err != nil {
			s.Fatal("Previous Sleep state is not 5: ", err)
		}
	}

	if testOpt.powermode == "shutdown" {
		s.Log("Performing shutdown")
		if err := dut.Conn().CommandContext(ctx, "shutdown", "-h", "now").Run(); err != nil {
			s.Fatal("Failed to run shutdown command: ", err)
		}
		if err := dut.WaitUnreachable(ctx); err != nil {
			s.Fatal("Failed to shutdown DUT: ", err)
		}
		s.Log("Power Normal Pressing")
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
			s.Fatal("Failed to set powerstate to ON: ", err)
		}
		cCtx, cancel := ctxutil.Shorten(ctx, time.Minute)
		defer cancel()
		// Setting power state ON, once again if system fails to boot.
		if err := dut.WaitConnect(cCtx); err != nil {
			if err := h.Servo.SetPowerState(ctx, servo.PowerStateOn); err != nil {
				s.Fatal("Failed to set powerstate to ON: ", err)
			}
			if err := dut.WaitConnect(ctx); err != nil {
				s.Fatal("Failed to wake up DUT: ", err)
			}
		}
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, 5); err != nil {
			s.Fatal("Previous Sleep state is not 5: ", err)
		}
	}

	if testOpt.powermode == "warmreset" {
		s.Log("Performing warm reset")
		if err := h.DUT.Reboot(ctx); err != nil {
			s.Fatal("Failed to reboot DUT: ", err)
		}

		if err := powercontrol.ValidatePrevSleepState(ctx, dut, 0); err != nil {
			s.Fatal("Previous Sleep state is not 0: ", err)
		}
	}
}
