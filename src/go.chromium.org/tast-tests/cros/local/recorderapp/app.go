// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	recorderTestPageURL = "chrome://recorder-app/#/test"
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
func StartAppWithSetup(ctx context.Context, cr *chrome.Chrome, setup Setup) (app *App, retErr error) {
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
	return a.RequestRecordingSummaryWithTimeout(30 * time.Second)
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
	return a.RequestTitleSuggestionsWithTimeout(30 * time.Second)
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
