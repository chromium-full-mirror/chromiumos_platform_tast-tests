// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package inputs

import (
	"bufio"
	"context"
	"io"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/useractions"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/inputs/fixture"
	"go.chromium.org/tast-tests/cros/local/inputs/testserver"
	"go.chromium.org/tast-tests/cros/local/inputs/util"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PhysicalKeyboardAlphanumeric,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Physical Keyboard alphanumeric keys are Functional",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Attr:         []string{"group:intel-nda"},
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"inputs_deps", "chrome"},
		Fixture:      fixture.ClamshellNonVK,
	})
}

const (
	typeTimeout = 500 * time.Millisecond
	keyPressDur = 100 * time.Millisecond
	keyValues   = "`1234567890-=[];',./abcdefghijklmnopqrstuvwxyz "
)

func PhysicalKeyboardAlphanumeric(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn
	uc := s.FixtValue().(fixture.FixtData).UserContext

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open Keyboard device: ", err)
	}
	defer kb.Close(ctx)

	devPath, err := deviceCheck(ctx)
	if err != nil {
		s.Fatal("Failed to get Keyboard scanner: ", err)
	}
	cmd, stdout, err := invokeEVTest(ctx, devPath)
	if err != nil {
		s.Fatal("Failed to invoke EvTest: ", err)
	}
	defer cmd.Wait()
	defer cmd.Kill()
	testKeyMap := map[string]string{
		"back":           "KEY_BACK",
		"refresh":        "KEY_REFRESH",
		"scale":          "KEY_SCALE",
		"brightnessdown": "KEY_BRIGHTNESSDOWN",
		"brightnessup":   "KEY_BRIGHTNESSUP",
		"mute":           "KEY_MUTE",
		"volumedown":     "KEY_VOLUMEDOWN",
		"volumeup":       "KEY_VOLUMEUP",
		"esc":            "KEY_ESC",

		"1":  "KEY_1",
		"2":  "KEY_2",
		"3":  "KEY_3",
		"4":  "KEY_4",
		"5":  "KEY_5",
		"6":  "KEY_6",
		"7":  "KEY_7",
		"8":  "KEY_8",
		"9":  "KEY_9",
		"0":  "KEY_0",
		"-":  "KEY_MINUS",
		"=":  "KEY_EQUAL",
		"q":  "KEY_Q",
		"w":  "KEY_W",
		"e":  "KEY_E",
		"r":  "KEY_R",
		"t":  "KEY_T",
		"y":  "KEY_Y",
		"u":  "KEY_U",
		"i":  "KEY_I",
		"o":  "KEY_O",
		"p":  "KEY_P",
		"[":  "KEY_LEFTBRACE",
		"]":  "KEY_RIGHTBRACE",
		"a":  "KEY_A",
		"s":  "KEY_S",
		"d":  "KEY_D",
		"f":  "KEY_F",
		"g":  "KEY_G",
		"h":  "KEY_H",
		"j":  "KEY_J",
		"k":  "KEY_K",
		"l":  "KEY_L",
		";":  "KEY_SEMICOLON",
		"`":  "KEY_GRAVE",
		"\\": "KEY_BACKSLASH",
		"z":  "KEY_Z",
		"x":  "KEY_X",
		"c":  "KEY_C",
		"v":  "KEY_V",
		"b":  "KEY_B",
		"n":  "KEY_N",
		"m":  "KEY_M",
		",":  "KEY_COMMA",
		".":  "KEY_DOT",
		"/":  "KEY_SLASH",

		"alt":    "KEY_LEFTALT",
		"ctrl":   "KEY_LEFTCTRL",
		"search": "KEY_LEFTMETA",
		"shift":  "KEY_LEFTSHIFT",
		"altgr":  "KEY_RIGHTALT",

		"backspace": "KEY_BACKSPACE",
		"enter":     "KEY_ENTER",
		"space":     "KEY_SPACE",
		"tab":       "KEY_TAB",
	}
	scannerTouchscreen := bufio.NewScanner(stdout)

	for key, keycode := range testKeyMap {

		if err := verifyKeyPress(ctx, kb, scannerTouchscreen, key, keycode); err != nil {
			s.Fatalf("Failed to verify key press %s: %v", key, err)
		}
	}

	its, err := testserver.LaunchBrowser(ctx, s.FixtValue().(fixture.FixtData).BrowserType, cr, tconn)
	if err != nil {
		s.Fatal("Failed to launch inputs test server: ", err)
	}
	inputField := testserver.TextAreaInputField

	defer its.CloseAll(cleanupCtx)

	actionName := "keyboard check"
	if err := uiauto.UserAction(actionName,
		uiauto.Combine(actionName,
			its.Clear(inputField),
			its.ClickFieldAndWaitForActive(inputField),
			kb.TypeAction(keyValues),
			util.WaitForFieldTextToBe(tconn, inputField.Finder(), keyValues),
		),
		uc,
		&useractions.UserActionCfg{
			Attributes: map[string]string{
				useractions.AttributeFeature: useractions.FeaturePKTyping,
			},
		},
	)(ctx); err != nil {
		s.Fatal("Failed to validate Key Press: ", err)
	}

	keyCombinations := []string{"ctrl+alt+refresh", "ctrl+alt+back", "ctrl+n", "ctrl+t", "ctrl+shift+n", "ctrl+w", "ctrl+r", "ctrl+alt+t"}

	for _, key := range keyCombinations {
		if err := kb.Accel(ctx, key); err != nil {
			s.Fatalf("Failed to press combination key  %s: %v", key, err)
		}
	}
}

// verifyKeyPress presses all the key and verifies using evtest.
func verifyKeyPress(ctx context.Context, kb *input.KeyboardEventWriter, scanner *bufio.Scanner, key, keycode string) error {
	keyPressEventPattern := `Event.*time.*code\s(\d*)\s\(` + keycode + `\)`
	keyPressEventRegex := regexp.MustCompile(keyPressEventPattern)

	text := make(chan string)
	go func() {
		defer close(text)
		for scanner.Scan() {
			text <- scanner.Text()
		}
	}()
	start := time.Now()
	if err := kb.Accel(ctx, key); err != nil {
		return errors.Wrap(err, "failed to press key")
	}

	for {
		select {
		case <-ctx.Done():
			return errors.New("did not detect keyboard event within expected time")
		case out := <-text:
			if match := keyPressEventRegex.FindStringSubmatch(out); match != nil {
				testing.ContextLogf(ctx, "key pressed detected in %s: %v", time.Since(start)-keyPressDur, match)
				return nil
			}
		}
	}
}

// deviceCheck returns the evtest scanner for the Keyboard device.
func deviceCheck(ctx context.Context) (string, error) {
	foundKB, devPath, err := input.FindPhysicalKeyboard(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to find device path for the keyboard")
	}
	if !foundKB {
		return "", errors.New("failed to find physical keyboard")
	}
	return devPath, nil
}

func invokeEVTest(ctx context.Context, devPath string) (*testexec.Cmd, io.Reader, error) {
	cmd := testexec.CommandContext(ctx, "evtest", devPath)
	stdout, err := cmd.StdoutPipe()
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to create stdout pipe")
	}
	if err := cmd.Start(); err != nil {
		return nil, nil, errors.Wrap(err, "failed to start scanner")
	}
	return cmd, stdout, nil
}
