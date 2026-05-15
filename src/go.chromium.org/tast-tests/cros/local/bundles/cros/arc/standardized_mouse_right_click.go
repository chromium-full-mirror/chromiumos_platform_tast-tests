// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/standardizedtestutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StandardizedMouseRightClick,
		Desc: "Functional test that installs an app and tests standard mouse right click functionality. Tests are only performed in clamshell mode as tablets don't allow mice",
		Contacts: []string{
			"arc-framework+tast@google.com",
			"arc-engprod@google.com",
			"yhanada@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "no_chrome_dcheck"},
		Timeout:      10 * time.Minute,
		Fixture:      "arcBooted",
		Params: []testing.Param{{
			Name: "vm",
			Val: standardizedtestutil.GetClamshellTest(
				runStandardizedMouseRightClickTest,
				standardizedtestutil.WithMouse(),
			),
			ExtraSoftwareDeps: []string{"android_vm"},
			ExtraHardwareDeps: hwdep.D(standardizedtestutil.ClamshellHardwareDep),
		}},
	})
}

func StandardizedMouseRightClick(ctx context.Context, s *testing.State) {
	const (
		apkName      = "ArcStandardizedInputTest.apk"
		appName      = "org.chromium.arc.testapp.arcstandardizedinputtest"
		activityName = ".PointerRightClickTestActivity"
	)

	t := s.Param().(standardizedtestutil.Test)
	standardizedtestutil.RunTest(ctx, s, apkName, appName, activityName, t)
}

func runStandardizedMouseRightClickTest(ctx context.Context, testParameters standardizedtestutil.TestFuncParams) error {
	btnRightClickID := testParameters.AppPkgName + ":id/btnRightClick"
	btnRightClickSelector := testParameters.Device.Object(ui.ID(btnRightClickID))

	// Setup the mouse.
	mouse := testParameters.Mouse

	if err := btnRightClickSelector.WaitForExists(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "unable to find the button to click")
	}

	if err := testParameters.Device.Object(ui.Text("POINTER RIGHT CLICK (1)")).WaitUntilGone(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "the success label should not yet exist")
	}

	if err := standardizedtestutil.MouseClickObject(ctx, testParameters, btnRightClickSelector, mouse, standardizedtestutil.RightPointerButton); err != nil {
		return errors.Wrap(err, "unable to click the button")
	}

	if err := testParameters.Device.Object(ui.Text("POINTER RIGHT CLICK (1)")).WaitForExists(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "the success label should exist")
	}

	if err := testParameters.Device.Object(ui.Text("POINTER RIGHT CLICK (2)")).WaitUntilGone(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "a single mouse click triggered two click events")
	}

	return nil
}
