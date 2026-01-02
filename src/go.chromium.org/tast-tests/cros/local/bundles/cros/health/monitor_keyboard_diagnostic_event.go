// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package health

import (
	"bufio"
	"context"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/diagnosticsapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/diagnosticsutils"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type keyboardEventTestParams struct {
	// Whether to force clamshell mode during the setup.
	forceClamshellMode bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: MonitorKeyboardDiagnosticEvent,
		Desc: "Monitors the keyboard diagnostic event detected properly or not",
		Contacts: []string{
			"cros-tdm-tpe-eng@google.com",
			"weiluanwang@google.com",
		},
		// ChromeOS > Platform > Enablement > Serviceability > Diagnostic & Health
		BugComponent: "b:982097",
		Attr:         []string{"group:mainline"},
		SoftwareDeps: []string{"diagnostics", "chrome"},
		// Form factors with internal keyboards:
		//  - Clamshell
		//  - Convertible
		//  - Detachable
		HardwareDeps: hwdep.D(hwdep.InternalKeyboard()),
		Fixture:      "crosHealthdRunning",
		Params: []testing.Param{{
			Name: "non_tablet_mode_form_factors",
			Val: keyboardEventTestParams{
				forceClamshellMode: false,
			},
			// The keyboard diagnostics is disabled for split modifier keyboard.
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Clamshell), hwdep.NoSplitModifierKeyboard()),
			ExtraAttr:         []string{"informational"},
		}, {
			Name: "tablet_mode_form_factors",
			Val: keyboardEventTestParams{
				forceClamshellMode: true,
			},
			// The keyboard diagnostics is disabled for split modifier keyboard.
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible, hwdep.Detachable), hwdep.NoSplitModifierKeyboard(), hwdep.SkipOnModel("kracko360")),
			ExtraAttr:         []string{"informational"},
		}, {
			// TODO(b/363140028): fix the failures on kracko360.
			Name: "tablet_mode_form_factors_unstable",
			Val: keyboardEventTestParams{
				forceClamshellMode: true,
			},
			ExtraHardwareDeps: hwdep.D(hwdep.FormFactor(hwdep.Convertible, hwdep.Detachable), hwdep.Model("kracko360")),
			ExtraAttr:         []string{"informational"},
		}},
	})
}

func triggerKeyboardDiagnosticEvent(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) error {
	ui := uiauto.New(tconn)

	// Check that the keyboard tester on the keyboard page is shown.
	keyboardTesterDoneButton := nodewith.NameContaining("Done").Role(role.Button)
	if err := ui.WaitUntilExists(keyboardTesterDoneButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to open keyboard tab with the keyboard tester")
	}

	// Press an arbitrary key.
	if err := kb.Accel(ctx, "Enter"); err != nil {
		return errors.Wrap(err, "failed to press a key")
	}

	// Press the "Done" button to exit the keyboard tester and emit a keyboard
	// diagnostic event.
	if err := ui.WithPollOpts(testing.PollOptions{Interval: time.Second, Timeout: 5 * time.Second}).LeftClick(keyboardTesterDoneButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the done button")
	}

	return nil
}

func verifyKeyboardDiagnosticEvent(eventLine string) error {
	re := regexp.MustCompile(`Keyboard diagnostic event received: the keybaord ".*" got (\d+) key\(s\) pressed.`)
	m := re.FindStringSubmatch(eventLine)

	if len(m) < 2 {
		return errors.Errorf("failed to detect the event: %s", eventLine)
	}

	numKeyPressed, err := strconv.Atoi(m[1])
	if err != nil {
		return errors.Errorf("unable to convert string to int: %s", m[1])
	}

	if numKeyPressed != 1 {
		return errors.Errorf("unexpected number of key pressed: got %d; want 1", numKeyPressed)
	}

	return nil
}

func MonitorKeyboardDiagnosticEvent(ctx context.Context, s *testing.State) {
	testParam := s.Param().(keyboardEventTestParams)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx, chrome.GuestLogin())
	if err != nil {
		s.Fatal("Failed to create Chrome instance: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect Test API: ", err)
	}

	if testParam.forceClamshellMode {
		cleanup, err := diagnosticsutils.EnsureClamshellMode(ctx, tconn)
		if err != nil {
			s.Fatal("Failed to ensure in clamshell mode: ", err)
		}
		defer cleanup(cleanupCtx)
	}

	// Prepare the keyboard tester page before start listening to events
	// because the UI may take a long time to display.
	if _, err := diagnosticsapp.Launch(ctx, tconn); err != nil {
		s.Fatal("Failed to launch diagnostics app: ", err)
	}
	if err := diagnosticsapp.OpenKeyboardTester(ctx, tconn); err != nil {
		s.Fatal("Failed to open keyboard tester: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to open the keyboard device: ", err)
	}
	defer kb.Close(cleanupCtx)

	// Run monitor command in background.
	monitorCmd := testexec.CommandContext(
		ctx,
		"cros-health-tool",
		"event",
		"--category=keyboard_diagnostic",
		"--length_seconds=20",
	)
	stdoutPipe, err := monitorCmd.StdoutPipe()
	if err != nil {
		s.Fatal("Failed to create stdout pipe: ", err)
	}
	if err := monitorCmd.Start(); err != nil {
		s.Fatal("Failed to run healthd monitor command: ", err)
	}
	defer monitorCmd.Wait(testexec.DumpLogOnError)

	scanner := bufio.NewScanner(stdoutPipe)

	// Wait for the subscription success message.
	if !scanner.Scan() {
		s.Fatal("Failed to scan next line for success message: ", scanner.Err())
	} else if line := scanner.Text(); !strings.HasPrefix(line, "Subscribe to keyboard_diagnostic events successfully") {
		s.Fatal("Failed to subscribe event in healthd:", line)
	}

	if err := triggerKeyboardDiagnosticEvent(ctx, tconn, kb); err != nil {
		s.Fatal("Failed to trigger keyboard diagnostic event: ", err)
	}

	if !scanner.Scan() {
		s.Fatal("Failed to scan next line: ", scanner.Err())
	} else if err := verifyKeyboardDiagnosticEvent(scanner.Text()); err != nil {
		s.Fatal("Keyboard diagnostic event verification failed: ", err)
	}

	if err := monitorCmd.Kill(); err != nil {
		s.Fatal("Failed to kill the event monitoring process: ", err)
	}
}
