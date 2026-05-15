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
		Func: StandardizedTrackpadZoom,
		Desc: "Functional test that installs an app and tests standard trackpad zoom in and zoom out functionality. Tests are only performed in clamshell mode as tablets don't support the trackpad",
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
				Val:               standardizedtestutil.GetClamshellTest(runStandardizedTrackpadZoomTest),
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraHardwareDeps: hwdep.D(standardizedtestutil.ClamshellHardwareDep),
			}},
	})
}

func StandardizedTrackpadZoom(ctx context.Context, s *testing.State) {
	const (
		apkName      = "ArcStandardizedInputTest.apk"
		appName      = "org.chromium.arc.testapp.arcstandardizedinputtest"
		activityName = ".ZoomTestActivity"
	)

	t := s.Param().(standardizedtestutil.Test)
	standardizedtestutil.RunTest(ctx, s, apkName, appName, activityName, t)
}

func runStandardizedTrackpadZoomTest(ctx context.Context, testParameters standardizedtestutil.TestFuncParams) error {
	txtZoomID := testParameters.AppPkgName + ":id/txtZoom"
	txtZoomSelector := testParameters.Device.Object(ui.ID(txtZoomID))

	txtZoomInStateID := testParameters.AppPkgName + ":id/txtZoomInState"
	zoomInSuccessLabelSelector := testParameters.Device.Object(ui.ID(txtZoomInStateID), ui.Text("ZOOM IN: COMPLETE"))

	txtZoomOutStateID := testParameters.AppPkgName + ":id/txtZoomOutState"
	zoomOutSuccessLabelSelector := testParameters.Device.Object(ui.ID(txtZoomOutStateID), ui.Text("ZOOM OUT: COMPLETE"))

	trackpad, err := input.Trackpad(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to initialize the trackpad")
	}
	defer trackpad.Close(ctx)

	if err := txtZoomSelector.WaitForExists(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to find the element to zoom in on")
	}

	// No labels should be in their complete state before the tests begin.
	if err := zoomInSuccessLabelSelector.WaitUntilGone(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to verify the zoom in success label does not yet exist")
	}

	if err := zoomOutSuccessLabelSelector.WaitUntilGone(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to verify the zoom out success label does not yet exist")
	}

	// After the zoom in, only the zoom in label should be in the success state.
	if err := standardizedtestutil.TrackpadZoom(ctx, trackpad, testParameters, txtZoomSelector, standardizedtestutil.ZoomIn); err != nil {
		return errors.Wrap(err, "failed to perform the zoom")
	}

	if err := zoomInSuccessLabelSelector.WaitForExists(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to verify the zoom in success label exists")
	}

	if err := zoomOutSuccessLabelSelector.WaitUntilGone(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to verify the zoom out success label does not yet exist")
	}

	// After the zoom out, all zoom labels should be in the success state.
	if err := standardizedtestutil.TrackpadZoom(ctx, trackpad, testParameters, txtZoomSelector, standardizedtestutil.ZoomOut); err != nil {
		return errors.Wrap(err, "failed to perform the zoom")
	}

	if err := zoomInSuccessLabelSelector.WaitForExists(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to verify the zoom in success label exists")
	}

	if err := zoomOutSuccessLabelSelector.WaitForExists(ctx, standardizedtestutil.ShortUITimeout); err != nil {
		return errors.Wrap(err, "failed to verify the zoom out success label exists")
	}

	return nil
}
