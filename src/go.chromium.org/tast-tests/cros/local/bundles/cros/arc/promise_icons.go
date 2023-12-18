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
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PromiseIcons,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that a Play Store installation creates a promise icon in Launcher",
		Contacts: []string{
			"chromeos-apps-foundation-team@google.com",
			"vpao@google.com",
		},
		BugComponent: "crbug:Platform>Apps>Foundation>AppService",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{
			{
				// Promise Icons requires Android R+ (android_container which includes pi will not work).
				Name:              "container_r",
				ExtraSoftwareDeps: []string{"android_container_r"},
			}, {
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
		packageName     = "com.netease.mrzhna"
		waitingLabel    = "LifeAfter, waiting"
		installingLabel = "LifeAfter, installing"
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

	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)

	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)

	s.Log("Start app installation in Play Store")
	if err := playstore.BeginAppInstall(ctx, arcDevice, uiAutomator, packageName, &playstore.Options{TryLimit: -1}); err != nil {
		s.Fatal("Failed to start app installation in Play Store: ", err)
	}

	s.Log("Open the Launcher")
	if err := launcher.Open(tconn)(ctx); err != nil {
		s.Fatal("Failed to open launcher: ", err)
	}

	s.Log("Search for the Waiting status promise icon")
	if err := ui.WaitUntilExists(launcher.AppItemViewFinder(waitingLabel))(ctx); err != nil {
		s.Log("Waiting status promise icon not found. Search for Installing status promise icon instead")
		if err := ui.WaitUntilExists(launcher.AppItemViewFinder(installingLabel))(ctx); err != nil {
			s.Fatal("Failed to find item with correct label in Launcher: ", err)
		}
	}
}
