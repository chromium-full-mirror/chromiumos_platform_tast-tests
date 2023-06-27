// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/camera/arcapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCCameraOrientation,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Ensures that camera orientation compatibility solution works as expected",
		Contacts:     []string{"chromeos-camera-eng@google.com", "shik@chromium.org"},
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Fixture:      "arcWithWorkingCamera",
		Data:         []string{arcapp.CameraAppApk},
		Timeout:      4 * time.Minute,
		Params: []testing.Param{{
			ExtraSoftwareDeps: []string{"android_p"},
		}, {
			Name:              "vm",
			ExtraSoftwareDeps: []string{"android_vm"},
		}},
		BugComponent: "b:978428",
	})
}

func ARCCameraOrientation(ctx context.Context, s *testing.State) {
	// Give cleanup actions a minute to run, even if we fail by exceeding our
	// deadline.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get Test API connection: ", err)
	}

	// The testing app expects to be launched in the clamshell mode.
	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure in clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	a := s.FixtValue().(*arc.PreData).ARC
	// Install camera testing app.
	if err := a.Install(ctx, s.DataPath(arcapp.CameraAppApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	s.Log("Starting app")
	cleanupAppFunc, err := arcapp.LaunchOrientationTestApp(ctx, a, tconn)
	if err != nil {
		s.Fatal("Failed to launch ARC camera app: ", err)
	}
	defer cleanupAppFunc(cleanupCtx, tconn)

	if err := arcapp.StartOrientationTest(ctx, a); err != nil {
		s.Fatal("Failed to start orientation test: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if passed, err := arcapp.OrientationTestPassed(ctx, a); err != nil {
			return errors.Wrap(err, "failed to wait for the orientation test finished")
		} else if !passed {
			return testing.PollBreak(errors.New("failed to pass orientation test"))
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Second}); err != nil {
		s.Fatal("Failed to pass orientation test: ", err)
	}
}
