// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"path/filepath"
	"regexp"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type checkKeyboardBacklightTest int

const (
	adjustBacklightWithKeyboardShortcuts checkKeyboardBacklightTest = iota
	lidCloseAndOpen
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CheckKeyboardBacklightFunctionality,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Confirm keyboard backlight support and check keyboard backlight functionality",
		Contacts: []string{
			"chromeos-faft@google.com",
			"cienet-firmware@cienet.corp-partner.google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_ec"},
		Requirements: []string{"sys-fw-0022-v02"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.ui.ScreenRecorderService"},
		HardwareDeps: hwdep.D(
			hwdep.ChromeEC(),
			hwdep.KeyboardBacklight(),
		),
		Fixture: fixture.NormalMode,
		Params: []testing.Param{{
			Val: adjustBacklightWithKeyboardShortcuts,
		}, {
			Name: "lid_close_and_open",
			Val:  lidCloseAndOpen,
		}},
	})
}

type timeoutError struct {
	*errors.E
}

var regPowerdBrightness = regexp.MustCompile(`Setting brightness to \d+ \((\d+)%\)`)

// CheckKeyboardBacklightFunctionality confirms keyboard backlight support and verifies its functionality.
func CheckKeyboardBacklightFunctionality(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	// Reboot the DUT with a cold reset so that the keyboard backlight
	// value from the ec command 'kblight' gets restored, just in case
	// it was left at the value set by previous tests.
	s.Log("Rebooting the DUT with a cold reset")
	if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
		s.Fatal("Failed to cold reset the DUT: ", err)
	}

	waitConnectCtx, cancelWaitConnect := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancelWaitConnect()

	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		s.Fatal("Failed to reconnect to dut: ", err)
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	s.Log("Starting a new Chrome")
	chromeService := pb.NewChromeServiceClient(h.RPCClient.Conn)
	if _, err := chromeService.New(ctx, &pb.NewRequest{
		LoginMode: pb.LoginMode_LOGIN_MODE_GUEST_LOGIN,
	}); err != nil {
		s.Fatal("Failed to create new Chrome at login: ", err)
	}
	defer chromeService.Close(ctx, &empty.Empty{})

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	testMethod := s.Param().(checkKeyboardBacklightTest)
	switch testMethod {
	case adjustBacklightWithKeyboardShortcuts:
		s.Log("Screen recorder started")
		filePath := filepath.Join(s.OutDir(), "kblightRecord.webm")
		screenRecorder := pb.NewScreenRecorderServiceClient(h.RPCClient.Conn)
		if _, err := screenRecorder.Start(ctx, &pb.StartRequest{
			FileName: filePath,
		}); err != nil {
			s.Fatal("Failed to start recording: ", err)
		}
		defer func(ctx context.Context) {
			res, err := screenRecorder.Stop(ctx, &empty.Empty{})
			if err != nil {
				s.Log("Unable to save the recording: ", err)
			} else {
				s.Logf("Screen recording saved to %s", res.FileName)
			}

			s.Log("Copying screen recording from DUT to local machine")
			destPath := filepath.Join(s.OutDir(), filepath.Base(res.FileName))
			if err := linuxssh.GetFile(ctx, s.DUT().Conn(), res.FileName, destPath, linuxssh.DereferenceSymlinks); err != nil {
				s.Fatal("Failed to copy screen recording to local machine: ", err)
			}
		}(cleanupCtx)
	case lidCloseAndOpen:
		defer func(ctx context.Context) {
			if err := h.Servo.OpenLid(ctx); err != nil {
				s.Fatal("Failed to open lid: ", err)
			}
		}(cleanupCtx)
	}

	kbLightUp, kbLightDown := getKeyForKbLightUpAndDown(h)
	// Press the keyboard to increase the backlight, creating a record in the powerd log.
	if err := h.Servo.PressKeys(ctx, []string{"<alt_l>", kbLightUp}, servo.DurTab); err != nil {
		s.Fatal("Failed to increase keyboard backlight: ", err)
	}
	currKBLight, err := func() (int, error) {
		matches, err := getKBLightValFromPowerd(ctx, h)
		if err != nil {
			return -1, err
		}
		return matches[len(matches)-1], nil
	}()
	if err != nil {
		s.Fatal("Failed to get current kblight: ", err)
	}
	switch currKBLight {
	case 0:
		s.Log("Keyboard initial backlight value is 0, attempting to increase the light to at least 30 percent before test")
		err = adjustKBBacklight(ctx, h, s.DUT(), 30, 15*time.Second, kbLightUp, "increasing")
	case 100:
		s.Log("Keyboard initial backlight value is 100, attempting to decrease the light to at leaset 30 percent before test")
		err = adjustKBBacklight(ctx, h, s.DUT(), 30, 15*time.Second, kbLightDown, "decreasing")
	}
	if err != nil {
		if _, ok := err.(*timeoutError); ok {
			s.Fatal("Test ended: ", err.(*timeoutError))
		} else {
			s.Fatal("Unexpected error: ", err)
		}
	}

	switch testMethod {
	case adjustBacklightWithKeyboardShortcuts:
		kbBacklightTesting := make(map[int]string, 2)
		kbBacklightTesting[0] = kbLightDown
		kbBacklightTesting[100] = kbLightUp

		for extremeValue, key := range kbBacklightTesting {
			s.Logf("-----Adjusting keyboard backlight till level %d -----", extremeValue)
			if err := adjustKBBacklight(ctx, h, s.DUT(), extremeValue, 15*time.Second, key, ""); err != nil {
				s.Fatal("Failed to adjust keyboard backlight: ", err)
			}
		}
	case lidCloseAndOpen:
		if err := checkKBLightWhenLidClosedOpen(ctx, h, s.DUT()); err != nil {
			s.Fatal("Failed to verify keyboard backlight level when lid is closed and reopened: ", err)
		}
	}
}

// checkKBLightWhenLidClosedOpen checks for keyboard backlight turned off
// when lid is closed, and turned back on when the lid reopens.
func checkKBLightWhenLidClosedOpen(ctx context.Context, h *firmware.Helper, dut *dut.DUT) error {
	if err := h.Servo.CloseLid(ctx); err != nil {
		return err
	}
	waitDisconnectCtx, cancel := context.WithTimeout(ctx, 30*time.Second)
	defer cancel()
	if err := h.DUT.WaitUnreachable(waitDisconnectCtx); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to become unreachable")
	}

	// Ensure DUT reaches G3, S3 or S0ix before attempting to open the lid.
	if err := h.WaitForPowerStates(ctx, firmware.PowerStateInterval, 1*time.Minute, "G3", "S3", "S0ix"); err != nil {
		return errors.Wrap(err, "failed to wait for DUT to reach G3, S3 or S0ix power state")
	}

	if err := h.Servo.OpenLid(ctx); err != nil {
		return err
	}
	waitConnectCtx, cancel := context.WithTimeout(ctx, 90*time.Second)
	defer cancel()
	if err := h.WaitConnect(waitConnectCtx, firmware.ResetEthernetDongle); err != nil {
		return errors.Wrap(err, "failed to reconnect to dut")
	}

	var kbLightValues []int
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var err error
		kbLightValues, err = getKBLightValFromPowerd(ctx, h)
		if err != nil {
			return err
		}
		if len(kbLightValues)%2 != 0 || len(kbLightValues) == 0 {
			return errors.New("unexpected number of keyboard backlight values")
		}
		return nil
	}, &testing.PollOptions{Timeout: 30 * time.Second, Interval: 1 * time.Second}); err != nil {
		return err
	}

	kbLightClosedLid := kbLightValues[len(kbLightValues)-2]
	if kbLightClosedLid != 0 {
		return errors.Errorf("expected kb backlight to be 0 when lid is closed, but got: %d", kbLightClosedLid)
	}
	kbLightOpenLid := kbLightValues[len(kbLightValues)-1]
	if kbLightOpenLid == 0 {
		return errors.New("got kb backlight level at 0 when lid is open")
	}
	return nil
}

// shouldContinue continues adjustment on keyboard backlight until reaching the desired value.
func shouldContinue(kbBacklight, extremeValue int, action string) bool {
	switch action {
	case "increasing":
		return kbBacklight <= extremeValue
	case "decreasing":
		return kbBacklight >= extremeValue
	default:
		return kbBacklight != extremeValue
	}
}

// adjustKBBacklight attempts to adjust keyboard backlight within a certain duration of time.
// If a timeout is reached, possibly because no physical kb light exists, some information will
// be logged regarding pwm values, and values from files evaluated in hwdep.
func adjustKBBacklight(ctx context.Context, h *firmware.Helper, d *dut.DUT, extremeValue int, dur time.Duration, actionKey, action string) error {
	getCurrentKBLight := func(ctx context.Context, h *firmware.Helper) (int, error) {
		matches, err := getKBLightValFromPowerd(ctx, h)
		if err != nil {
			return -1, err
		}
		return matches[len(matches)-1], nil
	}
	kbLight, err := getCurrentKBLight(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to get kb backlight")
	}

	// Set a specific duration on adjusting the kb light.
	endTime := time.Now().Add(dur)
	for shouldContinue(kbLight, extremeValue, action) {
		timeNow := time.Now()
		if timeNow.After(endTime) {
			kbLightFromEC, err := h.Servo.GetKBBacklight(ctx)
			if err != nil {
				testing.ContextLog(ctx, "Failed to run ec command to get kb backlight")
			}
			if !shouldContinue(kbLightFromEC, extremeValue, action) {
				return nil
			}
			return &timeoutError{E: errors.Errorf(
				"timeout in adjusting kb backlight. Got kb light from ec command: %d", kbLightFromEC)}
		}
		testing.ContextLogf(ctx, "Attempting to match, current: %d, expected: %d", kbLight, extremeValue)
		if err := h.Servo.PressKeys(ctx, []string{"<alt_l>", actionKey}, servo.DurTab); err != nil {
			return errors.Wrapf(err, "failed to press alt_l and %v", actionKey)
		}

		// GoBigSleepLint: key presses delay to adjust the keyboard backlight.
		if err := testing.Sleep(ctx, h.Config.KeypressDelay); err != nil {
			return errors.Wrap(err, "failed to sleep")
		}
		kbLight, err = getCurrentKBLight(ctx, h)
		if err != nil {
			return errors.Wrap(err, "failed to get kb backlight")
		}
	}
	return nil
}

// getKBLightValFromPowerd captures the keyboard backlight value from the powerd log.
func getKBLightValFromPowerd(ctx context.Context, h *firmware.Helper) ([]int, error) {
	bashCmd := "grep keyboard_backlight_controller.*Setting' 'brightness /var/log/power_manager/powerd.LATEST"
	out, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", bashCmd).Output()
	if err != nil {
		return nil, err
	}
	data := regPowerdBrightness.FindAllSubmatch(out, -1)
	if len(data) == 0 {
		return nil, errors.New("did not find any records in the powerd log about kb backlight")
	}
	var kbLightVals []int
	for _, val := range data {
		found, err := strconv.Atoi(string(val[1]))
		if err != nil {
			return nil, errors.Errorf("unable to parse %s", string(val[1]))
		}
		kbLightVals = append(kbLightVals, found)
	}
	return kbLightVals, nil
}

// getKeyForKbLightUpAndDown checks for the respective shortcuts to increase
// and decrease keyboard backlight brightness.
func getKeyForKbLightUpAndDown(h *firmware.Helper) (string, string) {
	kbLightUp := "<f7>"
	kbLightDown := "<f6>"
	modelsWithShiftedShortcuts := []string{"atlas", "eve"}
	// Some models use <f6> and <f5> instead for adjusting the kb light.
	for _, model := range modelsWithShiftedShortcuts {
		if h.Model == model {
			kbLightUp = "<f6>"
			kbLightDown = "<f5>"
		}
	}
	return kbLightUp, kbLightDown
}
