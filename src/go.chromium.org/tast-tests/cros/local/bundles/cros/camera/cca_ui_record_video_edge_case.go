// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package camera

import (
	"context"
	"os"
	"strings"
	"time"

	"github.com/mafredri/cdp/protocol/target"

	"go.chromium.org/tast-tests/cros/local/camera/cca"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/disk"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CCAUIRecordVideoEdgeCase,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Opens CCA and verifies edge cases for video recording",
		Contacts:     []string{"chromeos-camera-eng@google.com", "kamchonlathorn@chromium.org"},
		BugComponent: "b:978428", // ChromeOS > Platform > Technologies > Camera > App & Framework
		Attr:         []string{"group:mainline", "informational", "group:camera-libcamera"},
		SoftwareDeps: []string{"camera_app", "chrome"},
		Timeout:      7 * time.Minute,
		Fixture:      "ccaTestBridgeReadyWithFakeHALCamera",
	})
}

// Constants taken from cryptohome for testing low storage recording.
const (
	mib                 uint64 = 1024 * 1024 // 1 mib
	lowTarget                  = 128 * mib
	criticallyLowTarget        = 32 * mib
	fillOffset                 = 8 * mib
)

type fillFile struct {
	name    string
	deleted bool
}

type lowStorageSubtest func(context.Context, *cca.App, *chrome.Chrome) error

func createFillFile(ctx context.Context, cr *chrome.Chrome, fillUntil uint64) (*fillFile, error) {
	userPath, err := cryptohome.UserPath(ctx, cr.NormalizedUser())
	if err != nil {
		return nil, errors.Wrap(err, "cannot find user home dir path")
	}
	// Allocate a bit more offset in case the disk-cleanup procedure takes place
	// to free more space after allocating the specified disk space. Also set
	// lowerMargin to handle the case we create a file while recording a video,
	// the recording video may contribute to disk's free space as well.
	fileName, err := disk.FillUntilWithRetry(ctx, userPath, fillUntil-fillOffset, 0, 16*mib)
	if err != nil {
		return nil, errors.Wrap(err, "cannot create a fill file")
	}
	newFile := &fillFile{name: fileName, deleted: false}
	return newFile, nil
}

func (f *fillFile) delete() error {
	if f.deleted {
		return nil
	}
	if err := os.Remove(f.name); err != nil {
		return errors.Wrap(err, "cannot delete the file")
	}
	f.deleted = true
	return nil
}

// CCAUIRecordVideoEdgeCase verifies recording video when storage is low works
// as expected.
func CCAUIRecordVideoEdgeCase(ctx context.Context, s *testing.State) {
	// TODO(b/244261957): Move testConfirmDialog from cca_ui_record_video to this test.
	// TODO(b/244261957): Add testLowStorageOnPause
	cr := s.FixtValue().(cca.FixtureData).Chrome
	runTestWithApp := s.FixtValue().(cca.FixtureData).RunTestWithApp
	subTestTimeout := 2 * time.Minute
	for _, tc := range []struct {
		name string
		run  lowStorageSubtest
	}{
		{"testLowStorageWarning", testLowStorageWarning},
		{"testLowStorageAutoStop", testLowStorageAutoStop},
		{"testLowStorageCannotStart", testLowStorageCannotStart},
	} {
		subTestCtx, cancel := context.WithTimeout(ctx, subTestTimeout)
		s.Run(subTestCtx, tc.name, func(ctx context.Context, s *testing.State) {
			// Pushing device to low storage may affect performance, we
			// should wait until CPU is stable before starting the new subtest.
			defer func() {
				// GoBigSleepLint: TODO(b/264217261): Find better approach to cooldown for scarlet/dru.
				if err := testing.Sleep(ctx, 5*time.Second); err != nil {
					s.Errorf("Failed to wait while sleep before cooldown CPU for %v subtest: %v", tc.name, err)
				}
				if err := cpu.WaitUntilIdle(ctx); err != nil {
					s.Errorf("Failed to wait until CPU idle for %v subtest: %v", tc.name, err)
				}
			}()

			if err := runTestWithApp(ctx, func(ctx context.Context, app *cca.App) error {
				return tc.run(ctx, app, cr)
			}, cca.TestWithAppParams{}); err != nil {
				s.Errorf("Failed to pass %v subtest: %v", tc.name, err)
			}
		})
		cancel()
	}
}

func checkVideoFile(ctx context.Context, app *cca.App, beforeSaveTime time.Time) error {
	testing.ContextLog(ctx, "Checking for saved video file")
	dir, err := app.SavedDir(ctx)
	if err != nil {
		return errors.Wrap(err, "cannot find saved directory")
	}
	video, err := app.WaitForFileSaved(ctx, dir, cca.VideoPattern, beforeSaveTime)
	if err != nil {
		return errors.Wrap(err, "failed to find saved video file")
	}
	if video.Size() == 0 {
		return errors.New("recorded video is empty")
	}
	return nil
}

// testLowStorageWarning tries to assert that warning visibility while recording
// video. The warning is expected to be shown if the storage is LOW, and will be
// hidden if the storage is back to NORMAL.
func testLowStorageWarning(ctx context.Context, app *cca.App, cr *chrome.Chrome) (retErr error) {
	testing.ContextLog(ctx, "Switch to video mode")
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		return errors.Wrap(err, "failed to switch to video mode")
	}

	// Create the first file to take up space.
	f1, err := createFillFile(ctx, cr, lowTarget)
	if err != nil {
		return err
	}
	defer func() {
		if err := f1.delete(); err != nil {
			retErr = errors.Wrap(retErr, err.Error())
		}
	}()

	testing.ContextLog(ctx, "Start recording video")
	if err := app.ClickShutter(ctx); err != nil {
		return errors.Wrap(err, "failed to click shutter to start recording")
	}
	if err := app.WaitForState(ctx, "recording", true); err != nil {
		return errors.Wrap(err, "recording is not started")
	}

	testing.ContextLog(ctx, "Waiting for warning to display")
	if err := app.WaitForVisibleState(ctx, cca.LowStorageWarning, true); err != nil {
		return errors.Wrap(err, "failed to wait for warning to display")
	}

	testing.ContextLog(ctx, "Waiting for warning to hide")
	if err := f1.delete(); err != nil {
		return errors.Wrap(err, "failed to delete the fill file")
	}
	if err := app.WaitForVisibleStateFor(ctx, cca.LowStorageWarning, false, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for warning to hide")
	}

	f2, err := createFillFile(ctx, cr, lowTarget)
	if err != nil {
		return err
	}
	defer func() {
		if err := f2.delete(); err != nil {
			retErr = errors.Wrap(retErr, err.Error())
		}
	}()

	testing.ContextLog(ctx, "Waiting for warning to re-display")
	if err := app.WaitForVisibleStateFor(ctx, cca.LowStorageWarning, true, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for warning to re-display")
	}

	stopRecordingTime := time.Now()
	if err := app.ClickShutter(ctx); err != nil {
		return errors.Wrap(err, "failed to stop recording")
	}
	if err := app.WaitForState(ctx, "recording", false); err != nil {
		return errors.Wrap(err, "recording is not stopped")
	}
	if err := checkVideoFile(ctx, app, stopRecordingTime); err != nil {
		return err
	}
	return nil
}

// testLowStorageAutoStop asserts that video would be automatically stopped
// after storage is changed to CRITICALLY_LOW. The dialog should appear, and
// clicking OK should close the dialog.
func testLowStorageAutoStop(ctx context.Context, app *cca.App, cr *chrome.Chrome) (retErr error) {
	testing.ContextLog(ctx, "Switch to video mode")
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		return errors.Wrap(err, "failed to switch to video mode")
	}

	testing.ContextLog(ctx, "Start recording video")
	if err := app.ClickShutter(ctx); err != nil {
		return errors.Wrap(err, "failed to click shutter to start recording")
	}
	if err := app.WaitForState(ctx, "recording", true); err != nil {
		return errors.Wrap(err, "recording is not started")
	}

	// GoBigSleepLint: Record the video for 1 second.
	if err := testing.Sleep(ctx, time.Second); err != nil {
		return err
	}
	// Create a big file to make the storage CRITICALLY_LOW.
	f, err := createFillFile(ctx, cr, criticallyLowTarget)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.delete(); err != nil {
			retErr = errors.Wrap(retErr, err.Error())
		}
	}()

	testing.ContextLog(ctx, "Waiting for recording to stop and the dialog to show up")
	beforeVideoEndTime := time.Now()
	if err := app.WaitForVisibleStateFor(ctx, cca.LowStorageDialog, true, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for auto-stop dialog to display")
	}
	if err := app.WaitForState(ctx, "recording", false); err != nil {
		return errors.Wrap(err, "recording is not stopped")
	}
	if err := checkVideoFile(ctx, app, beforeVideoEndTime); err != nil {
		return err
	}

	if err := app.Click(ctx, cca.LowStorageDialogOKButton); err != nil {
		return errors.Wrap(err, "failed to click OK button on low storage dialog")
	}
	testing.ContextLog(ctx, "Waiting for the dialog to close")
	if err := app.WaitForVisibleState(ctx, cca.LowStorageDialog, false); err != nil {
		return errors.Wrap(err, "failed to wait for auto-stop dialog to close")
	}

	return nil
}

// testLowStorageCannotStart asserts that video recording could not start if the
// storage is CRITICALLY_LOW. This also tests opening Storage management page
// from the dialog.
func testLowStorageCannotStart(ctx context.Context, app *cca.App, cr *chrome.Chrome) (retErr error) {
	testing.ContextLog(ctx, "Switch to video mode")
	if err := app.SwitchMode(ctx, cca.Video); err != nil {
		return errors.Wrap(err, "failed to switch to video mode")
	}

	f, err := createFillFile(ctx, cr, criticallyLowTarget)
	if err != nil {
		return err
	}
	defer func() {
		if err := f.delete(); err != nil {
			retErr = errors.Wrap(retErr, err.Error())
		}
	}()

	testing.ContextLog(ctx, "Waiting for the dialog to show up")
	if err := app.ClickShutter(ctx); err != nil {
		return err
	}
	if err := app.WaitForVisibleState(ctx, cca.LowStorageDialog, true); err != nil {
		return errors.Wrap(err, "failed to wait for cannot-start dialog to display")
	}
	if recordingState, err := app.State(ctx, "recording"); err != nil {
		return errors.Wrap(err, "failed to fetch recording state")
	} else if recordingState == true {
		return errors.New("failed to disallow recording, recording still started")
	}

	testing.ContextLog(ctx, "Opening Storage management page")
	if err := app.Click(ctx, cca.LowStorageDialogManageButton); err != nil {
		return errors.Wrap(err, "failed to click storage management button on low storage dialog")
	}
	checkManagePagePrefix := func(t *target.Info) bool {
		url := "chrome://os-settings/storage"
		return strings.HasPrefix(t.URL, url)
	}
	var managePageTargetID target.ID
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		managePageTargets, err := cr.FindTargets(ctx, checkManagePagePrefix)
		if err != nil {
			return testing.PollBreak(err)
		}
		if len(managePageTargets) == 0 {
			return errors.New("Storage management page should be launched")
		} else if len(managePageTargets) > 1 {
			return errors.New("More than one Storage management page is launched")
		}
		managePageTargetID = managePageTargets[0].TargetID
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second}); err != nil {
		return errors.Wrap(err, "failed to launch Storage management page within given time")
	}
	if err := cr.CloseTarget(ctx, managePageTargetID); err != nil {
		return errors.Wrap(err, "failed to close Storage management")
	}

	testing.ContextLog(ctx, "Waiting for the dialog to close")
	if err := app.WaitForVisibleState(ctx, cca.LowStorageDialog, false); err != nil {
		return errors.Wrap(err, "failed to wait for cannot-start dialog to close")
	}

	return nil
}
