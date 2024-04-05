// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	// TODO(b/238157101): We are not running this test on any bots intentionally.
	// ambalavanan.m.m@intel.com needs to add this test as part of a new suite.
	testing.AddTest(&testing.Test{
		Func:         SDCardPlugUnplugDuringSleep,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies microSD card functionality with plug and unplug during sleep",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome"},
		VarDeps:      []string{"servo", "intel.sdCardDetectionName"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.X86()),
		Attr:         []string{"group:intel-usb-set1"},
		Params: []testing.Param{{
			Name:    "plug",
			Val:     true,
			Timeout: 5 * time.Minute,
		}, {
			Name:    "unplug",
			Val:     false,
			Timeout: 5 * time.Minute,
		},
		}})
}

// SDCardPlugUnplugDuringSleep checks microSD card functionality with plug
// and unplug during sleep.
// Pre-requisite: SD card must be inserted into servo microSD card slot.
func SDCardPlugUnplugDuringSleep(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	dut := s.DUT()

	sdCardName := s.RequiredVar("intel.sdCardDetectionName")
	servoSpec := s.RequiredVar("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Error("Failed to power-on DUT at cleanup: ", err)
			}
		}
		if err := utils.UnplugSDCardViaServo(ctx, pxy); err != nil {
			s.Error("Failed to unplug microSD card as cleanup: ", err)
		}
	}(cleanupCtx)

	// Performs Chrome login.
	if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
		s.Fatal("Failed to login Chrome: ", err)
	}

	slpOpSetPre, pkgOpSetPre, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get SLP counter and C10 package values before suspend-resume: ", err)
	}

	if err := utils.PlugSDCardViaServo(ctx, pxy); err != nil {
		s.Fatal("Failed to plug microSD storage device to DUT: ", err)
	}

	if err := utils.WaitForSDCardDetection(ctx, dut, sdCardName); err != nil {
		s.Fatal("Failed to wait for microSD card detection: ", err)
	}

	powerOffCtx, cancel := context.WithTimeout(ctx, 10*time.Second)
	defer cancel()
	cmd := dut.Conn().CommandContext(powerOffCtx, "powerd_dbus_suspend")
	if err := cmd.Start(); err != nil {
		s.Fatal("Failed to suspend DUT with powerd_dbus_suspend command: ", err)
	}

	sdCtx, cancel := context.WithTimeout(ctx, 40*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		s.Fatal("Failed to wait for unreachable: ", err)
	}

	// Plug/unplug microSD storage device while DUT in sleep.
	isPlug := s.Param().(bool)
	if isPlug {
		s.Log("Plugging microSD after suspend")
		if err := utils.PlugSDCardViaServo(ctx, pxy); err != nil {
			s.Fatal("Failed to plug microSD storage device to DUT: ", err)
		}
	} else {
		s.Log("Unplugging microSD after suspend")
		if err := utils.UnplugSDCardViaServo(ctx, pxy); err != nil {
			s.Fatal("Failed to unplug microSD storage device from DUT: ", err)
		}
	}

	sdCtx, cancel = context.WithTimeout(ctx, 20*time.Second)
	defer cancel()
	if err := dut.WaitUnreachable(sdCtx); err != nil {
		s.Fatal("Failed: DUT is not unreachable after plug/unplug of microSD: ", err)
	}

	s.Log("Waking DUT with power button normal press via servo")
	if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
		s.Fatal("Failed to wake DUT at cleanup: ", err)
	}

	slpOpSetPost, pkgOpSetPost, err := powercontrol.SlpAndC10PackageValues(ctx, dut)
	if err != nil {
		s.Fatal("Failed to get SLP counter and C10 package values after lid-open: ", err)
	}

	if slpOpSetPre == slpOpSetPost {
		s.Fatalf("Failed: SLP counter value %v should be different from the one before %v", slpOpSetPost, slpOpSetPre)
	}
	if slpOpSetPost == 0 {
		s.Fatal("Failed: SLP counter value must be non-zero, got: ", slpOpSetPost)
	}

	if pkgOpSetPre == pkgOpSetPost {
		s.Fatalf("Failed: Package C10 value %q must be different from the one before %q", pkgOpSetPost, pkgOpSetPre)
	}
	if pkgOpSetPost == "0x0" || pkgOpSetPost == "0" {
		s.Fatal("Failed: Package C10 should be non-zero, got: ", pkgOpSetPost)
	}

	if isPlug {
		if err := utils.WaitForSDCardDetection(ctx, dut, sdCardName); err != nil {
			s.Fatal("Failed to wait for microSD card detection after plug: ", err)
		}
	}
}
