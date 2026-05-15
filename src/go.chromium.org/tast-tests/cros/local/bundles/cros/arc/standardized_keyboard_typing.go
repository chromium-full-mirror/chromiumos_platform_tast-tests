// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/standardizedtestutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StandardizedKeyboardTyping,
		Desc: "Functional test that installs an app and tests standard keyboard typing functionality. Test are performed in clamshell and touchview mode. This does not test the virtual, on-screen keyboard",
		Contacts: []string{
			"arc-framework+tast@google.com",
			"arc-engprod@google.com",
			"yhanada@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "group:input-tools"},
		SoftwareDeps: []string{"chrome", "no_chrome_dcheck"},
		Timeout:      10 * time.Minute,
		// Disabled temporarily on Kukui Devices due to test flakiness with FakeKeyboardHeuristic (b/245854219).
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("kakadu", "katsu", "kodama")),
		Fixture:      "arcBooted",
		Params: []testing.Param{
			{
				Name:              "vm",
				Val:               standardizedtestutil.GetClamshellTest(runStandardizedKeyboardTypingTest),
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(standardizedtestutil.ClamshellHardwareDep),
			}, {
				Name:              "vm_tablet_mode",
				Val:               standardizedtestutil.GetTabletTest(runStandardizedKeyboardTypingTest),
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(standardizedtestutil.TabletHardwareDep),
			}},
	})
}

// StandardizedKeyboardTyping runs all the provided test cases.
func StandardizedKeyboardTyping(ctx context.Context, s *testing.State) {
	const (
		apkName      = "ArcStandardizedInputTest.apk"
		appName      = "org.chromium.arc.testapp.arcstandardizedinputtest"
		activityName = ".TypingTestActivity"
	)

	t := s.Param().(standardizedtestutil.Test)
	standardizedtestutil.RunTest(ctx, s, apkName, appName, activityName, t)
}

// runStandardizedKeyboardTypingTest types into the input field, and ensures the text appears.
// This does not use the virtual, on screen keyboard.
func runStandardizedKeyboardTypingTest(ctx context.Context, testParameters standardizedtestutil.TestFuncParams) error {
	kbd, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "unable to create virtual keyboard")
	}
	defer kbd.Close(ctx)

	textKeyboardInputID := testParameters.AppPkgName + ":id/textKeyboardInput"
	const textForTest = "abcdEFGH0123!@#$"

	if err := standardizedtestutil.ClickInputAndGuaranteeFocus(ctx, testParameters, ui.ID(textKeyboardInputID)); err != nil {
		return errors.Wrap(err, "unable to focus the input")
	}

	if err := kbd.Type(ctx, textForTest); err != nil {
		return errors.Wrapf(err, "unable to type: %v", textForTest)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if text, err := testParameters.Device.Object(ui.ID(textKeyboardInputID)).GetText(ctx); err != nil {
			return errors.Wrapf(err, "unable to get text from object id %q", textKeyboardInputID)
		} else if text != textForTest {
			return errors.Errorf("typed text is not expected. got: %q, want: %q", text, textForTest)
		}
		return nil
	}, &testing.PollOptions{Timeout: standardizedtestutil.ShortUITimeout}); err != nil {
		return errors.Wrapf(err, "unable to confirm %v was typed", textForTest)
	}

	return nil
}
