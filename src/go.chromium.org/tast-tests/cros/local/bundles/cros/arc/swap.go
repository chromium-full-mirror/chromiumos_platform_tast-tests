// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/swap"
	"go.chromium.org/tast-tests/cros/local/arc/vm"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         Swap,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that vmm-swap is correctly enabled/disabled based on app state",
		Contacts: []string{
			"cros-vm-technology@google.com",
			"kawasin@google.com",
			"hikalium@chromium.org",
		},
		// ChromeOS > Platform > Virtualization > VM Technology
		BugComponent: "b:930563",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"android_vm", "chrome", "crosvm_swap"},
		Fixture:      "arcBootedBoostedVmmSwap",
		Timeout:      chrome.LoginTimeout + arc.BootTimeout + 2*time.Minute,
	})
}

func Swap(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*arc.PreData).Chrome
	a := s.FixtValue().(*arc.PreData).ARC

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	if err := a.Install(ctx, arc.APKPath(swap.TestApk)); err != nil {
		s.Fatal("Failed to install the APK: ", err)
	}

	socketPath, err := vm.SocketPath()
	if err != nil {
		s.Fatal("Failed to get crosvm sock: ", err)
	}

	// No Android apps are open on login, so the current fixture's
	// settings should ensure swap is enabled more-or-less immediately.
	s.Log("Waiting for swap to be enabled")
	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Pending}); err != nil {
		s.Fatal("Failed to wait for swap to be enabled: ", err)
	}

	// Launch the test app and check that swap gets disabled
	if err := launcher.LaunchApp(tconn, swap.AppName /* short name should be same as AppName*/)(ctx); err != nil {
		s.Fatal("Failed to launch: ", err)
	}

	s.Log("Waiting for swap to be disabled")
	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Ready}); err != nil {
		s.Fatal("Failed to wait for swap to be enabled: ", err)
	}

	ui := uiauto.New(tconn)
	appButton := nodewith.Name(swap.AppName).ClassName("ash/ShelfAppButton")
	if err := uiauto.Combine("minimize ARC window",
		ui.WaitUntilExists(appButton),
		ui.LeftClick(appButton))(ctx); err != nil {
		s.Fatal("Failed to minimize app: ", err)
	}

	s.Log("Checking that swap is still disabled after a delay")
	// GoBigSleepLint: Verify that swap remains disabled when the app window is
	// minimized. The best we can do for a negative check like this is sleep and
	// then check.
	testing.Sleep(ctx, 5*time.Second)
	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Ready}); err != nil {
		s.Fatal("Failed to wait for swap to be enabled: ", err)
	}

	// Stop the minimized activity and verify that swap is re-enabled
	activity, err := arc.NewActivity(a, swap.Pkg, swap.Activity)
	if err != nil {
		s.Fatal("Failed to create activity object: ", err)
	}

	if err := activity.Stop(ctx, tconn); err != nil {
		s.Fatal("Failed to stop activity: ", err)
	}

	s.Log("Waiting for swap to be re-enabled")
	if err := swap.WaitForStatus(ctx, socketPath, []swap.Status{swap.Pending}); err != nil {
		s.Fatal("Failed to wait for swap to be enabled: ", err)
	}
}
