// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package recorderapp

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/errors"
)

// ComponentQuery represents the UI component in Recorder App.
type ComponentQuery struct {
	// Name is used to represent the component name when logging error.
	Name string
	// Query is Javascript code used to query the component.
	Query string
}

// Component creates ComponentQuery based on the given key. Valid `key` values
// are the keys in `UI_COMPONENTS` constant defined in
// chromium/src/ash/webui/recorder_app_ui/resources/core/test_helper.ts.
func Component(key string) ComponentQuery {
	return ComponentQuery{
		Name:  key,
		Query: fmt.Sprintf(`TestHelper.resolveComponent('%s')`, key),
	}
}

var (
	// FirstSuggestedTitle is the first of all suggested recording titles.
	FirstSuggestedTitle = Component("firstSuggestedTitle")
	// FirstRecordingCard is the first recording listed in the main page.
	FirstRecordingCard = Component("firstRecordingCard")
	// MainPage is the page container for main page.
	MainPage = Component("mainPage")
	// PlaybackBackButton is the button to go to main page from the playback page.
	PlaybackBackButton = Component("playbackBackButton")
	// PlaybackPage is the page container for playback page.
	PlaybackPage = Component("playbackPage")
	// PlaybackPauseButton is the button used to pause the audio playback.
	PlaybackPauseButton = Component("playbackPauseButton")
	// PlaybackTranscriptionToggleButton is the toggle button to show/hide
	// transcription in playback page.
	PlaybackTranscriptionToggleButton = Component("playbackTranscriptionToggleButton")
	// RecordPage is the container for the record page.
	RecordPage = Component("recordPage")
	// RenameTitleText is the recording title that can be clicked to rename.
	RenameTitleText = Component("renameTitleText")
	// SuggestTitleButton is the button used to get suggestions for title.
	SuggestTitleButton = Component("suggestTitleButton")
	// SummaryContainer is the container for recording summary.
	SummaryContainer = Component("summaryContainer")
	// StartRecordingButton is the button used to start the recording.
	StartRecordingButton = Component("startRecordingButton")
	// StopRecordingButton is the button used to stop the recording.
	StopRecordingButton = Component("stopRecordingButton")
	// ToggleSummaryButton is the toggle button to show/hide recording summary.
	ToggleSummaryButton = Component("toggleSummaryButton")
)

// PlaybackPlayButton is the button used to play the audio playback.
// TODO(b/355374546): Update to use `Component()` once the change from the app side is upreved.
var PlaybackPlayButton = ComponentQuery{
	Name:  "playbackPlayButton",
	Query: `document.querySelector('recorder-app').shadowRoot.querySelector('playback-page').shadowRoot.querySelector('cra-icon-button[id="play-button"]')`,
}

// ClickImmediately returns an action to click on the element resolved from the query.
func (a *App) ClickImmediately(query ComponentQuery) uiauto.Action {
	return func(ctx context.Context) error {
		code := fmt.Sprintf(`%s.click()`, query.Query)
		if err := a.conn.Eval(ctx, code, nil); err != nil {
			return errors.Wrapf(err, "failed to click on %s", query.Name)
		}
		return nil
	}
}

// WaitUntilExists waits until the queried component exists for the default duration (5 seconds).
func (a *App) WaitUntilExists(query ComponentQuery) uiauto.Action {
	return a.WaitUntilExistsFor(query, 5*time.Second)
}

// WaitUntilExistsFor waits until the queried component exists for the specified duration.
func (a *App) WaitUntilExistsFor(query ComponentQuery, duration time.Duration) uiauto.Action {
	return func(ctx context.Context) error {
		code := fmt.Sprintf(`%s instanceof Element`, query.Query)
		if err := a.conn.WaitForExprWithTimeout(ctx, code, duration); err != nil {
			return errors.Wrapf(err, "failed to wait until %s exists", query.Name)
		}
		return nil
	}
}

// ClickWhenExists waits until the queried component exists and then click the
// component. This is a recommended way to perform click since it is common that
// the component is lazy loaded and not exists immediately.
func (a *App) ClickWhenExists(query ComponentQuery) uiauto.Action {
	return uiauto.Combine(fmt.Sprintf("Wait until %s exists and click", query.Name),
		a.WaitUntilExists(query),
		a.ClickImmediately(query),
	)
}

// WaitUntilGone waits until the queried component exists for the default duration (5 seconds).
func (a *App) WaitUntilGone(query ComponentQuery) uiauto.Action {
	return a.WaitUntilGoneFor(query, 5*time.Second)
}

// WaitUntilGoneFor waits until the queried component not exists for the specified duration.
func (a *App) WaitUntilGoneFor(query ComponentQuery, duration time.Duration) uiauto.Action {
	return func(ctx context.Context) error {
		code := fmt.Sprintf(`(function() {
			try {
				return !(%s instanceof Element);
			} catch(e) {
				// UI query may throw the error when the element is not exists.
				return true;
			}
		})()`, query.Query)
		if err := a.conn.WaitForExprWithTimeout(ctx, code, duration); err != nil {
			return errors.Wrapf(err, "failed to wait until %s gone", query.Name)
		}
		return nil
	}
}
