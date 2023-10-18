// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ConfigChanges,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that configChanges property in AndroidManifest.xml prevents an activity to restart on the configuration update",
		Contacts:     []string{"arc-framework+tast@google.com", "yhanada@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Chrome Integration
		BugComponent: "b:537221",
		Attr:         []string{"informational", "group:mainline"},
		SoftwareDeps: []string{"android_container", "chrome"},
		Fixture:      "arcBooted",
		Timeout:      4 * time.Minute,
	})
}

func ConfigChanges(ctx context.Context, s *testing.State) {
	// Reserver 10 seconds for various cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	p := s.FixtValue().(*arc.PreData)
	cr := p.Chrome
	a := p.ARC
	d := p.UIDevice

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	infos, err := display.GetInfo(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get display info: ", err)
	}
	if len(infos) == 0 {
		s.Fatal("No display found")
	}
	var info *display.Info
	for i := range infos {
		if infos[i].IsInternal {
			info = &infos[i]
		}
	}
	if info == nil {
		s.Log("No internal display found. Default to the first display")
		info = &infos[0]
	}

	if info.Bounds.Height > info.Bounds.Width {
		originalAngle, err := display.RotationToAngle(info.Rotation)
		if err != nil {
			s.Fatal("Original display rotation info is invalid: ", err)
		}
		if err := display.SetDisplayRotationSync(ctx, tconn, info.ID, display.Rotate90); err != nil {
			s.Fatal("Failed to rotate display: ", err)
		}
		// Restore the initial rotation.
		defer display.SetDisplayRotationSync(cleanupCtx, tconn, info.ID, originalAngle)
	}

	const (
		apk = "ArcConfigChangesTest.apk"
		pkg = "org.chromium.arc.testapp.configchanges"
		cls = ".MainActivity"
	)

	s.Log("Installing app")
	if err := a.Install(ctx, arc.APKPath(apk)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	s.Log("Starting app")
	act, err := arc.NewActivity(a, pkg, cls)
	if err != nil {
		s.Fatal("Failed starting app: ", err)
	}
	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed starting app: ", err)
	}

	const (
		resumeCountID = "org.chromium.arc.testapp.configchanges:id/resume_count"
		buttonID      = "org.chromium.arc.testapp.configchanges:id/button"
	)

	resumeCount := d.Object(ui.ID(resumeCountID))
	if err := resumeCount.WaitForExists(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to find the label: ", err)
	}

	// Get how many times onResume() is called for this activity.
	initCount, err := resumeCount.GetText(ctx)
	if err != nil {
		s.Fatal("Failed to get text: ", err)
	}

	initBounds, err := act.WindowBounds(ctx)
	if err != nil {
		s.Fatal("Failed to get window bounds: ", err)
	}

	if err := d.Object(ui.ID(buttonID)).Click(ctx); err != nil {
		s.Fatal("Failed to click the button: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		updatedBounds, err := act.WindowBounds(ctx)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to get window bounds"))
		}
		if initBounds == updatedBounds {
			return errors.Errorf("window bounds did not change: %v", initBounds)
		}
		return nil
	}, nil); err != nil {
		s.Fatal("Failed to wait for the window bounds to change: ", err)
	}

	// In case the test is broken, the activity may be still relaunching at this point.
	// Wait for it to be relaunched.
	if err := resumeCount.WaitForExists(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to find the label: ", err)
	}

	updatedCount, err := resumeCount.GetText(ctx)
	if err != nil {
		s.Fatal("Failed to get text: ", err)
	}

	if initCount != updatedCount {
		s.Fatal("The activity relaunched between orientation change")
	}
}
