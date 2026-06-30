// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	atSignin     = "atSignin"
	afterSignin  = "afterSignin"
	atLockScreen = "atLockScreen"
)

type tabletModeTestParams struct {
	hasLid        bool
	tabletModeOn  string
	tabletModeOff string
}

func init() {
	testing.AddTest(&testing.Test{
		Func: ECTabletMode,
		Desc: "Checks that power button actions behave as expected in tablet mode, replacing case 1.4.9",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_ec", "firmware_stressed", "firmware_meets_kpi", "firmware_ec_ro", "firmware_ec_rw"},
		SoftwareDeps: []string{"chrome"},
		Vars:         []string{"ui.signinProfileTestExtensionManifestKey"},
		ServiceDeps:  []string{"tast.cros.ui.PowerMenuService", "tast.cros.graphics.ScreenshotService"},
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Params: []testing.Param{{
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible), hwdep.SkipOnModel("nautilus", "nautiluslte", "akali360", "pantheon", "pyke", "sona", "vayne")),
			Val: tabletModeTestParams{
				hasLid:        true,
				tabletModeOn:  "tabletmode on",
				tabletModeOff: "tabletmode off",
			},
		}, {
			Name:              "detachable",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Detachable)),
			Val: tabletModeTestParams{
				hasLid:        true,
				tabletModeOn:  "basestate detach",
				tabletModeOff: "basestate attach",
			},
		}, {
			Name:              "chromeslate",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromeslate)),
			Val: tabletModeTestParams{
				hasLid: false,
			},
		}},
	})
}

func ECTabletMode(ctx context.Context, s *testing.State) {
	d := s.DUT()

	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	// Run EC command to put DUT in tablet mode.
	args := s.Param().(tabletModeTestParams)
	cmd := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain)
	if args.hasLid {
		if _, err := h.Servo.RunTabletModeCommandGetOutput(ctx, args.tabletModeOn); err != nil {
			s.Logf("Failed to run %s, attempting rotation angles with ectool instead", args.tabletModeOn)
			// Setting tabletModeAngle to 0 will force DUT into tablet mode.
			if err := cmd.ForceTabletModeAngle(ctx, "0", "0"); err != nil {
				s.Fatal("Failed to set tablet mode angle: ", err)
			}
		}
		defer func() {
			if _, err := h.Servo.RunTabletModeCommandGetOutput(ctx, args.tabletModeOff); err != nil {
				s.Logf("Failed to run %s, attempting rotation angles with ectool instead", args.tabletModeOff)
				// Setting tabletModeAngle to 360 will force DUT into clamshell mode.
				if err := cmd.ForceTabletModeAngle(ctx, "360", "0"); err != nil {
					s.Fatal("Failed to set tablet mode angle: ", err)
				}
			}
		}()
	}

	// As commented in ticket b:259153719, tablet mode emulation wasn't preserved on a few
	// DUTs over a warm reset. For steps that verify that devices would warm reset into
	// tablet mode, we could maybe manually test the platforms listed in skipWarmResetList by
	// physically folding them into a tablet mode position first.
	skipWarmResetList := []string{"jacuzzi", "hatch", "strongbad"}
	skipWarmReset := func(dutPlatform string, knownList []string) bool {
		for _, name := range knownList {
			if strings.HasPrefix(dutPlatform, name) {
				return true
			}
		}
		return false
	}
	var powerCycleDUT func() error
	if !skipWarmReset(h.Board, skipWarmResetList) {
		powerCycleDUT = func() error {
			s.Log("Power-cycling DUT with a warm reset")
			return h.Servo.SetPowerState(ctx, servo.PowerStateWarmReset)
		}
	}
	// Warm reset cancels the tablet mode emulation on Strongbad machines,
	// but running the remote 'reboot' command preserves it.
	// To-do: revisit and run the reboot command on all machines, after some
	// more local testing.
	if h.Board == "strongbad" {
		powerCycleDUT = func() error {
			s.Log("Running reboot command remotely on DUT")
			return h.DUT.Conn().CommandContext(ctx, "reboot").Run()
		}
	}
	if powerCycleDUT != nil {
		h.CloseRPCConnection(ctx)
		if err := powerCycleDUT(); err != nil {
			s.Fatal("Failed to power-cycle DUT: ", err)
		}

		waitUnreachableCtx, cancelUnreachable := context.WithTimeout(ctx, 2*time.Minute)
		defer cancelUnreachable()
		if err := h.DUT.WaitUnreachable(waitUnreachableCtx); err != nil {
			s.Fatal("Failed to wait DUT unreachable: ", err)
		}

		s.Log("Wait for DUT to power ON")
		waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, 3*time.Minute)
		defer cancelWaitConnect()

		if err := d.WaitConnect(waitConnectCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}
	}

	// Connect to the RPC service on the DUT.
	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	// Get initial boot ID.
	r := reporters.New(d)
	origID, err := r.BootID(ctx)
	if err != nil {
		s.Fatal("Failed to read the original boot ID: ", err)
	}

	// The checkDisplay function checks whether display is on/off
	// by attempting to capture a screenshot. If capturing a screenshot fails,
	// the returned stderr message, "CRTC not found. Is the screen on?", would
	// be returned and checked. Also, in this case, since the screenshot file
	// saved is not needed, it would always get deleted immediately.
	screenshotService := graphics.NewScreenshotServiceClient(h.RPCClient.Conn)
	checkDisplay := func(ctx context.Context) error {
		if _, err := screenshotService.CaptureScreenAndDelete(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to take screenshot")
		}
		return nil
	}

	val, err := s.DUT().Conn().CommandContext(ctx, "cros_config", "/keyboard", "disable-power-button-in-tablet-mode").Output()
	powerButtonDisabled := err == nil && string(val) == "true"

	// The checkPowerMenu function will check whether the power menu is present
	// after holding the power button for about one second.
	powerMenuService := ui.NewPowerMenuServiceClient(h.RPCClient.Conn)
	checkPowerMenu := func(ctx context.Context) error {
		res, err := powerMenuService.PowerMenuPresent(ctx, &empty.Empty{})
		if err != nil {
			return errors.Wrap(err, "failed to check power menu")
		}
		if powerButtonDisabled && res.IsMenuPresent {
			return errors.New("power menu incorrectly triggered")
		}
		if !powerButtonDisabled && !res.IsMenuPresent {
			return errors.New("power menu does not exist")
		}
		return nil
	}

	lockScreen := func(ctx context.Context) error {
		if _, err := powerMenuService.Lock(ctx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to lock screen")
		}
		return nil
	}

	turnDisplayOffAndOn := func(ctx context.Context) error {
		expectedState := []bool{false, true}
		if powerButtonDisabled {
			// Power button has no function, the display will stay on
			expectedState = []bool{true, true}
		}

		s.Log("Turn display off then on, and check that display behaves as expected")
		for _, turnOn := range expectedState {
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurTab); err != nil {
					return errors.Wrap(err, "error pressing power_key:tab")
				}
				// GoBigSleepLint: Simulate a specific speed of power button press to turn display on and off.
				if err := testing.Sleep(ctx, 1*time.Second); err != nil {
					return errors.Wrap(err, "error in sleeping for 1 second after pressing on the power key")
				}

				switch turnOn {
				case false:
					err := checkDisplay(ctx)
					if err == nil {
						return errors.New("unexpectedly able to take screenshot after setting display power off")
					}
					if !strings.Contains(err.Error(), "CRTC not found. Is the screen on?") {
						return errors.Wrap(err, "unexpected error when taking screenshot")
					}
				case true:
					if err := checkDisplay(ctx); err != nil {
						return errors.Wrap(err, "display was not turned ON")
					}
				}
				return nil
			}, &testing.PollOptions{Interval: 1 * time.Second, Timeout: 30 * time.Second}); err != nil {
				return errors.Wrap(err, "failed to set display on/off")
			}
		}
		return nil
	}

	repeatedSteps := func(testCase string) {
		switch testCase {
		case atSignin:
			s.Logf("------------------------Perform testCase: %s------------------------", testCase)
			// Chrome instance is necessary to check for the presence of the power menu.
			// Start Chrome to show the login screen with a user pod.
			manifestKey, ok := s.Var("ui.signinProfileTestExtensionManifestKey")
			if !ok {
				s.Fatal("Failed to get the required secret ui.signinProfileTestExtensionManifestKey. Please install the tast-tests-private repo using the instructions at https://chromium.googlesource.com/chromiumos/third_party/autotest/+/HEAD/docs/faft-how-to-run-doc.md#tast-tests-private")
			}
			signInRequest := ui.NewChromeRequest{
				Login: false,
				Key:   manifestKey,
			}
			if _, err := powerMenuService.NewChrome(ctx, &signInRequest); err != nil {
				s.Fatal("Failed to create new chrome instance with no login for powerMenuService: ", err)
			}
			// Close chrome instance and restart one again with login.
			defer powerMenuService.CloseChrome(ctx, &empty.Empty{})

		case afterSignin:
			s.Logf("------------------------Perform testCase: %s------------------------", testCase)
			// Start Chrome and log in as testuser.
			signInRequest := ui.NewChromeRequest{
				Login: true,
				Key:   "",
			}
			if _, err := powerMenuService.NewChrome(ctx, &signInRequest); err != nil {
				s.Fatal("Failed to create new chrome instance with login for powerMenuService: ", err)
			}

		case atLockScreen:
			s.Logf("------------------------Perform testCase: %s------------------------", testCase)
			s.Log("Lock Screen")
			if err := lockScreen(ctx); err != nil {
				s.Fatal("Lock-screen did not behave as expected: ", err)
			}
		}

		if err := turnDisplayOffAndOn(ctx); err != nil {
			s.Fatal("Display did not behave as expected: ", err)
		}

		s.Log("Bring up the power menu with power_key:press")
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.DurPress); err != nil {
				return errors.Wrap(err, "error bringing up the power menu")
			}
			if err := checkPowerMenu(ctx); err != nil {
				return errors.Wrapf(err, "power menu did not behave as expected %s", testCase)
			}
			return nil
		}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
			s.Fatal("Failed to check whether the power menu is present: ", err)
		}
		s.Logf("Power menu exists: %s", testCase)

		if err := turnDisplayOffAndOn(ctx); err != nil {
			s.Fatal("Display did not behave as expected: ", err)
		}

		// Short press on power button to activate the pre-shutdown animation.
		// Differentiate the press durations on Zork from the other platforms.
		// Depending on Stainless results, a new flag may be created inside
		// fw-testing-configs for a more general use.
		s.Log("Activate the pre-shutdown animation")
		var whiteScreenPwrDur time.Duration
		if h.Config.Platform == "zork" {
			whiteScreenPwrDur = 2000 * time.Millisecond
		} else {
			whiteScreenPwrDur = (h.Config.HoldPwrButtonPowerOff) / 3
		}
		if err := h.Servo.KeypressWithDuration(ctx, servo.PowerKey, servo.Dur((whiteScreenPwrDur))); err != nil {
			s.Fatal("Failed to set a KeypressControl by servo: ", err)
		}

		// Verify that DUT did not reboot.
		curID, err := r.BootID(ctx)
		if err != nil {
			s.Fatal("Failed to read the current boot ID: ", err)
		}
		if curID != origID {
			s.Fatalf("DUT rebooted after short power press, got current ID: %s, and ID before: %s", curID, origID)
		}
		s.Log("DUT did not reboot")
	}

	var testCases = []string{atSignin, afterSignin, atLockScreen}
	for _, testCase := range testCases {
		repeatedSteps(testCase)
	}
}
