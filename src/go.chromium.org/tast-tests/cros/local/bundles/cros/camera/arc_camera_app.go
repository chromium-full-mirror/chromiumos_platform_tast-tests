// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/media/caps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/camera/arcapp"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/upstart"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ARCCameraApp,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Data:         []string{arcapp.CameraAppApk},
		Desc:         "Checks basic Android camera functionalities work under ARC",
		Contacts:     []string{"chromeos-camera-eng@google.com", "seannli@google.com"},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", caps.BuiltinOrVividCamera},
		Fixture:      "arcWithWorkingCamera",
		BugComponent: "b:978428",
	})
}

func ARCCameraApp(ctx context.Context, s *testing.State) {
	// Ensure camera service running to avoid bad state from previous tests.
	if err := upstart.EnsureJobRunning(ctx, "cros-camera"); err != nil {
		s.Fatal("Failed to start cros-camera: ", err)
	}

	cr := s.FixtValue().(*arc.PreData).Chrome
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	a := s.FixtValue().(*arc.PreData).ARC
	if err := a.Install(ctx, s.DataPath(arcapp.CameraAppApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	// Prepare host-side access to Android's SDCard partition, which should store the generated photo and video files.
	cleanupFunc, err := arc.MountSDCardPartitionOnHostWithSSHFSIfVirtioBlkDataEnabled(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to make Android's SDCard partition available on host: ", err)
	}
	defer cleanupFunc(cleanupCtx)

	subTestTimeout := 60 * time.Second
	for _, tst := range []struct {
		name     string
		testFunc func(context.Context, *chrome.Chrome, *arc.ARC) error
	}{{
		"take_photo",
		arcapp.TakePhoto,
	}, {
		"record_video",
		recordVideo,
	}} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tst.name, func(ctx context.Context, s *testing.State) {
			cleanupCtx := ctx
			ctx, cancelCleanup := ctxutil.Shorten(ctx, 3*time.Second)
			defer cancelCleanup()

			cleanupFunc, err := arcapp.LaunchARCCameraApp(ctx, a, tconn)
			if err != nil {
				s.Fatal("Failed to launch ARC camera app: ", err)
			}
			defer cleanupFunc(cleanupCtx, tconn)

			numOfCameras, err := arcapp.GetNumOfCameras(ctx, a)
			if err != nil {
				s.Fatal("Failed to get number of cameras: ", err)
			}
			for i := 0; i < numOfCameras; i++ {
				if i > 0 {
					testing.ContextLog(ctx, "Switch camera")
					if err := arcapp.SwitchCamera(ctx, a); err != nil {
						s.Fatal("Failed to switch camera: ", err)
					}
				}

				testing.ContextLog(ctx, "Testing camera ", i)
				if err := tst.testFunc(ctx, cr, a); err != nil {
					s.Fatalf("Failed when running sub test %v: %v", tst.name, err)
				}
			}
		})
		cancel()
	}
}

// recordVideo test if the video recording works via ARC camera test app.
func recordVideo(ctx context.Context, cr *chrome.Chrome, a *arc.ARC) error {
	if err := arcapp.StartRecording(ctx, cr, a); err != nil {
		return errors.Wrap(err, "failed to start recording")
	}

	// GoBigSleepLint: Record the video for 5 seconds.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		return errors.Wrap(err, "failed to sleep")
	}

	return arcapp.StopRecording(ctx, cr, a)
}
