// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type displayTestParams struct {
	tabletMode  bool
	displayType string
	cswitchPort string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisplayDetection,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies external display detection",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Vars: []string{
			"intel.iterations",
			"intel.cSwitchPort",
			"intel.domainIP",
		},
		// To skip on duffy(Chromebox) with no internal display.
		HardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		Attr:         []string{"group:intel-cswitch-set1"},
		Params: []testing.Param{{
			Name:    "dp_clamshell_mode",
			Fixture: "chromeLoggedIn",
			Val: displayTestParams{
				tabletMode: false,
				// The Type-C DP is connected to C-Switch in P2 as per the intel_cswitch_set1 suite setup.
				cswitchPort: "2",
				displayType: usbutils.TypeCDP,
			},
			Timeout: 10 * time.Minute,
		}, {
			Name:    "dp_tablet_mode",
			Fixture: "chromeLoggedIn",
			Val: displayTestParams{
				tabletMode: true,
				// The Type-C DP is connected to C-Switch in P2 as per the intel_cswitch_set1 suite setup.
				cswitchPort: "2",
				displayType: usbutils.TypeCDP,
			},
			Timeout: 10 * time.Minute,
		}, {
			Name:    "hdmi_clamshell_mode",
			Fixture: "chromeLoggedIn",
			Val: displayTestParams{
				tabletMode: false,
				// The Type-C HDMI is connected to C-Switch in P1 as per the intel_cswitch_set1 suite setup.
				cswitchPort: "1",
				displayType: usbutils.TypeCHDMI,
			},
			Timeout: 10 * time.Minute,
		}, {
			Name:    "hdmi_tablet_mode",
			Fixture: "chromeLoggedIn",
			Val: displayTestParams{
				tabletMode: true,
				// The Type-C HDMI is connected to C-Switch in P1 as per the intel_cswitch_set1 suite setup.
				cswitchPort: "1",
				displayType: usbutils.TypeCHDMI,
			},
			Timeout: 10 * time.Minute,
		}},
	})
}

func DisplayDetection(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	testOpt := s.Param().(displayTestParams)

	iterCount := 2 // Use default iteration 2 to plug-unplug external display.
	if iter, ok := s.Var("intel.iterations"); ok {
		if iterCount, err = strconv.Atoi(iter); err != nil {
			s.Fatalf("Failed to parse iteration value %q: %v", iter, err)
		}
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	if testOpt.tabletMode {
		cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, true)
		if err != nil {
			s.Fatal("Failed to enable tablet mode: ", err)
		}
		defer cleanup(cleanupCtx)
	}

	cswitchVar := testOpt.cswitchPort
	if cswitchON, ok := s.Var("intel.cSwitchPort"); ok {
		cswitchVar = cswitchON
	}

	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("intel.domainIP")

	// Create C-Switch session that performs hot plug-unplug on USB4 device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create session: ", err)
	}
	defer func(ctx context.Context) {
		if err := cswitch.CloseSession(ctx, sessionID, domainIP); err != nil {
			s.Log("Failed to close session: ", err)
		}
	}(cleanupCtx)

	for i := 1; i <= iterCount; i++ {
		s.Logf("Iteration: %d/%d", i, iterCount)
		s.Log("Plugging external display to DUT")
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cswitchVar, domainIP); err != nil {
			s.Fatal("Failed to enable c-switch port: ", err)
		}
		numberOfDisplay := 1
		spec := usbutils.DisplaySpec{
			NumberOfDisplays: &numberOfDisplay,
			DisplayType:      testOpt.displayType,
		}
		if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
			s.Fatal("Failed to check for connected external HDMI display: ", err)
		}
		const cSwitchOFF = "0"
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)
			numberOfDisplay := 0
			spec := usbutils.DisplaySpec{NumberOfDisplays: &numberOfDisplay}
			if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
				s.Fatal("Failed to wait for display to be unplugged: ", err)
			}
		}
	}
}
