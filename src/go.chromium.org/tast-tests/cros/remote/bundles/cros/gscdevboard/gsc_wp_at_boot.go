// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package gscdevboard

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/ti50"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/gscdevboard/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware/ti50/fixture"
	"go.chromium.org/tast/core/testing"
)

type testWPAtBootConfig struct {
	action         string
	wpSetting      string
	fwmp           bool
	followBattPres bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:    GSCWPAtBoot,
		Desc:    "Verifies the WP setting is correctly setup after different types of resets",
		Timeout: 2 * time.Minute,
		Contacts: []string{
			"cros-hwsec@google.com", // CrOS GSC Developers
			"mruthven@chromium.org", // Test Author
		},
		BugComponent: "b:715469", // ChromeOS > Platform > System > Hardware Security > HwSec GSC > Ti50
		Attr: []string{"group:gsc",
			"gsc_dt_ab", "gsc_dt_shield", "gsc_h1_shield", "gsc_ot_shield", "gsc_ot_fpga_cw310",
			"gsc_image_ti50",
			"gsc_nightly"},
		Fixture: fixture.GSCOpenCCD,
		Params: []testing.Param{{
			Name: "plt_rst_follow_batt_pres_wp_disabled",
			Val: testWPAtBootConfig{
				action:         "plt_rst",
				wpSetting:      "disable",
				followBattPres: true,
			},
		}, {
			Name: "plt_rst_follow_batt_pres_wp_enabled",
			Val: testWPAtBootConfig{
				action:         "plt_rst",
				wpSetting:      "enable",
				followBattPres: true,
			},
		}, {
			Name: "plt_rst_fwmp_wp_disable",
			Val: testWPAtBootConfig{
				action:    "plt_rst",
				wpSetting: "disable",
				fwmp:      true,
			},
		}, {
			Name: "plt_rst_fwmp_wp_enable",
			Val: testWPAtBootConfig{
				action:    "plt_rst",
				wpSetting: "enable",
				fwmp:      true,
			},
		}, {
			Name: "plt_rst_fwmp_follow_batt_pres",
			Val: testWPAtBootConfig{
				action:         "plt_rst",
				wpSetting:      "disable",
				fwmp:           true,
				followBattPres: true,
			},
		}, {
			Name: "plt_rst_wp_disabled",
			Val: testWPAtBootConfig{
				action:    "plt_rst",
				wpSetting: "disable",
			},
		}, {
			Name: "plt_rst_wp_enabled",
			Val: testWPAtBootConfig{
				action:    "plt_rst",
				wpSetting: "enable",
			},
		}, {
			Name: "reboot_cmd_follow_batt_pres_wp_disabled",
			Val: testWPAtBootConfig{
				action:         "reboot",
				wpSetting:      "disable",
				followBattPres: true,
			},
		}, {
			Name: "reboot_cmd_follow_batt_pres_wp_enabled",
			Val: testWPAtBootConfig{
				action:         "reboot",
				wpSetting:      "enable",
				followBattPres: true,
			},
		}, {
			Name: "reboot_cmd_fwmp_follow_batt_pres",
			Val: testWPAtBootConfig{
				action:         "reboot",
				wpSetting:      "disable",
				fwmp:           true,
				followBattPres: true,
			},
		}, {
			Name: "reboot_cmd_fwmp_wp_disable",
			Val: testWPAtBootConfig{
				action:    "reboot",
				wpSetting: "disable",
				fwmp:      true,
			},
		}, {
			Name: "reboot_cmd_fwmp_wp_enable",
			Val: testWPAtBootConfig{
				action:    "reboot",
				wpSetting: "enable",
				fwmp:      true,
			},
		}, {
			Name: "reboot_cmd_wp_disabled",
			Val: testWPAtBootConfig{
				action:    "reboot",
				wpSetting: "disable",
			},
		}, {
			Name: "reboot_cmd_wp_enabled",
			Val: testWPAtBootConfig{
				action:    "reboot",
				wpSetting: "enable",
			},
		}, {
			Name: "wp_cmd_fwmp_wp_disabled",
			Val: testWPAtBootConfig{
				action:    "wp command",
				wpSetting: "disable",
				fwmp:      true,
			},
		}, {
			Name: "wp_cmd_fwmp_wp_enabled",
			Val: testWPAtBootConfig{
				action:    "wp command",
				wpSetting: "enable",
				fwmp:      true,
			},
		}, {
			Name: "wp_cmd_wp_disabled",
			Val: testWPAtBootConfig{
				action:    "wp command",
				wpSetting: "disable",
			},
		}, {
			Name: "wp_cmd_wp_enabled",
			Val: testWPAtBootConfig{
				action:    "wp command",
				wpSetting: "enable",
			},
		}},
	})
}

// GSCWPAtBoot verifies WP atboot
func GSCWPAtBoot(ctx context.Context, s *testing.State) {
	b := utils.NewDevboardHelper(s)
	i := ti50.MustOpenCrOSImage(ctx, b, s, b.TestbedType)
	th := utils.FirmwareTestingHelper{FirmwareTestingHelperDelegate: s}
	defer i.Close(ctx)

	config := s.Param().(testWPAtBootConfig)
	action := config.action
	wpSetting := config.wpSetting
	wpEnabledPreFwmp := wpSetting != "disable"
	wpEnabled := wpEnabledPreFwmp || config.fwmp
	if config.followBattPres {
		wpSetting = "follow_batt_pres"
	}

	var bootWpThreshold int
	if b.TestbedType == ti50.GscH1Shield {
		if wpEnabled {
			bootWpThreshold = 0
		} else {
			// When WP is disabled, Cr50 briefly enables it when it
			// reboots.
			bootWpThreshold = 2
		}
	} else {
		// Ti50 runs verification at boot. It enables WP during
		// verification. It's ok if Ti50 pulses WP during reset.
		bootWpThreshold = 2
	}

	s.Log("Verify WP")
	s.Log("action: ", wpSetting)
	s.Log("wpSetting: ", wpSetting)
	s.Log("fwmp: ", config.fwmp)
	s.Log("expect WP enabled: ", wpEnabled)
	// Release the GSC from reset.
	b.ResetWithStraps(ctx, ti50.FfClamshell, ti50.CCDModeOff)
	th.MustSucceed(i.WaitUntilBooted(ctx), "gsc failed to boot")

	// Startup GPIO monitoring. Clear events
	gpioMonitor := b.GpioMonitorStart(ctx, ti50.GpioTi50EcRstL, ti50.GpioTi50WriteProtectL)
	events := b.GpioMonitorRead(ctx, gpioMonitor)
	s.Log("Cleared Events: ", events)

	wpCmd := fmt.Sprintf("wp %s atboot", wpSetting)
	_, err := i.Command(ctx, wpCmd)
	th.MustSucceed(err, "Failed "+wpCmd)

	// Setup battery presence
	if config.followBattPres {
		var bpSetting string
		if !wpEnabledPreFwmp {
			bpSetting = "dis"
		}
		bpCmd := fmt.Sprintf("bp %sconnected atboot", bpSetting)
		_, err := i.Command(ctx, bpCmd)
		th.MustSucceed(err, "Failed "+bpCmd)
		bp, err := i.Command(ctx, "bp")
		th.MustSucceed(err, "Failed to get batt pres")
		s.Log("BP: ", bp)
	}

	// Wait for the WP change
	_, wpChanged := wpAtBootCheckSignals(ctx, b, gpioMonitor)
	s.Log("WP events: ", wpChanged)

	// Remove the FWMP at the end of the test.
	defer b.ResetAndTpmRemoveFWMP(ctx, i, ti50.FfClamshell)
	// Create the FWMP and verify WP.
	if config.fwmp {
		// Verify the WP setting before creating the fwmp
		if wpEnabledPreFwmp == b.GpioGet(ctx, ti50.GpioTi50WriteProtectL) {
			s.Fatal("Failed to setup initial WP")
		}

		_, err = b.ResetAndTpmSetFWMP(ctx, i, utils.FWMPDisableUnlock,
			ti50.CCDModeOff, ti50.FfClamshell)
		th.MustSucceed(err, "Failed to create FWMP")
	}

	wp, err := i.Command(ctx, "wp")
	th.MustSucceed(err, "Failed to get write protect")
	s.Log("WP: ", wp)

	wpIsEnabled := !b.GpioGet(ctx, ti50.GpioTi50WriteProtectL)
	// Verify the WP setting
	if wpEnabled != wpIsEnabled {
		s.Fatal("Failed to setup WP")
	}
	s.Logf("Setup WP: %t", wpIsEnabled)

	// Read from the gpio monitor to clear existing events.
	events = b.GpioMonitorRead(ctx, gpioMonitor)
	s.Log("Cleared Events: ", events)

	for attempt := 0; attempt < 5; attempt++ {
		var expectReboot bool
		switch action {
		case "plt_rst":
			// Ti50 reboots to run AP RO verification on the first
			// PLT_RST_L pulse after WP is enabled. If WP was
			// enabled before creating the FWMP, then APRO
			// verification was already run. It shouldn't get
			// triggered again.
			expectReboot = (b.TestbedType != ti50.GscH1Shield &&
				attempt == 0 && wpEnabled &&
				!(config.fwmp && wpEnabledPreFwmp))
			// Toggling PLT_RST_L on ti50 should trigger AP RO verification
			// once since WP was just enabled.
			b.GpioSet(ctx, ti50.GpioTi50PltRstL, true)
			b.GpioSet(ctx, ti50.GpioTi50PltRstL, false)
			err = i.CommandImage.WaitUntilBooted(ctx, 8*time.Second)
			th.MustSucceed(err, "GSC unresponsive after PLT_RST_L")
		case "reboot":
			expectReboot = true
			th.MustSucceed(i.Reboot(ctx), "Failed to reboot GSC")
		case "wp command":
			expectReboot = false
			_, err := i.Command(ctx, "wp "+wpSetting)
			th.MustSucceed(err, "Failed wp "+wpSetting)
		}

		// Verify the WP setting
		if wpEnabled == b.GpioGet(ctx, ti50.GpioTi50WriteProtectL) {
			s.Fatalf("%s: wp changed", action)
		}

		// Check if GSC reset or the WP signal toggled.
		resetDetected, wpChanged := wpAtBootCheckSignals(ctx, b, gpioMonitor)
		s.Logf("%s %d: wpChanged %d", action, attempt, wpChanged)
		s.Logf("%s %d: resetDetected %t", action, attempt, resetDetected)
		if expectReboot {
			if !resetDetected {
				s.Errorf("%s %d: did not detect GSC reset", action, attempt)
			}
			if wpChanged > bootWpThreshold {
				s.Errorf("%s %d: WP changed %d times", action, attempt, wpChanged)
			}
		} else {
			if resetDetected {
				s.Errorf("%s %d: unexpected reboot", action, attempt)
			}
			if wpChanged > 0 {
				s.Errorf("%s %d: WP changed %d times", action, attempt, wpChanged)
			}

		}
	}
}

// wpAtBootCheckSignals returns the number of times EC_RST_L and WP_L changed state
func wpAtBootCheckSignals(ctx context.Context, b utils.DevboardHelper, gpioMonitor utils.GpioMonitorSession) (bool, int) {
	events := b.GpioMonitorWait(ctx, gpioMonitor, 2*time.Second, 100*time.Millisecond)
	ecRstChanged := 0
	wpChanged := 0
	testing.ContextLog(ctx, "Events:")
	for _, event := range events.Sorted {
		testing.ContextLogf(ctx, "%+v", event)
		if event.Name == ti50.GpioTi50EcRstL {
			ecRstChanged++
		}
		if event.Name == ti50.GpioTi50WriteProtectL {
			wpChanged++
		}
	}
	// EC_RST_L pulses signal that GSC reset.
	// WP_L changes mean GSC is changing the WP state.
	return ecRstChanged != 0, wpChanged
}
