// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"net/http"
	"net/http/httptest"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/pre"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchGalleryFromNotifications,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Verify Gallery launches correctly when opening image from notifications",
		Contacts: []string{
			"backlight-swe@google.com",
		},
		BugComponent: "b:562866",
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Data:         []string{"gear_wheels_4000x3000_20200624.jpg", "download_link.html"},
		Params: []testing.Param{
			{
				Name:              "stable",
				Fixture:           fixture.LoggedIn,
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				ExtraAttr:         []string{"group:mainline"},
			}, {
				Name:    "unstable",
				Fixture: fixture.LoggedIn,
				// b:238260020 - disable aged (>1y) unpromoted informational tests
				// ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraHardwareDeps: hwdep.D(pre.AppsUnstableModels),
			}, {
				Name:              "lacros",
				Fixture:           fixture.LacrosLoggedIn,
				ExtraSoftwareDeps: []string{"lacros", "lacros_stable"},
				ExtraAttr:         []string{"group:mainline"},
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
			},
		},
	})
}

// LaunchGalleryFromNotifications verifies Gallery opens when Chrome notifications are clicked.
func LaunchGalleryFromNotifications(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	const (
		testImageFileName    = "gear_wheels_4000x3000_20200624.jpg"
		uiTimeout            = 20 * time.Second
		downloadCompleteText = "Download complete"
	)
	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get user's Download path: ", err)
	}
	testImageFileLocation := filepath.Join(downloadsPath, testImageFileName)
	defer os.Remove(testImageFileLocation)

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Create cleanup context to ensure UI tree dumps correctly.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// SWA installation is not guaranteed during startup.
	// Using this wait to check installation finished before starting test.
	s.Log("Wait for Gallery to be installed")
	err = ash.WaitForChromeAppInstalled(ctx, tconn, apps.Gallery.ID, 2*time.Minute)
	if err != nil {
		s.Fatal("Failed to wait for installed app: ", err)
	}

	conn, err := cr.NewConn(ctx, filepath.Join(server.URL, "download_link.html"))
	if err != nil {
		s.Fatal("Failed navigating to image on local server: ", err)
	}
	defer conn.Close()

	_, err = ash.WaitForNotification(ctx, tconn, uiTimeout, ash.WaitTitle(downloadCompleteText))
	if err != nil {
		s.Fatalf("Failed waiting %v for download notification", uiTimeout)
	}

	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)
	notificationFinder := nodewith.Role(role.StaticText).Name(downloadCompleteText)

	if err := ui.LeftClick(notificationFinder)(ctx); err != nil {
		s.Fatal("Failed finding notification and clicking it: ", err)
	}

	s.Log("Wait for Gallery shown in shelf")
	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, time.Minute); err != nil {
		s.Fatal("Failed to check Gallery in shelf: ", err)
	}

	s.Log("Wait for Gallery app rendering")
	imageElementFinder := nodewith.Role(role.Image).Name(testImageFileName).Ancestor(galleryapp.RootFinder)
	// Use image section to verify Gallery App rendering.
	if err := ui.WaitUntilExists(imageElementFinder)(ctx); err != nil {
		s.Fatal("Failed to render Gallery: ", err)
	}
}
