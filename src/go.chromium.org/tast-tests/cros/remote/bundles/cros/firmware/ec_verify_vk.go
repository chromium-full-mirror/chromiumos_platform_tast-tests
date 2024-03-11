// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Models in noTabletMode are convertibles that were observed to not support
// tablet mode. Following are the observed issues with these models:
// 1. Got "Command 'tabletmode' not found or ambiguous" after sending
// 'tabletmode on' in ec console.
// 2. Got "EC result 3 (INVALID_PARAM)" after sending
// 'ectool motionsense tablet_mode_angle' in VT2.
// 3. Got "EC result 1 (INVALID_COMMAND) Failed to set tablet mode, rv=-1001"
// after sending 'ectool tabletmode on' in VT2.
var noTabletMode = []string{
	"akali360",
	"nautilus",
	"nautiluslte",
	"pantheon",
	"robo360",
}

type dutType int

const (
	convertible dutType = iota
	detachable
	chromeslate
)

type dutTestParams struct {
	canDoTabletSwitch bool
	formFactor        dutType
	tabletModeOn      string
	tabletModeOff     string
}

type verifyVkErr struct {
	*errors.E
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ECVerifyVK,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify whether virtual keyboard window is present during change in tablet mode",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec"},
		Requirements: []string{"sys-fw-0022-v02"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.ui.ScreenRecorderService", "tast.cros.browser.ChromeService", "tast.cros.ui.CheckVirtualKeyboardService", "tast.cros.firmware.UtilsService"},
		Fixture:      fixture.NormalMode,
		Timeout:      8 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.TouchScreen()),
		Params: []testing.Param{{
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible), hwdep.SkipOnModel(noTabletMode...)),
			Val: dutTestParams{
				canDoTabletSwitch: true,
				formFactor:        convertible,
				tabletModeOn:      "tabletmode on",
				tabletModeOff:     "tabletmode off",
			},
		}, {
			Name:              "detachable",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Detachable), hwdep.Keyboard()),
			Val: dutTestParams{
				canDoTabletSwitch: true,
				formFactor:        detachable,
				tabletModeOn:      "basestate detach",
				tabletModeOff:     "basestate attach",
			},
			ExtraAttr: []string{"firmware_detachable"},
		}, {
			Name:              "chromeslate",
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Chromeslate)),
			Val: dutTestParams{
				canDoTabletSwitch: false,
				formFactor:        chromeslate,
			},
		}},
	})
}

func ECVerifyVK(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}
	// Perform a hard reset on DUT to ensure removal of any
	// old settings that might potentially have an impact on
	// this test.
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to cold reset DUT at the beginning of test: ", err)
	}
	h.DisconnectDUT(ctx)

	// Wait for DUT to reconnect.
	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	if err := h.RequireRPCUtils(ctx); err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	s.Log("Logging in as a guest user")
	chromeService := pb.NewChromeServiceClient(h.RPCClient.Conn)
	if _, err := chromeService.New(ctx, &pb.NewRequest{
		LoginMode: pb.LoginMode_LOGIN_MODE_GUEST_LOGIN,
	}); err != nil {
		s.Fatal("Failed to login: ", err)
	}
	defer chromeService.Close(ctx, &empty.Empty{})

	s.Log("Screen recorder started")
	filePath := filepath.Join(s.OutDir(), "ecVerifyVK.webm")
	startRequest := pb.StartRequest{
		FileName: filePath,
	}
	screenRecorder := pb.NewScreenRecorderServiceClient(h.RPCClient.Conn)

	if _, err := screenRecorder.Start(ctx, &startRequest); err != nil {
		s.Fatal("Failed to start recording: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	defer func(ctx context.Context) {
		// Restore init keyboard and usb keyboard to the default value.
		if err := h.Servo.SetOnOff(ctx, servo.InitKeyboard, servo.On); err != nil {
			s.Fatal("Failed to set init keyboard to on: ", err)
		}
		if err := h.Servo.SetOnOff(ctx, servo.USBKeyboard, servo.Off); err != nil {
			s.Fatal("Failed to set usb keyboard to off: ", err)
		}
		res, err := screenRecorder.Stop(ctx, &empty.Empty{})
		if err != nil {
			s.Log("Unable to save the recording: ", err)
		} else {
			s.Logf("Screen recording saved to %s", res.FileName)
		}

		testing.ContextLog(ctx, "Copying screen recording from DUT to local machine")
		destPath := filepath.Join(s.OutDir(), filepath.Base(res.FileName))
		if err := linuxssh.GetFile(ctx, s.DUT().Conn(), res.FileName, destPath, linuxssh.DereferenceSymlinks); err != nil {
			s.Fatal("Failed to copy screen recording to local machine: ", err)
		}
	}(cleanupCtx)

	// Restore tablet mode settings so that DUT won't
	// be left in tablet mode at the end of test.
	args := s.Param().(dutTestParams)
	ecTool := firmware.NewECTool(s.DUT(), firmware.ECToolNameMain)
	var restoreLaptopMode bool
	defer func(ctx context.Context) {
		if restoreLaptopMode {
			s.Log("Restoring laptop mode setting at the end of test")
			if _, err := h.Servo.RunTabletModeCommandGetOutput(ctx, args.tabletModeOff); err != nil {
				s.Fatal("Unable to restore laptop mode: ", err)
			}
		}
		if args.formFactor == convertible {
			// Set tablet mode angles to the default settings under ectool
			// at the end of test if they are not.
			tabletModeAngleInit, hysInit, err := ecTool.SaveTabletModeAngles(ctx)
			if err != nil {
				s.Fatal("Failed to read tablet mode angles: ", err)
			} else if tabletModeAngleInit != "180" || hysInit != "20" {
				s.Log("Restoring ectool tablet mode angles to the default settings")
				if err := ecTool.ForceTabletModeAngle(ctx, "180", "20"); err != nil {
					s.Fatal("Failed to restore tablet mode angles to the default settings: ", err)
				}
			}
		}
	}(cleanupCtx)

	vkService := pb.NewCheckVirtualKeyboardServiceClient(h.RPCClient.Conn)
	for _, tc := range []struct {
		formFactor        dutType
		canDoTabletSwitch bool
		turnTabletModeOn  bool
		tabletModeCmd     string
	}{
		{args.formFactor, args.canDoTabletSwitch, true, args.tabletModeOn},
		{args.formFactor, args.canDoTabletSwitch, false, args.tabletModeOff},
	} {
		if err := switchDUTMode(ctx, h, tc.canDoTabletSwitch, tc.turnTabletModeOn, tc.tabletModeCmd, ecTool); err != nil {
			s.Fatalf("Failed to run %s: %v", tc.tabletModeCmd, err)
		}
		if tc.turnTabletModeOn {
			restoreLaptopMode = true
		} else {
			restoreLaptopMode = false
		}
		s.Log("Sleeping for a few seconds")
		// GoBigSleepLint: Short delay to ensure that switching to tablet mode has
		// fully taken effect.
		if err := testing.Sleep(ctx, 5*time.Second); err != nil {
			s.Fatal("Failed to sleep: ", err)
		}
		s.Log("Checking tablet mode status")
		dutInTabletMode, err := checkTabletMode(ctx, h, tc.turnTabletModeOn)
		if err != nil {
			s.Fatal("Failed to check DUT's tablet mode status: ", err)
		}

		verifyVK := func() error {
			s.Log("Using search bar to trigger virtual keyboard")
			if _, err := vkService.ClickSearchBar(ctx, &pb.CheckVirtualKeyboardRequest{
				IsDutTabletMode: dutInTabletMode,
			}); err != nil {
				return errors.Wrap(err, "failed to click Search Bar")
			}
			s.Log("Checking if virtual keyboard is present")
			if err := checkVKIsPresent(ctx, h, vkService, dutInTabletMode); err != nil {
				return &verifyVkErr{E: errors.Wrap(err, "failed to check VK is present")}
			}
			return nil
		}
		for _, testCase := range []struct {
			keyboard servo.OnOffControl
			onOff    servo.OnOffValue
		}{
			{keyboard: servo.InitKeyboard, onOff: servo.On},
			{keyboard: servo.InitKeyboard, onOff: servo.Off},
			{keyboard: servo.USBKeyboard, onOff: servo.On},
			{keyboard: servo.USBKeyboard, onOff: servo.Off},
		} {
			if err := setAndVerifyKeyboard(ctx, h, testCase.keyboard, testCase.onOff); err != nil {
				s.Fatalf("Failed to set %v to %v: %v", testCase.keyboard, testCase.onOff, err)
			}
		}
		if err := checkServoKeyboard(ctx, h, false); err != nil {
			if _, ok := err.(*servoKeyboardErr); !ok {
				s.Fatal("Failed to check servo keyboard: ", err)
			}
		}
		if err := verifyVK(); err != nil {
			_, ok := err.(*verifyVkErr)
			if tc.turnTabletModeOn && ok {
				// If on-screen keyboard is enabled, vk should appear
				// when search bar is clicked in both laptop and tablet
				// mode. For debugging purposes, if checking for vk failed
				// in tablet mode, check again with on-screen keyboard allowed.
				s.Log("Unable to find virtual keyboard in tablet mode, testing with on-screen keyboard enabled")
				if _, err := vkService.EnableOnscreenKeyboard(ctx, &empty.Empty{}); err != nil {
					s.Fatal("Failed to enable on-screen keyboard: ", err)
				}
				if err := verifyVK(); err != nil {
					s.Fatal("Failed to verify on-screen keyboard: ", err)
				}
			}
			s.Fatal("Failed to verify virtual keyboard, but passed with on-screen keyboard enabled: ", err)
		}
		if tc.formFactor == chromeslate {
			// Because chromeslates do not support clamshell mode,
			// skip the clamshell mode test.
			break
		}
	}
}

func switchDUTMode(ctx context.Context, h *firmware.Helper, canDoTabletSwitch, turnTabletModeOn bool, tabletModeCmd string, ecTool *firmware.ECTool) error {
	if !canDoTabletSwitch {
		return nil
	}
	forceTabletModeAngle := func(ctx context.Context) error {
		if turnTabletModeOn {
			// Setting tabletModeAngle to 0s will force DUT into tablet mode.
			if err := ecTool.ForceTabletModeAngle(ctx, "0", "0"); err != nil {
				return errors.Wrap(err, "failed to force DUT into tablet mode")
			}
		} else {
			// Setting tabletModeAngle to 360 will force DUT into clamshell mode.
			if err := ecTool.ForceTabletModeAngle(ctx, "360", "0"); err != nil {
				return errors.Wrap(err, "failed to force DUT into clamshell mode")
			}
		}
		return nil
	}
	testing.ContextLogf(ctx, "Running EC command %s to change DUT's tablet mode state", tabletModeCmd)
	_, err := h.Servo.RunTabletModeCommandGetOutput(ctx, tabletModeCmd)
	if err != nil {
		if _, ok := err.(*servo.TabletModeCmdUnsupportedErr); !ok {
			return errors.Wrap(err, "failed to set DUT tablet mode state")
		}
		testing.ContextLogf(ctx, "Failed to set DUT tablet mode state, and got: %v. Attempting to set tablet_mode_angle with ectool instead", err)
		if err := forceTabletModeAngle(ctx); err != nil {
			return errors.Wrap(err, "failed to set DUT tablet mode state")
		}
		return nil
	}
	return nil
}

func checkTabletMode(ctx context.Context, h *firmware.Helper, turnTabletModeOn bool) (bool, error) {
	// Reuse the existing guest session created via ChromeService at the beginning of test.
	if _, err := h.RPCUtils.ReuseChrome(ctx, &empty.Empty{}); err != nil {
		return false, errors.Wrap(err, "failed to reuse Chrome session for the utils service")
	}
	res, err := h.RPCUtils.EvalTabletMode(ctx, &empty.Empty{})
	if err != nil {
		return false, errors.Wrap(err, "unable to evaluate whether ChromeOS is in tablet mode")
	} else if res.TabletModeEnabled != turnTabletModeOn {
		return false, errors.Errorf("expecting tablet mode on: %t, but got: %t", turnTabletModeOn, res.TabletModeEnabled)
	}
	testing.ContextLogf(ctx, "ChromeOS in tabletmode: %t", res.TabletModeEnabled)
	return res.TabletModeEnabled, nil
}

func checkVKIsPresent(ctx context.Context, h *firmware.Helper, cvkc pb.CheckVirtualKeyboardServiceClient, tabletMode bool) error {
	req := pb.CheckVirtualKeyboardRequest{
		IsDutTabletMode: tabletMode,
	}
	res, err := cvkc.CheckVirtualKeyboardIsPresent(ctx, &req)
	if err != nil {
		return err
	} else if tabletMode != res.IsVirtualKeyboardPresent {
		return errors.Errorf(
			"found unexpected behavior, and got tabletmode: %t, VirtualKeyboardPresent: %t",
			tabletMode, res.IsVirtualKeyboardPresent)
	}
	return nil
}

func setAndVerifyKeyboard(ctx context.Context, h *firmware.Helper, keyboard servo.OnOffControl, onOffVal servo.OnOffValue) error {
	var onoff bool
	switch onOffVal {
	case servo.On:
		onoff = true
	case servo.Off:
		onoff = false
	}
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Setting %s to %s", keyboard, onOffVal)
		if err := h.Servo.SetOnOff(ctx, keyboard, onOffVal); err != nil {
			return errors.Wrapf(err, "failed to set %s to %s", keyboard, onOffVal)
		}
		keyboardValue, err := h.Servo.GetOnOff(ctx, keyboard)
		if err != nil {
			return errors.Wrapf(err, "failed to get %s", keyboard)
		}
		if keyboardValue != onoff {
			return errors.Errorf("got unexpected %s value: %t", keyboard, keyboardValue)
		}
		return nil
	}, &testing.PollOptions{Interval: time.Second, Timeout: 20 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to set and verify keyboard")
	}
	return nil
}

type servoKeyboardErr struct {
	*errors.E
}

func checkServoKeyboard(ctx context.Context, h *firmware.Helper, expectedExist bool) error {
	servoKeyboardString := "Atmel Corp. LUFA Keyboard Demo Application"
	lsusbOutput, err := h.DUT.Conn().CommandContext(ctx, "lsusb").Output(ssh.DumpLogOnError)
	if err != nil {
		return errors.Wrap(err, "running lsusb")
	}
	isExist := strings.Contains(string(lsusbOutput), servoKeyboardString)
	if expectedExist != isExist {
		testing.ContextLogf(ctx, "Expected servo keyboard exists: %v, but got: %v", expectedExist, isExist)
		return &servoKeyboardErr{errors.Errorf("got unexpected servo keyboard value: %v", isExist)}
	}
	return nil
}
