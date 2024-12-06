// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package util functions that help with common actions for Mantis tast tests.
package util

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/updateengine"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// DownloadAndOpenFileInGallery is used to download an image and then open the image in Gallery app.
func DownloadAndOpenFileInGallery(ctx context.Context, cr *chrome.Chrome, testFileDataPath, testFile string) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		return errors.Wrap(err, "failed to get user's Download path")
	}

	ui := uiauto.New(tconn)
	localFileLocation := filepath.Join(downloadsPath, testFile)
	if err := ui.WithInterval(time.Second).Retry(10, func(context.Context) error {
		return fsutil.CopyFile(testFileDataPath, localFileLocation)
	})(ctx); err != nil {
		return errors.Wrapf(err, "failed to copy the test image to %s", localFileLocation)
	}

	// SWA installation is not guaranteed during startup.
	// Using this wait to check installation finished before starting test.
	testing.ContextLog(ctx, "Wait for Gallery to be installed")
	if err := ash.WaitForChromeAppInstalled(ctx, tconn, apps.Gallery.ID, 2*time.Minute); err != nil {
		return errors.Wrap(err, "failed to wait for Gallery to be installed")
	}

	// Open the Files App.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "launching the Files App failed")
	}

	if err := uiauto.Combine("open Downloads folder and double click file to launch Gallery",
		files.OpenDownloads(),
		files.WithTimeout(30*time.Second).WaitForFile(testFile),
		files.OpenFile(testFile),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open file in Downloads")
	}

	testing.ContextLog(ctx, "Wait for Gallery shown in shelf")
	if err := ash.WaitForApp(ctx, tconn, apps.Gallery.ID, time.Minute); err != nil {
		return errors.Wrap(err, "failed to check Gallery in shelf")
	}

	// Use image section to verify Gallery App rendering.
	testing.ContextLog(ctx, "Wait for Gallery app rendering")
	imageElement := nodewith.Role(role.Image).Name(testFile).Ancestor(galleryapp.RootFinder)
	if err := ui.WithInterval(time.Second).WaitUntilExists(imageElement)(ctx); err != nil {
		return errors.Wrap(err, "failed to render Gallery")
	}

	return nil
}

// EnsureDLCInstalled ensures a DLC package is installed.
func EnsureDLCInstalled(ctx context.Context, dlcID string) error {
	// Ensure that the update engine service is ready to receive DLC install request from DLC service.
	if err := upstart.StartJobAndWaitForDbusService(ctx, updateengine.JobName, updateengine.ServiceName); err != nil {
		return errors.Wrapf(err, "failed to ensure %s running", updateengine.JobName)
	}

	// Check dlcservice is up and running.
	if err := upstart.EnsureJobRunning(ctx, dlc.JobName); err != nil {
		return errors.Wrapf(err, "failed to ensure %s running", dlc.JobName)
	}

	// Install DLC.
	if err := dlc.Install(ctx, dlcID, ""); err != nil {
		return errors.Wrap(err, "failed to install DLC")
	}

	return nil
}

// DrawOnImage draws a line on image in Gallery app.
func DrawOnImage(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	backlightDropTarget := nodewith.HasClass("backlight-drop-target").Ancestor(galleryapp.RootFinder)
	imageCanvas := nodewith.Role(role.Image).Ancestor(backlightDropTarget).First()
	canvasBounds, err := ui.ImmediateLocation(ctx, imageCanvas)
	if err != nil {
		return errors.Wrap(err, "failed to get the canvas location")
	}

	startLocation := coords.NewPoint(
		canvasBounds.CenterX(),
		canvasBounds.CenterY(),
	)
	if err := mouse.Move(tconn, startLocation, 200*time.Millisecond)(ctx); err != nil {
		return errors.Wrap(err, "failed to move mouse")
	}

	if err := mouse.Press(tconn, mouse.LeftButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to press mouse")
	}

	endLocation := coords.NewPoint(
		canvasBounds.CenterX()+50,
		canvasBounds.CenterY()+50,
	)
	if err := mouse.Move(tconn, endLocation, 200*time.Millisecond)(ctx); err != nil {
		return errors.Wrap(err, "failed to move mouse")
	}

	if err := mouse.Release(tconn, mouse.LeftButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to release mouse")
	}

	return nil
}

// WaitForSpinner waits for spinner until it is gone.
func WaitForSpinner(ctx context.Context, tconn *chrome.TestConn, ui *uiauto.Context) error {
	spinner := nodewith.HasClass("mdc-circular-progress__spinner-layer").Ancestor(galleryapp.RootFinder)
	if err := uiauto.Combine("Waiting for spinner",
		ui.WithTimeout(3*time.Second).WaitUntilExists(spinner),
		ui.WithTimeout(1*time.Minute).WaitUntilGone(spinner))(ctx); err != nil {
		return errors.Wrap(err, "error while waiting for spinner")
	}

	return nil
}
