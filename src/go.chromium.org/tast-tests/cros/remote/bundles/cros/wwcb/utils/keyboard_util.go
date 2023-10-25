// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package utils used to do some component excution function.
package utils

import (
	"bufio"
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/dutfs"
	pb "go.chromium.org/tast-tests/cros/services/cros/apps"
	"go.chromium.org/tast-tests/cros/services/cros/inputs"
	"go.chromium.org/tast-tests/cros/services/cros/ui"
	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	typeTimeout           = 5 * time.Second
	testFilePath          = "/tmp/CharacterKeysTest.txt"
	specialKeysTestAnswer = "137"
)

// ReadKeyPress read the key input from the keyboard.
func ReadKeyPress(ctx context.Context, scanner *bufio.Scanner, s *testing.State, pressFunc func(ctx context.Context, keyByte []byte, duration time.Duration) error, keyByte []byte, keyCode string) error {
	regex := `Event.*time.*code\s(\d*)\s\(` + keyCode + `\)`
	expMatch := regexp.MustCompile(regex)
	text := make(chan string)
	go func() {
		defer close(text)
		for scanner.Scan() {
			text <- scanner.Text()
		}
	}()
	if err := pressFunc(ctx, keyByte, 100*time.Millisecond); err != nil {
		return errors.Wrapf(err, "unable to Press the %s Keycode", keyCode)
	}
	for {
		select {
		case <-time.After(typeTimeout):
			return errors.New("did not detect keycode within expected time")
		case out := <-text:
			if match := expMatch.FindStringSubmatch(out); match != nil {
				testing.ContextLog(ctx, "key pressed detected: ", match)
				return nil
			}
		}
	}
}

// CheckCharacterKeys check the input and functionality of character keys.
func CheckCharacterKeys(ctx context.Context, s *testing.State, cl *rpc.Client, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, kb inputs.KeyboardServiceClient, device string) error {
	// Launch the crosh app.
	if err := LaunchCroshApp(ctx, appsSvc, uiautoSvc, kb); err != nil {
		return errors.Wrap(err, "failed to launch crosh app")
	}
	defer CloseCroshApp(ctx, uiautoSvc, kb)
	// Open nano and test typing in /tmp.
	if _, err := kb.Type(ctx, &inputs.TypeRequest{
		Key: fmt.Sprintf("shell\nnano %s\n", testFilePath),
	}); err != nil {
		return errors.Wrap(err, "failed to open nano and enter the command")
	}
	cmd := s.DUT().Conn().CommandContext(ctx, "evtest", device)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "unable to obtain stdout pipe")
	}
	scanner := bufio.NewScanner(stdout)
	cmd.Start()
	inputKey := ""
	for testKey, testValue := range ChatKeyCodes {
		key, keyCode := testKey, testValue.CBKeyCode
		inputKey += ChatKeyCodes[key].CBChatKeyString
		testing.ContextLogf(ctx, "Pressing key %q, expecting to read keycode %q", key, keyCode)
		if err := ReadKeyPress(ctx, scanner, s, SinglePressKeyboard, ChatKeyCodes[key].KeyCodeBytes, keyCode); err != nil {
			return errors.Wrap(err, "failed to read key")
		}
	}
	// Save the file.
	if _, err := kb.Accel(ctx, &inputs.AccelRequest{
		Key: "Ctrl+S",
	}); err != nil {
		return errors.Wrap(err, "failed to save the file using nano")
	}
	fs := dutfs.NewClient(cl.Conn)
	defer fs.Remove(ctx, testFilePath)
	// Close the file.
	if _, err := kb.Accel(ctx, &inputs.AccelRequest{
		Key: "Ctrl+X",
	}); err != nil {
		return errors.Wrap(err, "failed to close the file using nano")
	}
	testFile, err := fs.ReadFile(ctx, testFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to read test file")
	}
	// Compare the contents of text files.
	if inputKey != strings.TrimSpace(string(testFile)) {
		return errors.Errorf("text file comparison failed. Expected: '%q', Got: '%q'", inputKey, strings.TrimSpace(string(testFile)))
	}
	return nil
}

// CheckMouseButton check mouse button functionality.
func CheckMouseButton(ctx context.Context, s *testing.State, device string) error {
	cmd := s.DUT().Conn().CommandContext(ctx, "evtest", device)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "unable to obtain stdout pipe")
	}
	scanner := bufio.NewScanner(stdout)
	cmd.Start()
	for testKey, testValue := range MouseKeyCodes {
		key, keyCode := testKey, testValue.CBKeyCode
		testing.ContextLogf(ctx, "Pressing key %q, expecting to read keycode %q", key, keyCode)
		if err = ReadKeyPress(ctx, scanner, s, MouseClick, MouseKeyCodes[key].KeyCodeBytes, keyCode); err != nil {
			return errors.Wrap(err, "failed to read key")
		}
	}
	return nil
}

// CheckSpecialKeys check the input and functionality of special keys.
func CheckSpecialKeys(ctx context.Context, s *testing.State, cl *rpc.Client, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, kb inputs.KeyboardServiceClient, device string) error {
	testKeyArr := []string{
		"PrintScreen",
		"Home",
		"End",
		"PageUp",
		"PageDown",
		"Up",
		"Down",
		"Left",
		"Right"}
	// Launch the crosh app.
	if err := LaunchCroshApp(ctx, appsSvc, uiautoSvc, kb); err != nil {
		return errors.Wrap(err, "failed to launch crosh app")
	}
	defer CloseCroshApp(ctx, uiautoSvc, kb)
	// Open nano and test typing in /tmp.
	if _, err := kb.Type(ctx, &inputs.TypeRequest{
		Key: fmt.Sprintf("shell\nnano %s\n01234\n56789", testFilePath),
	}); err != nil {
		return errors.Wrap(err, "failed to open nano and enter the command")
	}
	cmd := s.DUT().Conn().CommandContext(ctx, "evtest", device)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return errors.Wrap(err, "unable to obtain stdout pipe")
	}
	scanner := bufio.NewScanner(stdout)
	cmd.Start()
	for _, testKey := range testKeyArr {
		key, keyCode := testKey, SpecialKeyCodes[testKey].CBKeyCode
		testing.ContextLogf(ctx, "Pressing key %q, expecting to read keycode %q", key, keyCode)
		if err = ReadKeyPress(ctx, scanner, s, SinglePressKeyboard, SpecialKeyCodes[key].KeyCodeBytes, keyCode); err != nil {
			return errors.Wrap(err, "failed to read key")
		}
		switch key {
		case "PrintScreen":
			messageFinder := &ui.Finder{
				NodeWiths: []*ui.NodeWith{
					{Value: &ui.NodeWith_HasClass{HasClass: "Label"}},
					{Value: &ui.NodeWith_Name{Name: "Drag to select an area to capture"}},
				},
			}
			closeButtonFinder := &ui.Finder{
				NodeWiths: []*ui.NodeWith{
					{Value: &ui.NodeWith_HasClass{HasClass: "IconButton"}},
					{Value: &ui.NodeWith_Name{Name: "Close"}},
				},
			}
			if _, err = uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: messageFinder}); err != nil {
				return errors.Wrap(err, "message does not exist")
			}
			if _, err = uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: closeButtonFinder}); err != nil {
				return errors.Wrap(err, "close button does not exist")
			}
			_, err = uiautoSvc.LeftClick(ctx, &ui.LeftClickRequest{Finder: closeButtonFinder})
			if err != nil {
				return errors.Wrap(err, "clicking close button failed")
			}
		case "Home":
			if err := SinglePressKeyboard(ctx, SpecialKeyCodes["Delete"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", key)
			}
		case "End":
			if err := SinglePressKeyboard(ctx, TopRowKeyCodes["Back"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", key)
			}
		case "PageUp":
			if err := SinglePressKeyboard(ctx, SpecialKeyCodes["Delete"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", key)
			}
		case "PageDown":
			if err := SinglePressKeyboard(ctx, TopRowKeyCodes["Back"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", key)
			}
			if err := SinglePressKeyboard(ctx, TopRowKeyCodes["Back"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key after the second press ", key)
			}
		case "Up":
			if err := SinglePressKeyboard(ctx, TopRowKeyCodes["Back"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", key)
			}
		case "Down":
			if err := SinglePressKeyboard(ctx, TopRowKeyCodes["Back"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", key)
			}
		case "Left":
			if err := SinglePressKeyboard(ctx, TopRowKeyCodes["Back"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", key)
			}
		case "Right":
			if err := SinglePressKeyboard(ctx, TopRowKeyCodes["Back"].KeyCodeBytes, 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", key)
			}
		default:
			return errors.Wrapf(err, "cannot find the %s key", key)
		}
	}
	// Save the file.
	if _, err := kb.Accel(ctx, &inputs.AccelRequest{
		Key: "Ctrl+S",
	}); err != nil {
		return errors.Wrap(err, "failed to save the file using nano")
	}
	fs := dutfs.NewClient(cl.Conn)
	defer fs.Remove(ctx, testFilePath)
	// Close the file.
	if _, err := kb.Accel(ctx, &inputs.AccelRequest{
		Key: "Ctrl+X",
	}); err != nil {
		return errors.Wrap(err, "failed to close the file using nano")
	}
	testFile, err := fs.ReadFile(ctx, testFilePath)
	if err != nil {
		return errors.Wrap(err, "failed to read test file")
	}
	// Compare the contents of text files.
	if strings.TrimSpace(string(testFile)) != specialKeysTestAnswer {
		return errors.Wrap(err, "text file comparison failed")
	}
	return nil
}

// CheckTopRowkeys check the input and functionality of top row keys.
func CheckTopRowkeys(ctx context.Context, s *testing.State, device string) error {
	getVolume := func() (int64, error) {
		cmd := "cras_test_client | grep INTERNAL_SPEAKER | awk '{print $3}'"
		out, err := s.DUT().Conn().CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
		volume, err := strconv.ParseInt(strings.TrimSpace(string(out)), 10, 64)
		if err != nil {
			return 0, errors.Wrap(err, "string conversion to int64 failed")
		}
		return volume, nil
	}
	getBrightness := func() (float64, error) {
		out, err := s.DUT().Conn().CommandContext(ctx, "backlight_tool", "--get_brightness_percent").Output(testexec.DumpLogOnError)
		if err != nil {
			return 0, errors.Wrap(err, "failed to obtain brightness")
		}
		brightness, err := strconv.ParseFloat(strings.TrimSpace(string(out)), 64)
		if err != nil {
			return 0, errors.Wrap(err, "string conversion to float failed")
		}
		return brightness, nil
	}
	testKeyArr := []string{
		"VolumeUp",
		"VolumeDown",
		"brightnessUp",
		"brightnessDown",
	}
	for _, testKey := range testKeyArr {
		testing.ContextLog(ctx, "Pressing key ", testKey)
		volumeBefore, err := getVolume()
		brightnessBefore, err := getBrightness()
		if err != nil {
			errors.Wrap(err, "unable to obtain the before volume")
		}
		switch testKey {
		case "VolumeUp":
			if err := SinglePressKeyboard(ctx, MediaKeyCodes[testKey], 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", testKey)
			}
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				volumeAfter, err := getVolume()
				if err != nil {
					return errors.Wrap(err, "unable to obtain the after volume")
				}
				if volumeAfter <= volumeBefore {
					return errors.Errorf("got volume %d, want more than %d", volumeAfter, volumeBefore)
				}
				return nil
			}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
				return errors.Wrap(err, "volume increase failed")
			}
		case "VolumeDown":
			if err := SinglePressKeyboard(ctx, MediaKeyCodes[testKey], 100*time.Millisecond); err != nil {
				return errors.Wrapf(err, "unable to Press the %s Key", testKey)
			}
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				volumeAfter, err := getVolume()
				if err != nil {
					return errors.Wrap(err, "unable to obtain the after volume")
				}
				if volumeAfter >= volumeBefore {
					return errors.Errorf("got volume %d, want less than %d", volumeAfter, volumeBefore)
				}
				return nil
			}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
				return errors.Wrap(err, "volume reduction failed")
			}
		case "brightnessUp":
			CombineKeyPressKeyboard(ctx, 100*time.Millisecond, []int{SpecialKeyCodes["LWin"].KeyCodeInt, TopRowKeyCodes["F7"].KeyCodeInt})
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				brightnessAfter, err := getBrightness()
				if err != nil {
					errors.Wrap(err, "unable to obtain the after volume")
				}
				if brightnessAfter <= brightnessBefore {
					return errors.Errorf("got brightness %f, want more than %f", brightnessAfter, brightnessBefore)
				}
				return nil
			}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
				return errors.Wrap(err, "brightness increase failed")
			}
		case "brightnessDown":
			CombineKeyPressKeyboard(ctx, 100*time.Millisecond, []int{SpecialKeyCodes["LWin"].KeyCodeInt, TopRowKeyCodes["F6"].KeyCodeInt})
			if err := testing.Poll(ctx, func(ctx context.Context) error {
				brightnessAfter, err := getBrightness()
				if err != nil {
					errors.Wrap(err, "unable to obtain the after volume")
				}
				if brightnessAfter >= brightnessBefore {
					return errors.Errorf("got brightness %f, want less than %f", brightnessAfter, brightnessBefore)
				}
				return nil
			}, &testing.PollOptions{Timeout: 5 * time.Second, Interval: 500 * time.Millisecond}); err != nil {
				return errors.Wrap(err, "brightness reduction failed")
			}
		default:
			return errors.Wrapf(err, "cannot find the %s key", testKey)
		}
	}
	return nil
}

// GetDeviceEventKbd retrieves keyboards info from dev folder.
func GetDeviceEventKbd(ctx context.Context, dut *dut.DUT) ([]string, error) {
	lsInfo, err := dut.Conn().CommandContext(ctx, "ls", "/dev/input/by-id").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get devices event kbd info")
	}
	var kbdList []string
	for _, str := range strings.Split(strings.TrimSpace(string(lsInfo)), "\n") {
		if strings.Contains(str, "kbd") && !strings.Contains(str, "if01") {
			kbdList = append(kbdList, str)
		}
	}
	return kbdList, err
}

// GetDeviceEventMouse retrieves mice info from dev folder.
func GetDeviceEventMouse(ctx context.Context, dut *dut.DUT) ([]string, error) {
	lsInfo, err := dut.Conn().CommandContext(ctx, "ls", "/dev/input/by-id").Output(testexec.DumpLogOnError)
	if err != nil {
		return nil, errors.Wrap(err, "unable to get devices event mouse info")
	}
	var mouseList []string
	for _, str := range strings.Split(strings.TrimSpace(string(lsInfo)), "\n") {
		if strings.Contains(str, "event-mouse") {
			mouseList = append(mouseList, str)
		}
	}
	return mouseList, err
}

// LaunchCroshApp launch the crosh app.
func LaunchCroshApp(ctx context.Context, appsSvc pb.AppsServiceClient, uiautoSvc ui.AutomationServiceClient, kb inputs.KeyboardServiceClient) error {
	croshFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "ash/ShelfAppButton"}},
			{Value: &ui.NodeWith_Name{Name: "crosh"}},
		},
	}
	if _, err := appsSvc.LaunchApp(ctx, &pb.LaunchAppRequest{AppName: "crosh", TimeoutSecs: 60}); err != nil {
		return errors.Wrap(err, "failed to launch crosh app")
	}
	// Wait for the crosh app until it opens.
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: croshFinder}); err != nil {
		return errors.Wrap(err, "crosh app not exist")
	}
	// GoBigSleepLint: Wait for crosh to become ready for input.
	testing.Sleep(ctx, 2*time.Second)
	return nil
}

// CloseCroshApp close the crosh app.
func CloseCroshApp(ctx context.Context, uiautoSvc ui.AutomationServiceClient, kb inputs.KeyboardServiceClient) {
	croshFinder := &ui.Finder{
		NodeWiths: []*ui.NodeWith{
			{Value: &ui.NodeWith_HasClass{HasClass: "ash/ShelfAppButton"}},
			{Value: &ui.NodeWith_Name{Name: "crosh"}},
		},
	}
	if _, err := uiautoSvc.WaitUntilExists(ctx, &ui.WaitUntilExistsRequest{Finder: croshFinder}); err == nil {
		kb.Type(ctx, &inputs.TypeRequest{Key: "exit\nexit\n"})
	}
}
