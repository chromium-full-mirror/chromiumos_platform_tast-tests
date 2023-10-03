// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/remote/dutfs"
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
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"chrome"},
		ServiceDeps:  []string{"tast.cros.browser.ChromeService", "tast.cros.ui.ScreenRecorderService"},
		HardwareDeps: hwdep.D(
			hwdep.ChromeEC(),
			hwdep.KeyboardBacklight(),
		),
		Fixture: fixture.NormalMode,
	})
}

type timeoutError struct {
	*errors.E
}

var getKBLightFnc func(h *firmware.Helper, ctx context.Context) (int, error)

// CheckKeyboardBacklightFunctionality confirms keyboard backlight support and verifies its functionality.
func CheckKeyboardBacklightFunctionality(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.RequireRPCClient(ctx); err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}

	// To-do: Remove after we're able to correctly identify
	// duts with keyboard backlight.
	hwdepResults := checkKBLightDependency(ctx, h)
	s.Log("Found hwdep values about keyboard backlight: ", hwdepResults)

	hasKbLight := true
	out, err := h.DUT.Conn().CommandContext(ctx, "backlight_tool", "--keyboard", "--get_brightness").CombinedOutput()
	if err != nil {
		testing.ContextLog(ctx, "Could not obtain output from backlight_tool: ", err)
	}
	outStr := strings.TrimSpace(string(out))
	if strings.Contains(outStr, "No backlight in") {
		hasKbLight = false
	}
	if !hasKbLight {
		s.Logf("Current checks in hwdep.KeyboardBacklight identified %s with backlight, but backlight_tool indicates otherwise", h.Model)
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

	kbLightUp, kbLightDown := getKeyForKbLightUpAndDown(h)
	// Initialize the method for getting kb light value.
	getKBLightFnc = func(h *firmware.Helper, ctx context.Context) (int, error) {
		return h.Servo.GetKBBacklight(ctx)
	}
	initValue, err := checkInitKBBacklight(ctx, h, kbLightUp)
	if err != nil {
		s.Fatal("Failed to check initial keybaord backlight value: ", err)
	}
	switch initValue {
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
	fsClient := dutfs.NewClient(h.RPCClient.Conn)
	minBrightness, maxBrightness, err := getMaxAndMinBrightness(ctx, h, fsClient)
	if err != nil {
		s.Fatal("Failed to get min and max kb backlight brightness level: ", err)
	}

	kbBacklightTesting := make(map[int]string, 2)
	kbBacklightTesting[minBrightness] = kbLightDown
	kbBacklightTesting[maxBrightness] = kbLightUp

	for extremeValue, key := range kbBacklightTesting {
		s.Logf("-----Adjusting keyboard backlight till level %d -----", extremeValue)
		if err := adjustKBBacklight(ctx, h, s.DUT(), extremeValue, 15*time.Second, key, ""); err != nil {
			s.Fatal("Failed to adjust keyboard backlight: ", err)
		}
	}
}

// checkInitKBBacklight presses on a key and checks the initial keyboard backlight value.
func checkInitKBBacklight(ctx context.Context, h *firmware.Helper, initPress string) (int, error) {
	// Press kb-light-up shortcut and check for the initial keyboard brightness value.
	if err := pressShortcut(ctx, h, initPress); err != nil {
		return 0, errors.Wrap(err, "failed to check initial kb backlight")
	}
	kbLight, err := getKBLightFnc(h, ctx)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get kb backlight")
	}
	return kbLight, nil
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
	// Check the pwm value before adjusting kb light if it exists.
	initialPwm, err := checkKbLightPwm(ctx, d)
	if err != nil {
		testing.ContextLog(ctx, "Checking initial kb light pwm failed: ", err)
	}

	kbLight, err := getKBLightFnc(h, ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get kb backlight")
	}

	// Set a specific duration on adjusting the kb light.
	endTime := time.Now().Add(dur)
	for shouldContinue(kbLight, extremeValue, action) {
		timeNow := time.Now()
		if timeNow.After(endTime) {
			// If checking KB light value from ec failed, scan the powerd log instead.
			getKBLightFnc = func(h *firmware.Helper, ctx context.Context) (int, error) {
				return getKBLightValFromPowerd(ctx, h)
			}
			kbLightPowerd, err := getKBLightFnc(h, ctx)
			if err != nil {
				testing.ContextLog(ctx, "Failed to get kb backlight from the powerd log")
			}
			if !shouldContinue(kbLightPowerd, extremeValue, action) {
				return nil
			}
			// At timeout, check the final pwm value for kb light if it exists.
			finalPwm, err := checkKbLightPwm(ctx, d)
			if err != nil {
				testing.ContextLog(ctx, "Checking final kb light pwm failed: ", err)
			}
			hwdepResults := checkKBLightDependency(ctx, h)
			return &timeoutError{E: errors.Errorf(
				"timeout in adjusting kb backlight. Got kb light initial pwm val: %s, final pwm val: %s, and hwdep val: %q, powerd log: %d",
				initialPwm, finalPwm, hwdepResults, kbLightPowerd)}
		}
		testing.ContextLogf(ctx, "Attempting to match, current: %d, expected: %d", kbLight, extremeValue)
		if err := pressShortcut(ctx, h, actionKey); err != nil {
			return errors.Wrap(err, "failed to adjust kb backlight brightness")
		}
		kbLight, err = getKBLightFnc(h, ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get kb backlight")
		}
	}
	return nil
}

// pressShortcut presses, then releases keys to adjust keyboard backlight.
func pressShortcut(ctx context.Context, h *firmware.Helper, actionKey string) error {
	// ShortCuts for decreasing keyboard backlight: Alt+F6 (Alt+BrightnessDown).
	// ShortCuts for increasing keyboard backlight: Alt+F7 (Alt+BrightnessUp).
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	if err := func(ctx context.Context) error {
		keyNames := []string{"<alt_l>", actionKey}
		for _, key := range keyNames {
			row, col, err := h.Servo.GetKeyRowCol(key, h.Model)
			if err != nil {
				return errors.Wrapf(err, "failed to get key column and row for %s", key)
			}
			holdKey := fmt.Sprintf("kbpress %d %d 1", col, row)
			releaseKey := fmt.Sprintf("kbpress %d %d 0", col, row)
			// Press key.
			if err := h.Servo.RunECCommand(ctx, holdKey); err != nil {
				return errors.Wrapf(err, "failed to press and hold %s", key)
			}
			// Release key.
			defer func(ctx context.Context, releaseKey, name string) error {
				if err := h.Servo.RunECCommand(ctx, releaseKey); err != nil {
					return errors.Wrapf(err, "failed to release %s", releaseKey)
				}
				return nil
			}(cleanupCtx, releaseKey, key)
		}
		return nil
	}(ctx); err != nil {
		return err
	}
	return nil
}

// checkKbLightPwm runs the host command 'ectool pwmgetduty kb' to collect pwm value.
func checkKbLightPwm(ctx context.Context, dut *dut.DUT) (string, error) {
	reFoundValue := regexp.MustCompile(`Current PWM duty:\s*\d*`)
	reValue := regexp.MustCompile(`\d+`)
	cmd := firmware.NewECTool(dut, firmware.ECToolNameMain)
	out, err := cmd.Command(ctx, "pwmgetduty", "kb").CombinedOutput()
	if err != nil {
		msg := strings.Split(strings.TrimSpace(string(out)), "\n")
		return "", errors.Errorf("running 'ectool pwmgetduty kb' on DUT failed: %v, and received: %v", err, msg)
	}
	match := reFoundValue.FindSubmatch(out)
	if len(match) == 0 {
		return "", errors.New("did not find pwm duty for kb light")
	}
	val := reValue.FindSubmatch(out)
	pwmVal := strings.TrimSpace(string(val[0]))
	return pwmVal, nil
}

// checkKBLightDependency logs the values from files evaluated by hwdep.KeyboardBacklight.
func checkKBLightDependency(ctx context.Context, h *firmware.Helper) map[string]string {
	reFoundVal := regexp.MustCompile(`\S*`)
	knownKBLightPaths := []string{
		"/run/chromeos-config/v1/keyboard/backlight",
		"/run/chromeos-config/v1/power/has-keyboard-backlight",
		"/usr/share/power_manager/has_keyboard_backlight"}

	hwdepValsMap := make(map[string]string)
	for _, path := range knownKBLightPaths {
		kbLight, err := h.Reporter.CatFile(ctx, path)
		val := reFoundVal.FindStringSubmatch(kbLight)
		if err != nil || len(val) == 0 {
			testing.ContextLogf(ctx, "Unable to read from %s", path)
			continue
		}
		hwdepValsMap[path] = string(kbLight)
	}
	return hwdepValsMap
}

// getKBLightValFromPowerd captures the keyboard backlight value from the powerd log.
func getKBLightValFromPowerd(ctx context.Context, h *firmware.Helper) (int, error) {
	bashCmd := "grep keyboard_backlight_controller.*Setting' 'brightness /var/log/power_manager/powerd.LATEST | tail -1"
	out, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", bashCmd).Output()
	if err != nil {
		return 0, err
	}
	reg := regexp.MustCompile(`Setting brightness to \d+ \((\d+)%\)`)
	val := reg.FindSubmatch(out)
	if len(val) == 0 {
		return 0, errors.New("unable to find match for kb backlight brightness value")
	}
	return strconv.Atoi(string(val[1]))
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

// getMaxAndMinBrightness checks for the maximum and minimum brightness level.
func getMaxAndMinBrightness(ctx context.Context, h *firmware.Helper, fsClient *dutfs.Client) (int, int, error) {
	if fsClient == nil {
		return 0, 0, errors.New("fsClient is nil")
	}
	// Nightfury and Kohaku have their keyboard backlight brightness constrained
	// to the limits specified in keyboard-backlight-user-steps. Declare the default min,
	// and max brightness values as 0 and 100 respectively, but switch to the ones in
	// keyboard-backlight-user-steps if they are available.
	var (
		minBrightness = 0
		maxBrightness = 100
	)
	userStepConfigPath := "/run/chromeos-config/v1/power/keyboard-backlight-user-steps"
	exists, err := fsClient.Exists(ctx, userStepConfigPath)
	if err != nil {
		return minBrightness, maxBrightness, errors.Wrapf(err, "failed to check for the existence of %s", userStepConfigPath)
	}
	if exists {
		out, err := h.Reporter.CatFileLines(ctx, userStepConfigPath)
		if err != nil {
			return minBrightness, maxBrightness, errors.Wrapf(err, "failed to read %s", userStepConfigPath)
		}
		if len(out) != 0 {
			max, err := strconv.ParseFloat(out[len(out)-1], 64)
			if err != nil {
				return minBrightness, maxBrightness, errors.Wrap(err, "failed to parse for max value")
			}
			min, err := strconv.ParseFloat(out[0], 64)
			if err != nil {
				return minBrightness, maxBrightness, errors.Wrap(err, "failed to parse for min value")
			}
			maxBrightness = int(max)
			minBrightness = int(min)
		}
	}
	return minBrightness, maxBrightness, nil
}
