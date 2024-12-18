// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/checked"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/restriction"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	recorderTestPageURL = "chrome://recorder-app/#/test"
	modelActionTimeout  = 4 * time.Minute
)

// App represents a ChromeOS Recorder App instance.
type App struct {
	conn  *chrome.Conn
	cr    *chrome.Chrome
	tconn *chrome.TestConn
}

// StartApp starts the Recorder App with default setup.
func StartApp(ctx context.Context, cr *chrome.Chrome) (app *App, retErr error) {
	return StartAppWithSetup(ctx, cr, Setup{})
}

// StartAppWithSetup starts the Recorder App with the specified setup.
func StartAppWithSetup(ctx context.Context, cr *chrome.Chrome, setup Setup) (retApp *App, retErr error) {
	// Reserve time to close the app in case there is an error.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	app, err := openAppToTestPage(ctx, cr)
	if err != nil {
		return nil, errors.Wrap(err, "failed to launch Recorder app")
	}
	defer func() {
		if retErr != nil {
			app.Close(cleanupCtx)
		}
	}()

	if err := app.ensureModelInstalled(ctx, setup); err != nil {
		return nil, errors.Wrap(err, "failed to ensure the necessary DLCs are installed")
	}

	if err := app.performSetup(ctx, setup); err != nil {
		return nil, errors.Wrap(err, "failed to perform setup")
	}

	// Go to main page, from the test page, to make the app ready for the test.
	if app.conn.Eval(ctx, "TestHelper.goToMainPage()", nil); err != nil {
		return nil, errors.Wrap(err, "failed to go to main page")
	}
	if app.WaitUntilExists(MainPage)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the UI to be ready")
	}

	testing.ContextLog(ctx, "Recorder App launched")
	return app, nil
}

// openAppToTestPage launches the Recorder App to its test page and connects to
// the test helper in the app side.
func openAppToTestPage(ctx context.Context, cr *chrome.Chrome) (app *App, retErr error) {
	// Reserve time to close the app in case there is an error.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Test API")
	}

	if err := apps.LaunchSystemWebApp(ctx, tconn, apps.Recorder.Name, recorderTestPageURL); err != nil {
		return nil, errors.Wrap(err, "failed to launch Recorder app")
	}
	defer func() {
		if retErr != nil {
			apps.Close(cleanupCtx, tconn, apps.Recorder.ID)
		}
	}()

	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(recorderTestPageURL))
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to the app")
	}

	// Wait for the app to initialize by observing the app UI element.
	if err := conn.WaitForExprWithTimeout(ctx, "document.querySelector('recorder-app') !== null", 10*time.Second); err != nil {
		return nil, errors.Wrap(err, "failed to wait for the app to initialize")
	}

	// Connect to the test helper to use util functions from the JS side.
	code := `(async function() {
		const {TestHelper} = await import('/core/test_helper.js');
		window.TestHelper = TestHelper;
	})()`
	if err := conn.Eval(ctx, code, nil); err != nil {
		return nil, errors.Wrap(err, "failed to connect to the test helper")
	}

	return &App{conn, cr, tconn}, nil
}

// Close cleans up the local cached data and closes the app.
func (a *App) Close(ctx context.Context) error {
	if err := a.conn.Eval(ctx, "TestHelper.removeCacheData()", nil); err != nil {
		return errors.Wrap(err, "failed to clear cached data in the app")
	}
	return apps.Close(ctx, a.tconn, apps.Recorder.ID)
}

// SetWindowStateAndWait sets the state of the Recorder App window and wait
// until the window is already updated to the target state.
func (a *App) SetWindowStateAndWait(ctx context.Context, targetState ash.WindowStateType) error {
	window, err := ash.FindWindow(ctx, a.tconn, func(w *ash.Window) bool {
		return w.Title == apps.Recorder.Name
	})
	if err != nil {
		return errors.Wrap(err, "failed to find the Recorder App window")
	}
	return ash.SetWindowStateAndWait(ctx, a.tconn, window.ID, targetState)
}

// StartRecording returns a function to start recording audio.
func (a *App) StartRecording() uiauto.Action {
	return uiauto.Combine("Start recording",
		a.ClickWhenExists(StartRecordingButton),
		a.WaitUntilExistsFor(RecordAudioWaveform, 10*time.Second),
	)
}

// StopRecording returns a function to stop recording audio.
func (a *App) StopRecording() uiauto.Action {
	return uiauto.Combine("Stop recording",
		a.ClickWhenExists(StopRecordingButton),
		// Long audio may take long time to save.
		a.WaitUntilExistsFor(PlaybackPage, 10*time.Second),
	)
}

// PlayFirstRecording returns a function to click on the first recording on the
// main page and wait until the playback page is ready.
func (a *App) PlayFirstRecording() uiauto.Action {
	return uiauto.Combine("Clicking on the first recording",
		a.ClickWhenExists(FirstRecordingCard),
		a.ClickWhenExists(PlaybackPlayButton),
		a.WaitUntilExists(PlaybackPauseButton),
	)
}

// RequestRecordingSummary returns a function to toggle the summary button in
// the playback page.
func (a *App) RequestRecordingSummary() uiauto.Action {
	return a.RequestRecordingSummaryWithTimeout(modelActionTimeout)
}

// RequestRecordingSummaryWithTimeout returns a function to toggle the summary
// and wait for the summary result with the specified duration.
func (a *App) RequestRecordingSummaryWithTimeout(timeout time.Duration) uiauto.Action {
	return uiauto.Combine("Requesting the recording summary",
		a.ClickWhenExists(ToggleSummaryButton),
		a.WaitUntilExistsFor(SummaryContainer, timeout),
	)
}

// RequestTitleSuggestions returns a function to request and wait for the title
// suggestions.
func (a *App) RequestTitleSuggestions() uiauto.Action {
	return a.RequestTitleSuggestionsWithTimeout(modelActionTimeout)
}

// RequestTitleSuggestionsWithTimeout returns a function to request and wait for
// the title suggestions for the specified duration.
func (a *App) RequestTitleSuggestionsWithTimeout(timeout time.Duration) uiauto.Action {
	return uiauto.Combine("Requesting title suggestions",
		a.ClickWhenExists(RenameTitleText),
		a.ClickWhenExists(SuggestTitleButton),
		a.WaitUntilExistsFor(FirstSuggestedTitle, timeout),
	)
}

// GoBackToMainPage returns a function to click on back button in the playback
// page to go back to main page.
func (a *App) GoBackToMainPage() uiauto.Action {
	return uiauto.Combine("Clicking on back button",
		a.ClickWhenExists(PlaybackBackButton),
		a.WaitUntilExists(MainPage),
	)
}

// WaitUntilPlaybackFinished waits until the playback finishes. The specified
// timeout should be greater than the audio duration.
func (a *App) WaitUntilPlaybackFinished(timeout time.Duration) uiauto.Action {
	return a.WaitUntilGoneFor(PlaybackPauseButton, timeout)
}

// SaveRecordingFiles downloads audio or transcript files based on requirement and
// returns the file paths. This function assumes that Recorder App is already shown.
func (a *App) SaveRecordingFiles(ctx context.Context, exportAudio, exportTranscript bool, username string) (dumpFilePaths []string, err error) {
	downloadsPath, err := cryptohome.DownloadsPath(ctx, username)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Downloads path")
	}

	dumpStartTime := time.Now()
	if err := a.exportRecording(ctx, exportAudio, exportTranscript); err != nil {
		return nil, errors.Wrap(err, "failed to export recording")
	}

	// Assume Recorder dump file name should start with "Audio recording".
	const (
		recorderFileName    = "Audio recording*.*"
		transcriptExtension = ".txt"
		audioExtension      = ".webm"
	)
	downloadStartTime := time.Now()
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		files, err := filepath.Glob(filepath.Join(downloadsPath, recorderFileName))
		if err != nil {
			return errors.Wrap(err, "failed to glob recorder file")
		}
		if len(files) == 0 {
			return errors.New("file not found")
		}
		for _, file := range files {
			ext := filepath.Ext(file)
			if ext != transcriptExtension && ext != audioExtension {
				continue
			}

			fState, err := os.Stat(file)
			if err != nil {
				continue
			}
			if fState.ModTime().After(dumpStartTime) {
				dumpFilePaths = append(dumpFilePaths, file)
			}
		}
		if len(dumpFilePaths) == 0 {
			return errors.Errorf("cannot find file modified after %v", dumpStartTime)
		}
		return nil
	}, &testing.PollOptions{Timeout: 2 * time.Minute, Interval: 3 * time.Second}); err != nil {
		return nil, errors.Wrap(err, "failed to find recorder dump file in Downloads folder")
	}
	testing.ContextLog(ctx, "Downloaded recorder recording file in ", time.Since(downloadStartTime))

	return dumpFilePaths, nil
}

// exportRecording exports the audio or the transcript files based on requirement.
func (a *App) exportRecording(ctx context.Context, exportAudio, exportTranscript bool) error {
	if !exportAudio && !exportTranscript {
		return errors.New("must export either the audio or the transcript file")
	}

	recorderWebArea := nodewith.Name("Recorder").Role(role.RootWebArea)
	moreOptionsButton := nodewith.Name("More options").Role(role.PopUpButton).Ancestor(recorderWebArea)
	exportButton := nodewith.Name("Export").Role(role.MenuItem).Ancestor(recorderWebArea)
	exportAudioButton := nodewith.Name("Export audio").Role(role.CheckBox).Ancestor(recorderWebArea)
	exportTranscriptButton := nodewith.Name("Export transcript").Role(role.CheckBox).Ancestor(recorderWebArea)
	saveButton := nodewith.NameContaining("Save").Role(role.Button).Ancestor(recorderWebArea)

	ui := uiauto.New(a.tconn)
	clickCheckBox := func(checkBoxFinder *nodewith.Finder, expectedCheckedStatus bool) action.Action {
		return func(ctx context.Context) error {
			nodeInfo, err := ui.Info(ctx, checkBoxFinder)
			if err != nil {
				return err
			}
			expectedChecked := checked.False
			if expectedCheckedStatus {
				expectedChecked = checked.True
			}
			if expectedChecked != nodeInfo.Checked {
				if nodeInfo.Restriction != restriction.None {
					return errors.Errorf("the checkbox %q can not be checked", checkBoxFinder.Pretty())
				}
				return ui.DoDefault(checkBoxFinder)(ctx)
			}
			return nil
		}
	}

	return uiauto.Combine("export recording",
		ui.DoDefaultUntil(moreOptionsButton, ui.Exists(exportButton)),
		ui.DoDefaultUntil(exportButton, ui.Exists(exportAudioButton)),
		clickCheckBox(exportAudioButton, exportAudio),
		clickCheckBox(exportTranscriptButton, exportTranscript),
		ui.DoDefault(saveButton),
	)(ctx)
}
