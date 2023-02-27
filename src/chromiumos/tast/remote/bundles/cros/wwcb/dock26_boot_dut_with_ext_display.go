// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"chromiumos/tast/common/servo"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/bundles/cros/wwcb/utils"
	"chromiumos/tast/remote/powercontrol"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Dock26BootDUTWithExtDisplay,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Boot DUT with external display already connected via dock, then verify that the DUT can detect the external display",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{"group:wwcb"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "DockingID", "ExtDispID1", "wwcbIPPowerIp"},
	})
}

func Dock26BootDUTWithExtDisplay(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	dockingID := s.RequiredVar("DockingID")
	extDispID := s.RequiredVar("ExtDispID1")

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to the servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	shutdownPowerState := "S5"
	if err := powercontrol.ShutdownAndWaitForPowerState(ctx, pxy, dut, shutdownPowerState); err != nil {
		s.Fatalf("Failed to shutdown and wait for %q powerstate: %v", shutdownPowerState, err)
	}

	// Open IP power to supply docking power.
	if err := utils.OpenIppower(ctx, []int{1}); err != nil {
		s.Fatal("Failed to open IP power: ", err)
	}
	defer utils.CloseIppower(cleanupCtx, []int{1})

	// Initialize fixtures to find the connected devices.
	if err := utils.InitFixture(ctx); err != nil {
		s.Fatal("Failed to initialize fixtures: ", err)
	}
	defer utils.CloseAllFixture(cleanupCtx)

	// Connect the external display via Dock before cold boot DUT.
	if err := utils.ControlFixture(ctx, extDispID, "on"); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}
	if err := utils.ControlFixture(ctx, dockingID, "on"); err != nil {
		s.Fatal("Failed to connect to the docking station: ", err)
	}

	waitCtx, cancel := context.WithTimeout(ctx, 120*time.Second)
	defer cancel()
	if err := pxy.Servo().KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
		s.Fatal("Failed to press the power button of the DUT: ", err)
	}
	if err := dut.WaitConnect(waitCtx); err != nil {
		s.Fatal("Failed to wait connect to the DUT: ", err)
	}

	if err := utils.VerifyDisplayCount(ctx, dut, 2); err != nil {
		s.Fatal("Failed to verify that the external display is connected: ", err)
	}
}
