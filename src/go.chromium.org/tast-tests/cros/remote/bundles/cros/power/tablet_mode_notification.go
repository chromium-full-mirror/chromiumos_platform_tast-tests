// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/tabletmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type tabletModeConfig struct {
	control tabletmode.Control
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TabletModeNotification,
		Desc:         "Verifies the EC's tablet mode change notification is received by the AP (powerd)",
		BugComponent: "b:1361410", // ChromeOS > Platform > Enablement > Power
		Contacts:     []string{"chromeos-power-team@google.com", "timvp@google.com", "cros-fw-engprod@google.com"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		SoftwareDeps: []string{"chrome"},
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:mainline", "informational", "group:firmware", "firmware_bios", "firmware_enabled", "firmware_meets_kpi", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw", "firmware_ec", "firmware_ec_ro", "firmware_ec_rw"},
		Timeout:      5 * time.Minute,
		// Restrict boards that don't support any method in the tabletmode package for forcing tabletmode.
		HardwareDeps: hwdep.D(hwdep.ChromeEC(),
			hwdep.SkipOnModel(
				"akali360", // nami: EC FW doesn't support 'ectool motionsense tablet_mode_angle' command
				"kodama",
				"nautilus",
				"nautiluslte",
				"nocturne",
				"pantheon",
				"robo360", // coral: EC FW doesn't support 'ectool motionsense tablet_mode_angle' command
				"soraka",
			)),
		Params: []testing.Param{{
			Name:              "convertible",
			Val:               tabletModeConfig{control: &tabletmode.ConvertibleModeControl{}},
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible)),
			ExtraAttr: []string{
				"group:release-health",
				"release-health_power",
			},
		}, {
			Name:              "detachable",
			Val:               tabletModeConfig{control: &tabletmode.DetachableModeControl{}},
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Detachable)),
			ExtraAttr: []string{
				"group:release-health",
				"release-health_power",
			},
		}},
	})
}

// isDUTInTabletMode returns true if the DUT is in tablet mode, false otherwise.
func isDUTInTabletMode(ctx context.Context, s *testing.State, dut *dut.DUT) bool {
	// Verify powerd received the tablet mode change notification.
	testing.ContextLog(ctx, "Get the tabletmode value from powerd")
	// Expected output from the `dbus-send` command is:
	//    boolean true
	// OR
	//    boolean false
	out, err := dut.Conn().CommandContext(ctx, "dbus-send", "--system", "--print-reply=literal", "--type=method_call", "--dest=org.chromium.PowerManager", "/org/chromium/PowerManager", "org.chromium.PowerManager.GetTabletMode").Output()
	if err != nil {
		s.Fatal("Failed to retrieve dbus-send output: ", err)
	}
	words := strings.Fields(string(out))
	if len(words) != 2 {
		s.Fatal("Received unexpected output from dbus-send: ", out)
	}

	testing.ContextLog(ctx, "powerd tabletmode: ", words[1])
	return words[1] == "true"
}

// verifyPowerdTogglesTabletMode waits for powerd to toggle tabletmode and verifies it's the correct
// value. Returns an error otherwise.
func verifyPowerdTogglesTabletMode(ctx context.Context, s *testing.State, dut *dut.DUT, expectedMode bool) {
	s.Log("Verify powerd received the tablet mode change notification to: ", expectedMode)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		isTabletMode := isDUTInTabletMode(ctx, s, dut)
		if isTabletMode != expectedMode {
			return errors.Errorf("powerd failed to toggle tabletmode: isTabletMode=%t", isTabletMode)
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 250 * time.Millisecond}); err != nil {
		s.Fatal("Failed to get tablet mode status: ", err)
	}
}

func TabletModeNotification(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	dut := s.DUT()
	tmc := s.Param().(tabletModeConfig).control
	if err := tmc.InitControl(ctx, dut); err != nil {
		s.Fatal("Failed to init TabletModeControl: ", err)
	}
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Performing cleanup")
		if err := tmc.Reset(ctx); err != nil {
			s.Fatal("Failed to restore tabletmode to the original settings: ", err)
		}
	}(cleanupCtx)

	testing.ContextLog(ctx, "Enable tablet mode")
	if err := tmc.ForceTabletMode(ctx); err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	// Give powerd time to process the notification and switch the DUT to tablet mode.
	verifyPowerdTogglesTabletMode(ctx, s, dut, true)

	// GoBigSleepLint: We aren't attempting to stress test the tablet mode switching
	// capabilities, so give powerd time to finish switching the DUT's state before
	// toggling it again.
	testing.Sleep(ctx, 1*time.Second)

	testing.ContextLog(ctx, "Disable tablet mode")
	if err := tmc.ForceLaptopMode(ctx); err != nil {
		s.Fatal("Failed to disable tablet mode: ", err)
	}
	// Give powerd time to process the notification and switch the DUT to tablet mode.
	verifyPowerdTogglesTabletMode(ctx, s, dut, false)

	// GoBigSleepLint: We aren't attempting to stress test the tablet mode switching
	// capabilities, so give powerd time to finish switching the DUT's state before
	// toggling it again.
	testing.Sleep(ctx, 1*time.Second)

	testing.ContextLog(ctx, "Enable tablet mode")
	if err := tmc.ForceTabletMode(ctx); err != nil {
		s.Fatal("Failed to enable tablet mode: ", err)
	}
	// Give powerd time to process the notification and switch the DUT to tablet mode.
	verifyPowerdTogglesTabletMode(ctx, s, dut, true)
}
