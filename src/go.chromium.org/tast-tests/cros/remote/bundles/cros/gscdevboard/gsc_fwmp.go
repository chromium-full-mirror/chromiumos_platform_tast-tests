// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/utils"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCFWMP,
		Desc:    "Verifies various FWMP enforcement for GSC",
		Timeout: 2 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"jettrink@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly",
			"gsc_smoke"},
		Fixture: fixture.GSCOpenCCD,
	})
}

// GSCFWMP verifies the FWMP can force write protect to be enabled
func GSCFWMP(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s, s.FixtValue().(*fixture.Value))
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	defer i.Close(ctx)

	// Initialize the TPM and remove the FWMP.
	// It is OK if this invocation fails, just making sure FWMP does not
	// exist before verifyWpDisableWithFmp() runs
	_, err := b.ResetAndTpmRemoveFWMP(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
	if err != nil {
		s.Log("init: failed to remove the FWMP: ", err)
	}
	// At the end of the test remove FWMP created by
	// verifyWpDisabledWithFwmp() below.
	defer b.ResetAndTpmRemoveFWMP(ctx, i, ti50.CCDModeOn, ti50.FfClamshell)
	verifyWpDisabledWithFwmp(ctx, s, b, i)
}

func verifyWpDisabledWithFwmp(ctx context.Context, s *testing.State, b utils.DevboardHelper, i *ti50.CrOSImage) {
	s.Log("Verify WP works before FWMP unlock disable")
	_, err := i.Command(ctx, "wp disable atboot")
	if err != nil {
		s.Fatal("Error communicating with ti50: ", err)
	}

	b.CheckWriteProtect(ctx, utils.WPDisabled, "after `wp disable` console command")

	_, err = b.ResetAndTpmSetFWMP(ctx, i, utils.FWMPDisableUnlock,
		ti50.CCDModeOn, ti50.FfClamshell)
	if err != nil {
		s.Fatal("Failed to create FWMP: ", err)
	}

	b.CheckWriteProtect(ctx, utils.WPEnabled, "after writing FWMP disable unlock")

	// Type the "wp disable" command again; this should have no affect because of FWMP
	_, err = i.Command(ctx, "wp disable atboot")
	if err != nil {
		s.Fatal("Error communicating with ti50: ", err)
	}

	b.CheckWriteProtect(ctx, utils.WPEnabled, "after `wp disable` command is blocked")

	// Verify FWMP force enable doesn't lead to multiple AP RO verification
	// reboots. This is mostly for Ti50. It also shouldn't trigger resets on
	// Cr50.
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL)
	// Read from the gpio monitor to clear existing events.
	events := b.GpioMonitorRead(ctx, gpioMonitor)
	s.Log("Cleared Events: ", events)
	for attempt := 0; attempt < 5; attempt++ {
		// Toggling PLT_RST_L on ti50 should trigger AP RO verification
		// once since WP was just enabled.
		b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
		b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)

		if err = i.CommandImage.WaitUntilBooted(ctx, 8*time.Second); err != nil {
			s.Fatalf("GSC unresponsive after %d PLT_RST_L pulses", attempt)
		}

		// Check if GSC reset or the WP signal toggled.
		ecRstChanged := fwmpCheckEcRst(ctx, b, gpioMonitor)
		s.Logf("reset %d: ecRstChanged %d", attempt, ecRstChanged)
		gscReset := ecRstChanged != 0
		if attempt == 0 && b.TestbedType != ti50.GscH1Shield {
			if !gscReset {
				s.Errorf("plt_rst %d: Ti50 did not reset to do AP RO verification", attempt)
			}
		} else if gscReset {
			s.Errorf("plt_rst %d: detected GSC reset EC_RST_L changed %d times", attempt, ecRstChanged)
		}
	}
}

// fwmpCheckEcRst returns the number of times EC_RST_L changed state. This
// clears the GPIO monitor events.
func fwmpCheckEcRst(ctx context.Context, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession) int {
	events := b.GpioMonitorWait(ctx, gpioMonitor, 2*time.Second, 100*time.Millisecond)
	ecRstChanged := 0
	testing.ContextLog(ctx, "Events:")
	for _, event := range events.Sorted {
		testing.ContextLogf(ctx, "%+v", event)
		if event.Name == ti50.GpioTi50EcRstL {
			ecRstChanged++
		}
	}
	// EC_RST_L pulses signal that GSC reset.
	return ecRstChanged
}
