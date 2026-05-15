// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/wm"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:     ScreenPinning,
		Desc:     "Checks that screen pinning is entered and exits correctly",
		Contacts: []string{"arc-framework+tast@google.com", "yhanada@chromium.org"},
		// ChromeOS > Software > ARC++ > Framework > Window Management
		BugComponent: "b:537272",
		SoftwareDeps: []string{"chrome"},
		Fixture:      "arcBooted",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			}},
	})
}

const (
	toastTimeout    = 5 * time.Second
	activityTimeout = 10 * time.Second
)

func ScreenPinning(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC
	cr := s.FixtValue().(*arc.PreData).Chrome
	d := s.FixtValue().(*arc.PreData).UIDevice

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get instance of keyboard: ", err)
	}
	defer keyboard.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to set device to clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)

	// Install both needed apks.
	s.Log("Installing apps")
	if err := a.Install(ctx, arc.APKPath(wm.APKNameArcWMTestApp24)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}
	if err := a.Install(ctx, arc.APKPath(wm.ResizeLockApkName)); err != nil {
		s.Fatal("Failed installing app: ", err)
	}

	act, err := arc.NewActivity(a, wm.Pkg24, wm.MainActivity)
	if err != nil {
		s.Fatal(err, "failed to create a new MainActivity")
	}
	defer act.Close(cleanupCtx)

	if err := act.StartWithDefaultOptions(ctx, tconn); err != nil {
		s.Fatal("Failed to start the MainActivity: ", err)
	}
	defer act.Stop(cleanupCtx, tconn)

	// Ensure can start and stop other activities before pinning.
	if err := ensureCanNowStartAnotherActivity(ctx, a, tconn, activityTimeout); err != nil {
		s.Fatal("Could not start resize activity before pinning: ", err)
	}

	s.Log("Entering pinned mode")
	if err := wm.ActivatePinModeInWmTestApp(ctx, d); err != nil {
		s.Fatal("Activity did not enter pinned mode: ", err)
	}

	// Ensure both chrome and Android have consistent pinned state
	if err := ash.WaitForARCAppWindowState(ctx, tconn, wm.Pkg24, ash.WindowStatePinned); err != nil {
		s.Fatal("Window did not enter pinned mode in Arc VM: ", err)
	}
	if err := wm.WaitForChromeWindowState(ctx, tconn, ash.WindowStatePinned); err != nil {
		s.Fatal("Window did not enter pinned mode in Ash: ", err)
	}
	// Ensure android system is in Pinned state.
	if err := wm.WaitForLockControllerState(ctx, a, arc.TaskModePinned); err != nil {
		s.Fatal("Lock state did not enter pinned state: ", err)
	}

	// Try to launch another activity, should not work since we are pinned.
	s.Log("Ensuring second activity cannot launch")
	if err := ensureCannotStartAnotherActivity(ctx, a, tconn, activityTimeout); err != nil {
		s.Fatal("Could not properly ensure other activities don't launch while in pinned due to: ", err)
	}

	// Press key combination to exit pinned state.
	s.Log("Exiting pinned mode")
	// UNPIN accelerator as defined by
	// http://cs/h/chrome-internal/codesearch/chrome/src/+/main:ash/public/cpp/accelerators.cc?l=145
	if err := keyboard.AccelPress(ctx, "Shift+Search+Esc"); err != nil {
		s.Fatal("Keyboard combination of Shift+Search+Esc failed to be pressed: ", err)
	}

	// Ensure both chrome and Android have consistent unpinned state
	if err := ash.WaitForARCAppWindowState(ctx, tconn, wm.Pkg24, ash.WindowStateNormal); err != nil {
		s.Fatal("Window did not exit pinned mode in Arc VM: ", err)
	}
	if err := wm.WaitForLockControllerState(ctx, a, arc.TaskModeNone); err != nil {
		s.Fatal("Lock state did not exit pinned state: ", err)
	}
	if err := wm.WaitForChromeWindowState(ctx, tconn, ash.WindowStateNormal); err != nil {
		s.Fatal("Window did not exit pinned mode: ", err)
	}

	s.Log("Ensuring can now launch second activity")

	if err := ensureCanNowStartAnotherActivity(ctx, a, tconn, activityTimeout); err != nil {
		s.Fatal("Could not start resize activity after unpinning: ", err)
	}
}

func ensureCannotStartAnotherActivity(ctx context.Context, a *arc.ARC, tconn *chrome.TestConn, timeout time.Duration) error {
	resizeActivity, err :=
		arc.NewActivity(a, wm.ResizeLockTestPkgName, wm.ResizeLockUnresizablePortraitActivityName)
	if err != nil {
		return errors.Wrap(err, "failed to create a new MainActivity")
	}
	defer resizeActivity.Close(ctx)

	timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCtxCancel()

	if err := resizeActivity.Start(timeoutCtx, tconn); err == nil {
		resizeActivity.Stop(ctx, tconn)
		return errors.New("expected to fail to start another activity while in Pinned state")
	}

	return nil
}

func ensureCanNowStartAnotherActivity(ctx context.Context, a *arc.ARC, tconn *chrome.TestConn, timeout time.Duration) error {
	resizeActivity, err :=
		arc.NewActivity(a, wm.ResizeLockTestPkgName, wm.ResizeLockUnresizablePortraitActivityName)
	if err != nil {
		return errors.Wrap(err, "failed to create a new MainActivity")
	}
	defer resizeActivity.Close(ctx)

	timeoutCtx, timeoutCtxCancel := context.WithTimeout(ctx, timeout)
	defer timeoutCtxCancel()

	if err := resizeActivity.Start(timeoutCtx, tconn); err != nil {
		return errors.Wrap(err, "failed to start activity after exiting Pinned mode")
	}
	defer resizeActivity.Stop(ctx, tconn)
	if err := ash.WaitForVisible(ctx, tconn, resizeActivity.PackageName()); err != nil {
		return errors.Wrap(err, "package did not become visible")
	}

	return nil
}
