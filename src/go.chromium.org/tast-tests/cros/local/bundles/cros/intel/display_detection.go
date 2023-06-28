// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/cswitch"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type displayTestParams struct {
	tabletMode    bool
	displayInfoRe map[string]*regexp.Regexp
	cswitchPort   string
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
		Params: []testing.Param{{
			Name:    "dp_clamshell_mode",
			Fixture: "chromeLoggedIn",
			Val: displayTestParams{
				tabletMode: false,
				// The Type-C DP is connected to C-Switch in P2 as per the intel_cswitch_set1 suite setup.
				cswitchPort: "2",
				displayInfoRe: map[string]*regexp.Regexp{
					"connectorInfoPtrns": regexp.MustCompile(`.*: connectors:\n.\s+\[CONNECTOR:\d+:[DP]+.*`),
					"connectedPtrns":     regexp.MustCompile(`\[CONNECTOR:\d+:DP.*status: connected`),
					"modesPtrns":         regexp.MustCompile(`modes:\n.*"\d+x\d+":.60`),
				},
			},
			Timeout: 10 * time.Minute,
		}, {
			Name:    "dp_tablet_mode",
			Fixture: "chromeLoggedIn",
			Val: displayTestParams{
				tabletMode: true,
				// The Type-C DP is connected to C-Switch in P2 as per the intel_cswitch_set1 suite setup.
				cswitchPort: "2",
				displayInfoRe: map[string]*regexp.Regexp{
					"connectorInfoPtrns": regexp.MustCompile(`.*: connectors:\n.\s+\[CONNECTOR:\d+:[DP]+.*`),
					"connectedPtrns":     regexp.MustCompile(`\[CONNECTOR:\d+:DP.*status: connected`),
					"modesPtrns":         regexp.MustCompile(`modes:\n.*"\d+x\d+":.60`),
				},
			},
			Timeout: 10 * time.Minute,
		}, {
			Name:    "hdmi_clamshell_mode",
			Fixture: "chromeLoggedIn",
			Val: displayTestParams{
				tabletMode: false,
				// The Type-C HDMI is connected to C-Switch in P1 as per the intel_cswitch_set1 suite setup.
				cswitchPort: "1",
				displayInfoRe: map[string]*regexp.Regexp{
					"connectorInfoPtrns": regexp.MustCompile(`.*: connectors:\n.\s+\[CONNECTOR:\d+:[HDMI]+.*`),
					"connectedPtrns":     regexp.MustCompile(`.*DP branch device present.*yes\n.*Type.*HDMI`),
					"modesPtrns":         regexp.MustCompile(`modes:\n.*"\d+x\d+":.60`),
				},
			},
			Timeout: 10 * time.Minute,
		}, {
			Name:    "hdmi_tablet_mode",
			Fixture: "chromeLoggedIn",
			Val: displayTestParams{
				tabletMode: true,
				// The Type-C HDMI is connected to C-Switch in P1 as per the intel_cswitch_set1 suite setup.
				cswitchPort: "1",
				displayInfoRe: map[string]*regexp.Regexp{
					"connectorInfoPtrns": regexp.MustCompile(`.*: connectors:\n.\s+\[CONNECTOR:\d+:[HDMI]+.*`),
					"connectedPtrns":     regexp.MustCompile(`.*DP branch device present.*yes\n.*Type.*HDMI`),
					"modesPtrns":         regexp.MustCompile(`modes:\n.*"\d+x\d+":.60`),
				},
			},
			Timeout: 10 * time.Minute,
		}},
	})
}

func DisplayDetection(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)
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

		displayInfoPatterns := []*regexp.Regexp{
			testOpt.displayInfoRe["connectorInfoPtrns"],
			testOpt.displayInfoRe["connectedPtrns"],
			testOpt.displayInfoRe["modesPtrns"],
		}
		if err := waitForExternalMonitorCount(ctx, 1, displayInfoPatterns); err != nil {
			s.Fatal("Failed connecting external display: ", err)
		}
		const cSwitchOFF = "0"
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			s.Fatal("Failed to disable c-switch port: ", err)

			if err := waitForExternalMonitorCount(ctx, 0, nil); err != nil {
				s.Fatal("Failed unplugging external display: ", err)
			}
		}
	}
}

// waitForExternalMonitorCount verifies for the connected numberOfDisplays.
func waitForExternalMonitorCount(ctx context.Context, numberOfDisplays int, regexpPatterns []*regexp.Regexp) error {
	const DisplayInfoFile = "/sys/kernel/debug/dri/0/i915_display_info"
	// This regexp will skip pipe A since that's the internal display detection.
	displayInfo := regexp.MustCompile(`.*pipe\s+[BCD]\]:\n.*active=yes, mode=.[0-9]+x[0-9]+.: [0-9]+.*\s+[hw: active=yes]+`)

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := testexec.CommandContext(ctx, "cat", DisplayInfoFile).Output()

		if err != nil {
			return errors.Wrap(err, "failed to run display info command ")
		}
		matchedString := displayInfo.FindAllString(string(out), -1)
		if len(matchedString) != numberOfDisplays {
			return errors.New("connected external display info not found")
		}
		if regexpPatterns != nil {
			for _, pattern := range regexpPatterns {
				if !pattern.MatchString(string(out)) {
					return errors.Errorf("failed %q error message", pattern)
				}
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 15 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "please connect external display as required")
	}
	return nil
}
