// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package gallerycuj provides gallery instance for cuj tests on ChromeOS.
package gallerycuj

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/galleryapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj/inputsimulations"
	"go.chromium.org/tast-tests/cros/local/chrome/display"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// removeAllFilesInDirectory removes all files in a directory but leaves
// the directory itself intact.
func removeAllFilesInDirectory(directory string) error {
	files, err := ioutil.ReadDir(directory)
	if err != nil {
		return errors.Wrapf(err, "failed to read files in %s", directory)
	}
	for _, f := range files {
		path := filepath.Join(directory, f.Name())
		if err := os.RemoveAll(path); err != nil {
			return errors.Wrapf(err, "failed to RemoveAll(%q)", path)
		}
	}
	return nil
}

// unzipTestFiles extracts test data files to the desired directory.
func unzipTestFiles(ctx context.Context, zipPath, filePath string) error {
	if err := testexec.CommandContext(ctx, "unzip", zipPath, "-d", filePath).Run(testexec.DumpLogOnError); err != nil {
		return errors.Wrapf(err, "failed to unzip test data from %v", zipPath)
	}
	return nil
}

// getFileNames returns the file names of the specified file type.
func getFileNames(ctx context.Context, filePath, fileType string) (fileNames []string, err error) {
	files, err := ioutil.ReadDir(filePath)
	if err != nil {
		return fileNames, errors.Wrap(err, "failed to read directory")
	}
	for _, f := range files {
		if strings.Contains(f.Name(), "."+fileType) {
			fileNames = append(fileNames, f.Name())
		}
	}
	return fileNames, nil
}

// dragFilesToGallery drags the specified files from FilesApp to Gallery.
func dragFilesToGallery(kb *input.KeyboardEventWriter, tconn *chrome.TestConn, files *filesapp.FilesApp, listFileNames []string) action.Action {
	return func(ctx context.Context) error {
		ui := uiauto.New(tconn)
		galleryLocation, err := ui.Location(ctx, galleryapp.RootFinder)
		if err != nil {
			return errors.Wrap(err, "failed to get Gallery window node position")
		}
		galleryText := nodewith.Name("View and edit your files").Role(role.StaticText)
		if err = ui.WithTimeout(time.Minute).RetryUntil(
			files.DragAndDropFilesWithTimeout(listFileNames, galleryLocation.CenterPoint(), kb, 5*time.Second),
			// Check whether the file is successfully loaded by Gallery.
			ui.WithTimeout(5*time.Second).WaitUntilGone(galleryText),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to drag files to Gallery")
		}
		return nil
	}
}

// dragAllFilesToGallery drags the all files from FilesApp to Gallery.
func dragAllFilesToGallery(kb *input.KeyboardEventWriter, tconn *chrome.TestConn) action.Action {
	ui := uiauto.New(tconn)
	listBoxOption := nodewith.Role(role.ListBoxOption).First()
	selectedText := nodewith.NameContaining("files selected").Role(role.StaticText)
	galleryText := nodewith.Name("View and edit your files").Role(role.StaticText)
	return uiauto.Retry(3, uiauto.NamedCombine("drag all files to Gallery",
		ui.LeftClick(listBoxOption),
		kb.AccelAction("Ctrl+A"),
		ui.WaitUntilExists(selectedText),
		ui.MousePress(mouse.LeftButton, listBoxOption),
		ui.MouseMoveTo(galleryText, 500*time.Millisecond),
		ui.MouseRelease(mouse.LeftButton),
		// Check whether the file is successfully loaded by Gallery.
		ui.WithTimeout(5*time.Second).WaitUntilGone(galleryText),
	))
}

// setWindowState sets the window with specified title to the specified state.
func setWindowState(tconn *chrome.TestConn, title string, state ash.WindowStateType) action.Action {
	return func(ctx context.Context) error {
		var window *ash.Window
		if err := ash.WaitForCondition(ctx, tconn, func(w *ash.Window) bool {
			window = w
			return strings.Contains(w.Title, title)
		}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
			return errors.Wrapf(err, "failed to find window %q", title)
		}
		if err := ash.SetWindowStateAndWait(ctx, tconn, window.ID, state); err != nil {
			return errors.Wrapf(err, "failed to set app %q to %q", title, state)
		}
		return nil
	}
}

// runAndWaitHistogram runs the function and waits for the histogram to be generated.
func runAndWaitHistogram(tconn *chrome.TestConn, f func(ctx context.Context) error, name string) action.Action {
	return func(ctx context.Context) error {
		histogram, err := metrics.RunAndWaitAny(ctx, tconn, 10*time.Second, f, name)
		if err != nil {
			testing.ContextLogf(ctx, "Failed to collect %q: %v", name, err)
		} else {
			testing.ContextLog(ctx, "Collect histogram: ", histogram)
		}
		return nil
	}
}

// scrollByMouse returns a function that repeatedly scrolls mouse wheel
// down and up for |duration|.
func scrollByMouse(mw *input.MouseEventWriter, duration time.Duration) action.Action {
	scroll := func(ctx context.Context) error {
		for _, scrollDown := range []bool{true, false} {
			if err := inputsimulations.RepeatMouseScroll(ctx, mw, scrollDown, 50*time.Millisecond, 40); err != nil {
				return errors.Wrap(err, "failed to repeatedly mouse scroll")
			}
		}
		return nil
	}
	return runActionFor(duration, scroll)
}

// scrollByKeyboard returns a function that repeatedly scrolls down and
// up through the keyboard for |duration|.
func scrollByKeyboard(kb *input.KeyboardEventWriter, duration time.Duration) action.Action {
	scroll := func(ctx context.Context) error {
		for _, key := range []string{"Down", "Up"} {
			if err := inputsimulations.RepeatKeyPress(ctx, kb, key, 50*time.Millisecond, 40); err != nil {
				return errors.Wrapf(err, "failed to repeatedly key press %q", key)
			}
		}
		return nil
	}
	return runActionFor(duration, scroll)
}

// runActionFor repeats action |a| until |minDuration| has passed.
func runActionFor(minDuration time.Duration, a action.Action) action.Action {
	return func(ctx context.Context) error {
		for endTime := time.Now().Add(minDuration); time.Now().Before(endTime); {
			if err := a(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

// runDragMouseCycle returns a function that presses the left mouse
// button at the center of the screen, and moves the mouse to the
// leftmost side of the screen and then to the rightmost side, then
// releases the mouse back at the center of the screen.
func runDragMouseCycle(tconn *chrome.TestConn, info *display.Info) action.Action {
	return func(ctx context.Context) error {
		if err := mouse.Move(tconn, info.Bounds.CenterPoint(), 2*time.Second)(ctx); err != nil {
			return err
		}
		return inputsimulations.RunDragMouseCycle(ctx, tconn, info)
	}
}

// waitForAndCloseDownloadCompleteNotification waits for "Download Complete"
// notification appear and closes it.
func waitForAndCloseDownloadCompleteNotification(tconn *chrome.TestConn) uiauto.Action {
	return func(ctx context.Context) error {
		_, err := ash.WaitForNotification(
			ctx, tconn, 15*time.Second, ash.WaitTitle("Download complete"))
		if err != nil {
			return err
		}
		return ash.CloseNotifications(ctx, tconn)
	}
}
