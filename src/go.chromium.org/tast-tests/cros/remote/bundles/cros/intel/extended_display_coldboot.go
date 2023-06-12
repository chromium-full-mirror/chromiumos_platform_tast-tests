// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/local/cswitch"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type extendedDisplayTestParams struct {
	displayInfoRe  []string
	ecStateToCheck string
	isTypecDP      bool
}

const (
	connectorDP   = `.*: connectors:\n.\s+\[CONNECTOR:\d+:[DP]+.*`
	connectedDP   = `\[CONNECTOR:\d+:DP.*status: connected`
	fullHDMode    = `modes:\n.*"\d+x\d+":.60`
	connectorHDMI = `.*: connectors:\n.\s+\[CONNECTOR:\d+:[HDMI]+.*`
	connectedHDMI = `\[CONNECTOR:\d+:HDMI.*status: connected`
	typecHDMI     = `.*DP branch device present.*yes\n.*Type.*HDMI`
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtendedDisplayColdboot,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies extended display functionality before and after performing cold boot",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		SoftwareDeps: []string{"chrome", "reboot"},
		ServiceDeps:  []string{"tast.cros.security.BootLockboxService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.InternalDisplay()),
		Vars: []string{
			"servo",
			"intel.cSwitchPort",
			"intel.domainIP",
		},
		VarDeps: []string{"intel.iteration"},
		Params: []testing.Param{{
			Name: "typec_dp",
			Val: extendedDisplayTestParams{
				displayInfoRe:  []string{connectorDP, connectedDP, fullHDMode},
				ecStateToCheck: "S5",
				isTypecDP:      true,
			},
			Timeout: 15 * time.Minute,
		}, {
			Name: "native_dp",
			Val: extendedDisplayTestParams{
				displayInfoRe:  []string{connectorDP, connectedDP, fullHDMode},
				ecStateToCheck: "S5",
				isTypecDP:      false,
			},
			Timeout: 15 * time.Minute,
		}, {
			Name: "typec_hdmi",
			Val: extendedDisplayTestParams{
				displayInfoRe:  []string{connectorDP, connectedDP, fullHDMode, typecHDMI},
				ecStateToCheck: "G3",
				isTypecDP:      false,
			},
			Timeout: 15 * time.Minute,
		}, {
			Name: "native_hdmi",
			Val: extendedDisplayTestParams{
				displayInfoRe:  []string{connectorHDMI, connectedHDMI, fullHDMode},
				ecStateToCheck: "S5",
				isTypecDP:      false,
			},
			Timeout: 15 * time.Minute,
		}},
	})
}

func ExtendedDisplayColdboot(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	servoSpec, _ := s.Var("servo")
	dut := s.DUT()
	testOpt := s.Param().(extendedDisplayTestParams)

	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(cleanupCtx)

	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power on DUT at cleanup: ", err)
			}
		}
	}(cleanupCtx)

	// Login to chrome and check for connected external display
	loginChrome := func(ctx context.Context) {
		// Perform a Chrome login.
		if err := powercontrol.ChromeOSLogin(ctx, dut, s.RPCHint()); err != nil {
			s.Fatal("Failed to login to Chrome: ", err)
		}

		if testOpt.isTypecDP {
			// cswitch port ID.
			cSwitchON := s.RequiredVar("intel.cSwitchPort")
			// IP address of Tqc server hosting device.
			domainIP := s.RequiredVar("intel.domainIP")

			// Create C-Switch session that performs hot plug-unplug external display.
			sessionID, err := cswitch.CreateSession(ctx, domainIP)
			if err != nil {
				s.Fatal("Failed to create session: ", err)
			}

			if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchON, domainIP); err != nil {
				s.Fatal("Failed to enable c-switch port: ", err)
			}

			cSwitchOFF := "0"
			defer func(ctx context.Context) {
				if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
					s.Fatal("Failed to disable c-switch port: ", err)
				}

				if err := cswitch.CloseSession(ctx, sessionID, domainIP); err != nil {
					s.Log("Failed to close session: ", err)
				}
			}(cleanupCtx)
		}

		if err := externalDisplayDetection(ctx, dut, 1, testOpt.displayInfoRe, testOpt.isTypecDP); err != nil {
			s.Fatal("Failed detecting external display: ", err)
		}
	}

	loginChrome(ctx)
	iter, err := strconv.Atoi(s.RequiredVar("intel.iteration"))
	if err != nil {
		s.Fatal("Failed to convert string to integer: ", err)
	}
	for i := 1; i <= iter; i++ {
		s.Logf("Iteration: %d/%d ", i, iter)
		if err := powercontrol.ShutdownAndWaitForPowerState(ctx, pxy, dut, testOpt.ecStateToCheck); err != nil {
			s.Fatalf("Failed to shutdown and wait for %q powerstate: %v", testOpt.ecStateToCheck, err)
		}

		if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
			s.Fatal("Failed to power on DUT: ", err)
		}

		// Login chrome after waking from coldboot.
		loginChrome(ctx)

		valid, err := powercontrol.IsPrevSleepStateAvailable(ctx, dut)

		if err != nil {
			s.Fatal("Failed to determine if prev_sleep_state is available: ", err)
		}

		if valid {
			// Performing prev_sleep_state check.
			const expectedPrevSleepState = 5
			if err := powercontrol.ValidatePrevSleepState(ctx, dut, expectedPrevSleepState); err != nil {
				s.Fatal("Failed to validate previous sleep state: ", err)
			}
		}
	}
}

// externalDisplayDetection verifies connected extended display is detected or not.
func externalDisplayDetection(ctx context.Context, dut *dut.DUT, numberOfDisplays int, regexpStrings []string, isTypecDP bool) error {
	displayInfoFile := "/sys/kernel/debug/dri/0/i915_display_info"
	typecDP := `.*DP branch device present.*no`
	displayInfo := regexp.MustCompile(`.*pipe\s+[BCD]\]:\n.*active=yes, mode=.[0-9]+x[0-9]+.: [0-9]+.*\s+[hw: active=yes]+`)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		out, err := dut.Conn().CommandContext(ctx, "cat", displayInfoFile).Output()
		if err != nil {
			return errors.Wrap(err, "failed to run display info command")
		}

		matchedString := displayInfo.FindAllString(string(out), -1)
		if len(matchedString) != numberOfDisplays {
			return errors.New("connected external display info not found")
		}

		if isTypecDP {
			re := regexp.MustCompile(typecDP)
			matches := re.FindAllString(string(out), -1)
			if len(matches) != numberOfDisplays+1 {
				return errors.New("failed to check for typec DP external display")
			}
		}

		for _, reString := range regexpStrings {
			re := regexp.MustCompile(reString)
			if !re.MatchString(string(out)) {
				return errors.Errorf("failed %q error message", re)
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
