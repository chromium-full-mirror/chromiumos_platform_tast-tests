// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast-tests/cros/remote/tabletmode"
	"go.chromium.org/tast-tests/cros/services/cros/security"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	cbmemSleepStateValue = 5
)

type shutdownWithCmdTabletModeParams struct {
	control tabletmode.Control
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShutdownWithCommandTabletMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that system comes back after executing shutdown command in tabletmode",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome", "reboot"},
		Attr:         []string{"group:mainline", "informational"},
		Vars:         []string{"servo"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.SkipOnModel("nautilus", "nautiluslte", "soraka", "pantheon", "nocturne", "kodama")),
		Params: []testing.Param{{
			Name:              "convertible",
			Val:               shutdownWithCmdTabletModeParams{control: &tabletmode.ConvertibleModeControl{}},
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible)),
			ExtraAttr:         []string{"group:intel-convertible"},
		}, {
			Name:              "detachable",
			Val:               shutdownWithCmdTabletModeParams{control: &tabletmode.DetachableModeControl{}},
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Detachable)),
		}},
		Timeout: 10 * time.Minute,
	})
}

func ShutdownWithCommandTabletMode(ctx context.Context, s *testing.State) {
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctxForCleanUp)

	tmc := s.Param().(shutdownWithCmdTabletModeParams).control
	if err := tmc.InitControl(ctx, dut); err != nil {
		s.Fatal("Failed to init TabletModeControl: ", err)
	}
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Resetting tabletmode")
		if err := tmc.Reset(ctx); err != nil {
			s.Fatal("Failed to restore tabletmode to the original settings: ", err)
		}
	}(ctxForCleanUp)

	// Force DUT into tablet mode.
	testing.ContextLog(ctx, "Put DUT into tablet mode")
	if err := tmc.ForceTabletMode(ctx); err != nil {
		s.Fatal("Failed to set DUT into tablet mode: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctxForCleanUp)

	client := security.NewBootLockboxServiceClient(cl.Conn)
	if _, err := client.NewChromeLogin(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to wake up DUT at cleanup: ", err)
			}
		}
	}(ctxForCleanUp)

	powerOffCtx, cancel := context.WithTimeout(ctx, 3*time.Second)
	defer cancel()
	if err := dut.Conn().CommandContext(powerOffCtx, "shutdown", "-h", "now").Run(); err != nil && !errors.Is(err, context.DeadlineExceeded) {
		s.Fatal("Failed to execute shutdown command: ", err)
	}

	sdCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		s.Fatal("Failed to shutdown DUT: ", err)
	}

	if err := powercontrol.ValidateG3PowerState(ctx, pxy); err != nil {
		s.Fatal("Failed to enter G3 after shutdown: ", err)
	}

	if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to wake up DUT: ", err)
	}

	valid, err := powercontrol.IsPrevSleepStateAvailable(ctx, dut)

	if err != nil {
		s.Fatal("Failed to determine if prev_sleep_state is available: ", err)
	}

	if valid {
		if err := powercontrol.ValidatePrevSleepState(ctx, dut, cbmemSleepStateValue); err != nil {
			s.Fatalf("Failed Previous Sleep state is not %v: %v", cbmemSleepStateValue, err)
		}
	}
}
