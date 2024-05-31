// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package projector is used for writing Projector tests.
package projector

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/projector"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SavedFileToDefaultFolder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Creates a screencast and verifies it is saved at the default screencast storage folder in DriveFS",
		Contacts:     []string{"cros-projector+tast@google.com", "xiqiruan@chromium.org"},
		// ChromeOS > Software > Family > Projector
		BugComponent: "b:1080013",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "ondevice_speech", "gaia"},
		HardwareDeps: hwdep.D(hwdep.Microphone()),
		Timeout:      12 * time.Minute,
		Fixture:      "projectorLogin",
	})
}

func SavedFileToDefaultFolder(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 4*time.Minute)
	defer cancel()

	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	driveFsClient, err := drivefs.NewDriveFs(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed waiting for DriveFS to start: ", err)
	}
	s.Log("Drivefs fully started")

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	app := s.FixtValue().(projector.HasApp).App()
	cleanup, err := projector.SetUpProjectorApp(ctx, tconn, app)
	if err != nil {
		s.Fatal("Failed to set up Projector app: ", err)
	}
	defer cleanup(cleanupCtx)

	// We need to clean up any screencasts after the test to
	// prevent taking up Drive quota over time.
	defer projector.DeleteScreencastItems(cleanupCtx, tconn)
	if err := projector.LaunchCreationFlow(ctx, tconn, false /*launchAnnotator*/); err != nil {
		s.Fatal("Failed to go through the new screencast creation flow: ", err)
	}

	// Verifies Screencast saved to right location.
	ui := uiauto.New(tconn).WithTimeout(2 * time.Minute)
	screencastItem := nodewith.ClassName("screencast-media").Role(role.GenericContainer).First()
	screencastTitle := nodewith.Role(role.StaticText).Ancestor(nodewith.ClassName("screencast-title").Ancestor(screencastItem))
	if err := ui.WaitUntilExists(screencastItem)(ctx); err != nil {
		s.Fatal("Failed to wait for the screencast item to show: ", err)
	}
	screencastInfo, err := ui.Info(ctx, screencastTitle)
	if err != nil {
		s.Fatal("Failed to get screencast title info: ", err)
	}

	containerFolderPath := driveFsClient.MyDrivePath(filepath.Join("Screencast recordings", screencastInfo.Name))
	mediaFile := filepath.Join(containerFolderPath, fmt.Sprintf("%s.webm", screencastInfo.Name))
	metadataFile := filepath.Join(containerFolderPath, fmt.Sprintf("%s.screencast", screencastInfo.Name))
	thumbnailFile := filepath.Join(containerFolderPath, "thumbnail.png")
	for _, screencastFile := range []string{mediaFile, metadataFile, thumbnailFile} {
		if _, err := os.Stat(screencastFile); err != nil {
			s.Fatal("Failed to locate screencast file in default folder: ", err)
		}
	}

}
