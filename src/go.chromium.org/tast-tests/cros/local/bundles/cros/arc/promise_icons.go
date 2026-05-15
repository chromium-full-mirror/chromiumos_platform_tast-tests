// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arc supports interacting with the ARC framework, which is used to run Android applications on Chrome OS.
package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: PromiseIcons,
		Desc: "Verifies that a Play Store installation creates a promise icon in Launcher",
		Contacts: []string{
			"lt-web-apps-team@google.com",
		},
		BugComponent: "b:1389907",
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{
			"chrome",
			"gaia",
		},
		HardwareDeps: hwdep.D(hwdep.MinStorage(16)),
		Params: []testing.Param{
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
		Fixture: "arcBootedWithPlayStore",
		Timeout: chrome.LoginTimeout + arc.BootTimeout + 4*time.Minute,
	})
}

// PromiseIcons test checks for the presence of a promise icon in the Launcher when a Play Store installation starts.
func PromiseIcons(ctx context.Context, s *testing.State) {
	const (
		// The test app must be:
		// 1. Compatible with all devices, including the Betty VM and devices
		// without touchscreens, or the Play Store will have a "This Chromebook
		// isn't compatible with this app" error instead of an install button.
		// 2. Large enough that install is not less than 20 seconds.
		packageName     = "com.kingsgroup.sos"
		installingLabel = "State of Survival: Zombie War, installing"
	)

	cr := s.FixtValue().(*arc.PreData).Chrome
	arcDevice := s.FixtValue().(*arc.PreData).ARC
	uiAutomator := s.FixtValue().(*arc.PreData).UIDevice

	// Give 5 seconds to clean up and dump out UI tree.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}
	handler := faillog.DumpUITreeWithScreenshotHandler(cleanupCtx, tconn, "chrome_ui_tree")
	s.AttachErrorHandlers(handler, handler)

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn, cr)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	s.Log("Start app installation in Play Store")
	if err := playstore.BeginAppInstall(ctx, arcDevice, uiAutomator, packageName, &playstore.Options{TryLimit: -1}); err != nil {
		s.Fatal("Failed to start app installation in Play Store: ", err)
	}
	defer arcDevice.Uninstall(ctx, packageName)

	s.Log("Open the Launcher")
	if err := launcher.Open(tconn)(ctx); err != nil {
		s.Fatal("Failed to open launcher: ", err)
	}

	if err := ui.WaitUntilExists(launcher.AppItemViewFinder(installingLabel))(ctx); err != nil {
		s.Fatal("Failed to find item with correct label in Launcher: ", err)
	}
}
