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
		Func: StandardizedTrackpadLeftClick,
		Desc: "Functional test that installs an app and tests standard trackpad left click functionality. Tests are only performed in clamshell mode as tablets don't support the trackpad",
		Contacts: []string{
			"arc-framework+tast@google.com",
			"arc-engprod@google.com",
			"yhanada@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Framework > Input
		BugComponent: "b:536706",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "no_chrome_dcheck"},
		Timeout:      10 * time.Minute,
		Fixture:      "arcBooted",
		Params: []testing.Param{
			{
				Name:              "vm",
				Val:               standardizedtestutil.GetClamshellTest(runStandardizedTrackpadLeftClickTest),
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(standardizedtestutil.ClamshellHardwareDep),
			},
		},
	})
}

func StandardizedTrackpadLeftClick(ctx context.Context, s *testing.State) {
	const (
		apkName      = "ArcStandardizedInputTest.apk"
		appName      = "org.chromium.arc.testapp.arcstandardizedinputtest"
		activityName = ".PointerLeftClickTestActivity"
	)

	t := s.Param().(standardizedtestutil.Test)
	standardizedtestutil.RunTest(ctx, s, apkName, appName, activityName, t)
}

func runStandardizedTrackpadLeftClickTest(ctx context.Context, testParameters standardizedtestutil.TestFuncParams) error {
	btnLeftClickID := testParameters.AppPkgName + ":id/btnLeftClick"
	btnLeftClickSelector := testParameters.Device.Object(ui.ID(btnLeftClickID))

	trackpad, err := input.Trackpad(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to setup the trackpad")
	}
	defer trackpad.Close(ctx)

	if err := btnLeftClickSelector.WaitForExists(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to find the button to click")
	}

	if err := standardizedtestutil.TrackpadClickObject(ctx, testParameters, btnLeftClickSelector, trackpad, standardizedtestutil.LeftPointerButton); err != nil {
		return errors.Wrap(err, "failed to click the button")
	}

	if err := testParameters.Device.Object(ui.Text("POINTER LEFT CLICK (1)")).WaitForExists(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to verify success")
	}

	if err := testParameters.Device.Object(ui.Text("POINTER LEFT CLICK (2)")).WaitUntilGone(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to verify only one click event was fired")
	}

	return nil
}
