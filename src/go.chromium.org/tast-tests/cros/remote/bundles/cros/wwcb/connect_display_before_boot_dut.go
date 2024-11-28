// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wwcb

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/log"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/wwcb/utils/topology"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConnectDisplayBeforeBootDUT,
		Desc:         "Boot DUT with external display already connected, then verify that the DUT can detect the external display",
		Contacts:     []string{"cros-wwcb-automation@google.com", "allion-wwcb@allion.corp-partner.google.com"},
		BugComponent: "b:1289112", // ChromeOS > External > WWCB > Allion > Automation
		Attr:         []string{
			"group:wwcb",
			"group:pasit",
			"group:release-health",
			"release-health_usb",
		},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"servo", "newTestItem"},
		TestBedDeps:  []string{tbdep.ServoStateWorking},
		Data:         []string{"Capabilities.json"},
		Fixture:      "wwcb.dock",
		Params: []testing.Param{
			{
				Name:      "fast",
				ExtraAttr: []string{"pasit_fast"},
			}},
	})
}

func ConnectDisplayBeforeBootDUT(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Set up the servo attached to the DUT.
	dut := s.DUT()
	servoSpec, _ := s.Var("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to the servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	if err := utils.ShutdownDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to shutdown DUT: ", err)
	}
	defer utils.PowerOnDUT(ctx, pxy, dut)

	// Connect the external display via Dock before cold boot DUT.
	tf := s.FixtValue().(*topology.TestFixture)
	if _, err := tf.Helper.ActivateDeviceByType(ctx, topology.DeviceTypeMonitor); err != nil {
		s.Fatal("Failed to connect to the external display: ", err)
	}

	defer func(ctx context.Context) {
		if s.HasError() {
			log.CollectedLogs(ctx, s.DUT(), s.OutDir())
		}
	}(ctx)

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

	if _, ok := s.Var("newTestItem"); ok {
		if err := tf.VerifyDockingInterface(ctx, s.DUT(), s.DataPath("Capabilities.json")); err != nil {
			s.Fatal("Failed to verify the docking station interface: ", err)
		}
	}
}
