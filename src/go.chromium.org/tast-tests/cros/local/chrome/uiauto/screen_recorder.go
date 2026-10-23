// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package uiauto

import (
	"context"
	"encoding/base64"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"github.com/google/uuid"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
)

// ScreenRecorder is a utility to record the screen during a test scenario.
type ScreenRecorder struct {
	isRecording   bool
	videoRecorder *chrome.JSObject
	result        string
	downloadsPath string
	tconn         *chrome.TestConn
	cr            *chrome.Chrome
}

type testingState interface {
	OutDir() string
	HasError() bool
}

func requestScreenShare(ctx context.Context, tconn *chrome.TestConn) (*ScreenRecorder, error) {
	expr := `({
			ensureOffscreen: async function(recreate) {
				const hasDoc = await chrome.offscreen.hasDocument();
				if (hasDoc && recreate) {
					await chrome.offscreen.closeDocument();
				}
				if (!hasDoc || recreate) {
					await chrome.offscreen.createDocument({
						url: 'offscreen.html',
						reasons: ['DISPLAY_MEDIA', 'USER_MEDIA'],
						justification: 'Screen share recording for Tast tests',
					});
				}
			},
			closeOffscreen: async function() {
				const hasDoc = await chrome.offscreen.hasDocument();
				if (hasDoc) {
					await chrome.offscreen.closeDocument();
				}
			},
			send: async function(action, recreate) {
				await this.ensureOffscreen(recreate);
				const resp = await chrome.runtime.sendMessage({
					target: 'offscreen-screen-recorder',
					action: action,
				});
				if (!resp || !resp.ok) {
					throw new Error((resp && resp.error) || 'Failed offscreen action: ' + action);
				}
				return resp.result;
			},
			request: function() {
				return this.send('request', true);
			},
			start: function() {
				return this.send('start', false);
			},
			stop: function() {
				return this.send('stop', false);
			},
			frameStatus: function() {
				return this.send('frameStatus', false);
			}
		})
	`
	videoRecorder := &chrome.JSObject{}
	if err := tconn.Eval(ctx, expr, videoRecorder); err != nil {
		return nil, errors.Wrap(err, "failed to initialize video recorder")
	}
	sr := &ScreenRecorder{isRecording: false, videoRecorder: videoRecorder}

	// Request to share the screen.
	if err := sr.videoRecorder.Call(ctx, nil, `function() {return this.request();}`); err != nil {
		return nil, errors.Wrap(err, "failed to request display media")
	}

	return sr, nil
}

// NewScreenRecorder creates a ScreenRecorder.
// It only needs to create one ScreenRecorder during one test.
func NewScreenRecorder(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*ScreenRecorder, error) {
	return &ScreenRecorder{
		cr:    cr,
		tconn: tconn,
	}, nil
}

// NewScreenShareRecorder creates a ScreenRecorder that shares and records the entire desktop screen via the test extension.
func NewScreenShareRecorder(ctx context.Context, tconn *chrome.TestConn) (*ScreenRecorder, error) {
	sr, err := requestScreenShare(ctx, tconn)
	if err != nil {
		return nil, err
	}
	if err := ChooseScreenRecorder(ctx, tconn); err != nil {
		return nil, err
	}
	return sr, nil
}

// ChooseScreenRecorder makes the selection to record the entire desktop screen.
func ChooseScreenRecorder(ctx context.Context, tconn *chrome.TestConn) error {
	ui := New(tconn)
	// There may be multiple "Choose what to share" nodes, so add First() here.
	shareScreenDialog := nodewith.NameStartingWith("Choose what to share").HasClass("DesktopMediaPickerDialogView").First()
	entireScreenTab := nodewith.NameRegex(regexp.MustCompile("(?i)Entire Screen")).Role(role.Tab).Ancestor(shareScreenDialog)
	entireScreenPane := nodewith.NameRegex(regexp.MustCompile("(?i)Entire Screen")).Role(role.TabPanel).Ancestor(shareScreenDialog)
	firstDisplay := nodewith.Role(role.Button).HasClass("DesktopMediaSourceView").Focusable().Ancestor(entireScreenPane).First()
	// The share button becomes focusable after the entire desktop button is clicked.
	shareButton := nodewith.Name("Share").Role(role.Button).Ancestor(shareScreenDialog).Focusable()

	return Combine("start screen recorder through ui",
		ui.WithInterval(500*time.Millisecond).DoDefaultUntil(entireScreenTab, ui.Exists(firstDisplay)),
		ui.WithInterval(500*time.Millisecond).DoDefaultUntil(firstDisplay, ui.Exists(shareButton)),
		ui.DoDefaultUntil(shareButton, ui.WithTimeout(time.Second).WaitUntilGone(shareButton)),
	)(ctx)
}

// NewWindowRecorder creates a ScreenRecorder, using a window as the media stream.
// The specific window can be chosen with the windowIndex parameter.
// For other information see the comment on the NewScreenRecorder function.
func NewWindowRecorder(ctx context.Context, tconn *chrome.TestConn, windowIndex int) (*ScreenRecorder, error) {
	sr, err := requestScreenShare(ctx, tconn)

	if err != nil {
		return nil, err
	}

	ui := New(tconn)
	shareScreenDialog := nodewith.NameStartingWith("Choose what to share").HasClass("DesktopMediaPickerDialogView").First()
	windowTab := nodewith.Name("Window").Role(role.Tab).Ancestor(shareScreenDialog)
	windowPane := nodewith.Name("Window").Role(role.TabPanel).Ancestor(shareScreenDialog)
	windowButton := nodewith.Role(role.Button).HasClass("DesktopMediaSourceView").Ancestor(windowPane).Nth(windowIndex)
	shareButton := nodewith.Name("Share").Role(role.Button).Ancestor(shareScreenDialog).Focusable()

	if err := Combine("start screen recorder through ui",
		ui.WithInterval(500*time.Millisecond).DoDefaultUntil(windowTab, ui.Exists(windowButton)),
		ui.WithInterval(500*time.Millisecond).DoDefaultUntil(windowButton, ui.Exists(shareButton)),
		ui.DoDefaultUntil(shareButton, ui.WithTimeout(time.Second).WaitUntilGone(shareButton)),
	)(ctx); err != nil {
		return nil, err
	}

	return sr, nil
}

// NewTabRecorder creates a ScreenRecorder, using a tab as the media stream.
// The specific tab can be chosen with the tabIndex parameter.
// For other information see the comment on the NewScreenRecorder function.
func NewTabRecorder(ctx context.Context, tconn *chrome.TestConn, tabIndex int) (*ScreenRecorder, error) {
	sr, err := requestScreenShare(ctx, tconn)

	if err != nil {
		return nil, err
	}

	ui := New(tconn)
	shareScreenDialog := nodewith.NameStartingWith("Choose what to share").HasClass("DesktopMediaPickerDialogView").First()
	shareChromeTabOption := nodewith.NameRegex(regexp.MustCompile("(Chromium|Chrome) Tab")).Role(role.Tab).Ancestor(shareScreenDialog)
	tabButton := nodewith.Role(role.Row).Ancestor(shareScreenDialog).Nth(tabIndex)
	shareButton := nodewith.Name("Share").Role(role.Button).Ancestor(shareScreenDialog).Focusable()

	if err := Combine("start screen recorder through ui",
		ui.WithInterval(500*time.Millisecond).DoDefaultUntil(shareChromeTabOption, ui.Exists(tabButton)),
		ui.WithInterval(500*time.Millisecond).DoDefaultUntil(tabButton, ui.Exists(shareButton)),
		ui.DoDefaultUntil(shareButton, ui.WithTimeout(time.Second).WaitUntilGone(shareButton)),
	)(ctx); err != nil {
		return nil, err
	}

	return sr, nil
}

// Start creates a new media recorder and starts to record the screen. As long as ScreenRecorder
// is not recording, it can start to record again.
func (r *ScreenRecorder) Start(ctx context.Context, tconn *chrome.TestConn) error {
	if r.videoRecorder != nil {
		if r.isRecording {
			return errors.Errorf("recorder is already recording (isRecording is %t)", r.isRecording)
		}
		if err := r.videoRecorder.Call(ctx, nil, `function() {return this.start();}`); err != nil {
			return errors.Wrap(err, "failed to start screen recorder")
		}
		r.isRecording = true
		return nil
	}
	downloadsPath, err := StartFromKB(ctx, tconn, r.cr)
	if err != nil {
		return errors.Wrap(err, "failed to start screen recording")
	}
	r.downloadsPath = downloadsPath
	return nil
}

// Stop ends the screen recording and stores the encoded base64 string.
func (r *ScreenRecorder) Stop(ctx context.Context) error {
	if r.videoRecorder != nil {
		if !r.isRecording {
			return errors.Errorf("recorder has not started recording (isRecording is %t)", r.isRecording)
		}
		if err := r.videoRecorder.Call(ctx, &r.result, `function() {return this.stop();}`); err != nil {
			return errors.Wrap(err, "failed to stop screen recorder")
		}
		r.isRecording = false
		return nil
	}
	if err := StopRecordFromUI(ctx, r.tconn); err != nil {
		return errors.Wrap(err, "failed to stop screen recording")
	}

	return nil
}

// SaveInBytes saves the latest encoded string into a decoded bytes file.
func (r *ScreenRecorder) SaveInBytes(ctx context.Context, filepath string) error {
	if r.videoRecorder != nil {
		parts := strings.Split(r.result, ",")
		if len(parts) < 2 {
			return errors.New("invalid recording result")
		}
		bytes, err := base64.StdEncoding.DecodeString(parts[1])
		if err != nil {
			return errors.Wrap(err, "failed to decode base64 string into bytes")
		}
		if err := os.WriteFile(filepath, bytes, 0644); err != nil {
			return errors.Wrapf(err, "failed to dump bytes to %s", filepath)
		}
		return nil
	}
	if err := SaveOneRecordFromKB(ctx, r.tconn, filepath, r.downloadsPath); err != nil {
		return errors.Wrapf(err, "failed to save %s", filepath)
	}
	os.RemoveAll(r.downloadsPath)
	r.downloadsPath = ""
	return nil // Successfully created an empty file.
}

// SaveInString saves the latest encoded string into a string file.
func (r *ScreenRecorder) SaveInString(ctx context.Context, filepath string) error {
	result := strings.Split(r.result, ",")[1]
	if err := os.WriteFile(filepath, []byte(result), 0644); err != nil {
		return errors.Wrapf(err, "failed to dump string to %s", filepath)
	}
	return nil
}

// FrameStatus returns the status of the frame being recorded.
func (r *ScreenRecorder) FrameStatus(ctx context.Context) (string, error) {
	var result string
	if err := r.videoRecorder.Call(ctx, &result, `function() {return this.frameStatus();}`); err != nil {
		return "", errors.Wrap(err, "failed to get frame status")
	}
	return result, nil
}

// Release frees the reference to Javascript for this video recorder.
func (r *ScreenRecorder) Release(ctx context.Context) {
	if r.videoRecorder != nil {
		_ = r.videoRecorder.Call(ctx, nil, `function() {return this.closeOffscreen();}`)
		r.videoRecorder.Release(ctx)
	}
}

// StopAndSaveOnError ends the screen recording and save it on error.
func (r *ScreenRecorder) StopAndSaveOnError(ctx context.Context, filepath string, hasError func() bool) {
	if r.videoRecorder != nil {
		if err := r.Stop(ctx); err != nil {
			testing.ContextLog(ctx, "Failed to stop recording: ", err)
			return
		}
		if hasError() {
			testing.ContextLogf(ctx, "Saving screen record to %s", filepath)
			if err := r.SaveInBytes(ctx, filepath); err != nil {
				testing.ContextLog(ctx, "Failed to save screen record in bytes: ", err)
			}
		}
		r.Release(ctx)
		return
	}
	defer func() {
		os.RemoveAll(r.downloadsPath)
		r.downloadsPath = ""
	}()
	if err := StopRecordFromUI(ctx, r.tconn); err != nil {
		testing.ContextLog(ctx, "Failed to stop recording: ", err)
		return
	}
	if hasError() {
		if err := r.SaveInBytes(ctx, filepath); err != nil {
			testing.ContextLog(ctx, "Failed to save screen recorder: ", err)
		}
	}
}

// ScreenRecorderStopSaveRelease stops, saves and releases the screen recorder.
func ScreenRecorderStopSaveRelease(ctx context.Context, r *ScreenRecorder, fileName string) {
	if r != nil {
		if err := r.Stop(ctx); err != nil {
			testing.ContextLogf(ctx, "Failed to stop recording: %s", err)
		} else {
			testing.ContextLogf(ctx, "Saving screen record to %s", fileName)
			if err := r.SaveInBytes(ctx, fileName); err != nil {
				testing.ContextLogf(ctx, "Failed to save screen record in bytes: %s", err)
			}
		}
		r.Release(ctx)
	}
}

// CreateAndStartScreenRecorder creates a ScreenRecorder and starts to record
// the screen. Handles errors by logging them via the context and returning nil.
// To handle errors manually, call CreateAndStartScreenRecorderWithError.
//
// Example usage:
//
//	recorder := uiauto.CreateAndStartScreenRecorder(ctx, tconn)
//	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)
func CreateAndStartScreenRecorder(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) *ScreenRecorder {
	recorder, err := CreateAndStartScreenRecorderWithError(ctx, tconn, cr)
	if err != nil {
		testing.ContextLog(ctx, "Failed to create and start the screen recorder: ", err)
	}
	return recorder
}

// CreateAndStartScreenRecorderWithError creates a ScreenRecorder and starts to
// record the screen. To handle errors automatically, call
// CreateAndStartScreenRecorder.
//
// Example usage:
//
//	recorder, err := uiauto.CreateAndStartScreenRecorderWithError(ctx, tconn)
//	if err != nil {
//		s.Log(ctx, "Failed to create and start the screen recorder: ", err)
//	}
//	defer uiauto.StopAndSaveOnError(cleanupCtx, recorder, filepath.Join(s.OutDir(), "screen_recording.webm"), s.HasError)
func CreateAndStartScreenRecorderWithError(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (*ScreenRecorder, error) {
	recorder, err := NewScreenRecorder(ctx, tconn, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to create screen recorder")
	}
	if recorder != nil {
		if err := recorder.Start(ctx, tconn); err != nil {
			return nil, errors.Wrap(err, "failed to start screen recorder")
		}
	}
	return recorder, nil
}

// StopAndSaveOnError ends the screen recording, and saves it on error.
// Skips if the recorder is nil.
func StopAndSaveOnError(ctx context.Context, sr *ScreenRecorder, filepath string, hasError func() bool) {
	if sr != nil {
		sr.StopAndSaveOnError(ctx, filepath, hasError)
	}
}

// generateUniqueFolderName create unique name for temp screen recording dir
func generateUniqueFolderName() (string, error) {
	id, err := uuid.NewRandom()
	if err != nil {
		return "", err
	}
	return id.String(), nil
}

// UniqueUserFolderName returns a unique path from the top level user folder.
func UniqueUserFolderName(ctx context.Context, cr *chrome.Chrome) (string, error) {
	originalDownloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	testing.ContextLog(ctx, "default downloadsPath is:", originalDownloadsPath)
	if err != nil {
		return "", errors.Wrap(err, "failed to get user's Downloads path")
	}

	parentDir := filepath.Dir(originalDownloadsPath)

	uniqueFolderName, err := generateUniqueFolderName()
	if err != nil {
		return "", errors.Wrap(err, "failed to generate unique folder name")
	}
	uniqueUserPath := filepath.Join(parentDir, uniqueFolderName)
	return uniqueUserPath, nil
}

// StartFromKB creates an EventWriter of keyboard and downloadsPath by normalized user
// to used by screen recording
func StartFromKB(ctx context.Context, tconn *chrome.TestConn, cr *chrome.Chrome) (string, error) {
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return "", errors.Wrap(err, "failed to setup keyboard for screen recording")
	}
	downloadsPath, err := UniqueUserFolderName(ctx, cr)
	if err != nil {
		return "", errors.Wrap(err, "failed to generate unique folder name")
	}

	if err := StartRecordFromUI(ctx, tconn, kb, downloadsPath); err != nil {
		testing.ContextLog(ctx, "Failed to start screen recording, attempting to clean up UI")
		if cleanupErr := cleanupScreenCaptureUI(ctx, tconn, kb); cleanupErr != nil {
			testing.ContextLog(ctx, "WARNING: Failed to clean up screen capture ui: ", cleanupErr)
		}
		return "", errors.Wrap(err, "failed to start screen recording on CrOS")
	}

	testing.ContextLog(ctx, "Started screen recording")

	return downloadsPath, nil
}

// StartRecordFromKB starts screen record from keyboard.
// It clicks Ctrl+Shift+F5 then select to record the whole desktop.
// The caller should also call StopRecordFromKB to stop the screen recorder,
// and save the record file.
// Here is an example to call this method:
//
//	if err := uiauto.StartRecordFromKB(ctx, tconn, keyboard); err != nil {
//	    s.Log("Failed to start recording: ", err)
//	}
//
//	defer uiauto.StopRecordFromKBAndSaveOnError(ctx, tconn, s.HasError, s.OutDir())
func StartRecordFromKB(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, downloadsPath string) error {
	screenRecordBtn := nodewith.NameRegex(regexp.MustCompile("Screen record.*")).Role(role.ToggleButton)
	fullScreenBtn := nodewith.NameRegex(regexp.MustCompile("Record full screen.*")).Role(role.ToggleButton)
	desktop := nodewith.Role(role.Window).First()
	ui := New(tconn)

	files, err := os.ReadDir(downloadsPath)
	if err != nil {
		return errors.Wrap(err, "failed to read files from Downloads")
	}
	expectNumber := len(files) + 1
	const timeout = 10 * time.Second
	checkRecordFile := func(ctx context.Context) error {
		return testing.Poll(ctx, func(ctx context.Context) error {
			files, err = os.ReadDir(downloadsPath)
			if err != nil {
				return errors.Wrap(err, "failed to read files from Downloads")
			}
			if len(files) == expectNumber {
				return nil
			}
			return errors.Wrapf(err, "failed to check number of files, got %d, want %d", len(files), expectNumber)
		}, &testing.PollOptions{Timeout: timeout})
	}
	return Combine("start screen record",
		kb.AccelAction("Ctrl+Shift+F5"),
		ui.LeftClick(screenRecordBtn),
		ui.LeftClick(fullScreenBtn),
		ui.LeftClick(desktop), // It needs to click any button to start, so clicking on the middle of the desktop.
		checkRecordFile,       // Check a new record file is created in Downloads.
	)(ctx)
}

// cleanupScreenCaptureUI attempts to close the screen capture UI by repeatedly pressing Escape.
// This is used to clean up the UI if it gets stuck during a test.
func cleanupScreenCaptureUI(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter) error {
	ui := New(tconn)
	screenRecordBtn := nodewith.NameRegex(regexp.MustCompile("Screen record.*")).Role(role.ToggleButton)
	myfilesBtn := nodewith.NameRegex(regexp.MustCompile("My files.*")).Role(role.Button)

	testing.ContextLog(ctx, "Checking for leftover screen capture UI to clean up")

	return testing.Poll(ctx, func(ctx context.Context) error {
		// Check for the existence of both windows elements.
		if ui.Exists(screenRecordBtn)(ctx) != nil && ui.Exists(myfilesBtn)(ctx) != nil {
			testing.ContextLog(ctx, "Screen record windows is gone. Cleanup successful")
			return nil
		}

		testing.ContextLog(ctx, "Leftover screen capture UI found, pressing ESC")
		if err := kb.Accel(ctx, "esc"); err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to press escape key during cleanup"))
		}

		return errors.New("screen capture UI text is still visible")
	}, &testing.PollOptions{
		Timeout:  10 * time.Second,
		Interval: 500 * time.Millisecond,
	})
}

// StartRecordFromUI starts screen record from UI and keyboard.
// It clicks screen capture then select to record the whole desktop.
// Using temporary dir during the recording process.
func StartRecordFromUI(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, downloadsPath string) error {
	screenRecordBtn := nodewith.NameRegex(regexp.MustCompile("Screen record.*")).Role(role.ToggleButton)
	fullScreenBtn := nodewith.NameRegex(regexp.MustCompile("Record full screen.*")).Role(role.ToggleButton)
	settingsBtn := nodewith.NameRegex(regexp.MustCompile("Settings.*")).Role(role.ToggleButton)
	selectBtn := nodewith.NameRegex(regexp.MustCompile("Select folder.*")).Role(role.Button)
	myfilesBtn := nodewith.NameRegex(regexp.MustCompile("My files.*")).Role(role.Button)
	newFolderBtn := nodewith.Name("New folder").Role(role.Button)
	openBtn := nodewith.Name("Open").Role(role.Button)
	desktop := nodewith.Role(role.Window).First()
	newFolderTextBox := nodewith.NameRegex(regexp.MustCompile("New folder.*")).Role(role.InlineTextBox)
	statusTrayBtm := nodewith.NameRegex(regexp.MustCompile("Status tray.*")).Role(role.Button).First()
	screenCaptureBtm := nodewith.Name("Screen capture").Role(role.Button)

	ui := New(tconn)

	var expectNumber int

	checkRecordFile := func(ctx context.Context) error {
		const timeout = 10 * time.Second
		return testing.Poll(ctx, func(ctx context.Context) error {
			files, err := os.ReadDir(downloadsPath)
			if err != nil {
				return errors.Wrap(err, "failed to read files from Downloads")
			}
			if len(files) == expectNumber {
				return nil
			}
			return errors.Wrapf(err, "failed to check number of files, got %d, want %d", len(files), expectNumber)
		}, &testing.PollOptions{Timeout: timeout})
	}

	err := Combine("open screen record settings",
		ui.LeftClick(statusTrayBtm),
		ui.LeftClick(screenCaptureBtm),
		ui.LeftClick(screenRecordBtn),
		ui.LeftClick(settingsBtn),
		ui.LeftClick(selectBtn),
	)(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to open screen record settings")
	}

	if _, err := os.Stat(downloadsPath); err != nil {
		expectNumber = 1
		base := filepath.Base(downloadsPath)
		testing.ContextLog(ctx, "start recording in temp folder:", base)
		err = Combine("create new folder and start recording",
			ui.LeftClick(myfilesBtn),
			ui.LeftClick(newFolderBtn),
			ui.WaitUntilExists(newFolderTextBox),
			// GoBigSleep: wait for a second to make sure it is editable.
			Sleep(time.Second),
			kb.TypeAction(base),
			kb.AccelAction("Enter"),
			ui.LeftClick(openBtn),
			ui.LeftClick(fullScreenBtn),
			ui.LeftClick(desktop),
			checkRecordFile,
		)(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create new folder")
		}
		return nil
	}
	files, err := os.ReadDir(downloadsPath)
	if err != nil {
		return errors.Wrap(err, "failed to read files from Downloads")
	}
	expectNumber = len(files) + 1
	err = Combine("select folder and start recording",
		ui.LeftClick(openBtn),
		ui.LeftClick(fullScreenBtn),
		ui.LeftClick(desktop),
		checkRecordFile,
	)(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to select folder and start recording")
	}
	return nil
}

// StopRecordFromKBAndSaveOnError stops the record started by StartRecordFromKB.
// If there is error, it copies the record file to the target dir .
// It also removes the record file from Downloads for cleanup.
func StopRecordFromKBAndSaveOnError(ctx context.Context, tconn *chrome.TestConn, hasError func() bool, dir, downloadsPath string) error {
	if err := StopRecordFromUI(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to stop recording")
	}
	if err := SaveRecordFromKBOnError(ctx, tconn, hasError, dir, downloadsPath); err != nil {
		return errors.Wrap(err, "failed to save recording")
	}
	return nil
}

// StopRecordFromUI stops the recording by click the stop button.
// Close "Screen recording completed" notification to avoid conflicts with other UI.
func StopRecordFromUI(ctx context.Context, tconn *chrome.TestConn) error {
	recordResult := nodewith.Name("Screen recording completed").Role(role.Alert)
	screenRordingTaken := nodewith.Name("Screen recording taken").Role(role.GenericContainer)
	screenRordingTakenCloseBtn := nodewith.Role(role.Button).Name("Notification close").HasClass("IconButton")
	ui := New(tconn)
	if err := Combine("stop record",
		ui.LeftClick(ScreenRecordStopButton),
		ui.WaitUntilExists(screenRordingTaken),
		ui.LeftClick(screenRordingTakenCloseBtn),
		ui.WaitUntilExists(recordResult))(ctx); err != nil {
		testing.ContextLog(ctx, "Failed to stop recording: ", err)
		return err
	}
	return nil
}

// ScreenRecordStopButton is the button to stop recording the screen.
var ScreenRecordStopButton = nodewith.Name("Stop screen recording").Role(role.Button)

// SaveRecordFromKBOnError saves the recording from StartRecordFromKB.
// This can be used without StopRecordFromKBAndSaveOnError if the screen recording was stopped automatically (i.e. if the screen was locked).
func SaveRecordFromKBOnError(ctx context.Context, tconn *chrome.TestConn, hasError func() bool, dir, downloadsPath string) error {
	testing.ContextLogf(ctx, "SaveRecordFromKBOnError to: %s", downloadsPath)
	files, err := os.ReadDir(downloadsPath)
	if err != nil {
		return errors.Wrap(err, "failed to read files from Downloads")
	}
	for _, f := range files {
		path := filepath.Join(downloadsPath, f.Name())
		if strings.HasSuffix(f.Name(), ".webm") {
			defer os.RemoveAll(path)
			if hasError() {
				if err := crash.MoveFilesToOut(ctx, dir, path); err != nil {
					return errors.Wrapf(err, "failed to copy records to %s", dir)
				}
				testing.ContextLogf(ctx, "Successfully copied the record file %s to %s", f.Name(), dir)
			}
		}
	}
	return nil
}

// SaveRecordFromKB saves the recording from StartRecordFromKB.
func SaveRecordFromKB(ctx context.Context, tconn *chrome.TestConn, dir, downloadsPath string) error {
	files, err := os.ReadDir(downloadsPath)
	if err != nil {
		return errors.Wrap(err, "failed to read files from Downloads")
	}
	for _, f := range files {
		path := filepath.Join(downloadsPath, f.Name())
		if strings.HasSuffix(f.Name(), ".webm") {
			defer os.RemoveAll(path)
			if err := crash.MoveFilesToOut(ctx, dir, path); err != nil {
				return errors.Wrapf(err, "failed to copy records to %s", dir)
			}
			testing.ContextLogf(ctx, "Successfully copied the record file %s to %s", f.Name(), dir)
		}
	}
	return nil
}

// SaveOneRecordFromKB saves the recording from StartRecordFromKB.
func SaveOneRecordFromKB(ctx context.Context, tconn *chrome.TestConn, filename, downloadsPath string) error {
	files, err := os.ReadDir(downloadsPath)
	if err != nil {
		return errors.Wrap(err, "failed to read files from Downloads")
	}
	testing.ContextLogf(ctx, "Saving files from %s to %s", downloadsPath, filename)
	for _, f := range files {
		path := filepath.Join(downloadsPath, f.Name())
		testing.ContextLogf(ctx, "See file %s %s", f.Name(), path)
		if strings.HasSuffix(f.Name(), ".webm") {
			err := fsutil.MoveFile(path, filename)
			if err != nil {
				return errors.Wrapf(err, "failed to move %q to %q", path, filename)
			}
			break
		}
	}
	return nil
}
