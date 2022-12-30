// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"net/url"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/testing"
)

const (
	longUITimeout   = time.Minute      // Used for situations where UI might take a long time to respond.
	mediumUITimeout = 30 * time.Second // Used for situations where UI response are slower.
	shortUITimeout  = 3 * time.Second  // Used for situations where UI response are faster.
)

var (
	// Find the web view of Meet window.
	meetingWebview = nodewith.ClassName("ContentsWebView").Role(role.WebView)

	moreOptionsButton = nodewith.Name("More options").Role(role.PopUpButton).Ancestor(meetingWebview)
)

// GoogleMeet represents a type of GoogleMeet meeting instance.
type GoogleMeet struct {
	conn  *chrome.Conn
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

// New creates a new GoogleMeet meeting instance.
func New(conn *chrome.Conn, tconn *chrome.TestConn) *GoogleMeet {
	return &GoogleMeet{conn, tconn, uiauto.New(tconn)}
}

// NewFromTarget creates a new GoogleMeet meeting instance from an existing web target.
func NewFromTarget(ctx context.Context, cr *chrome.Chrome, tm chrome.TargetMatcher) (*GoogleMeet, error) {
	conn, err := cr.NewConnForTarget(ctx, tm)
	if err != nil {
		return nil, err
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	return New(conn, tconn), nil
}

// StartNewMeeting starts a new Google Meeting using given browser.
// The caller should explicitly call cleanup function to release resources and close Chrome browser.
// Example:
//
//	gm, cleanup, err := googlemeet.StartNewMeeting(ctx, cr, browserType,nil)
//	if err != nil {
//	     s.Fatal("Failed to start meeting: ", err)
//	}
//	defer cleanup(cleanupCtx)
func StartNewMeeting(ctx context.Context, cr *chrome.Chrome, browserType browser.Type, urlParams map[string]string) (*GoogleMeet, action.Action, error) {
	newMeetingURL := "http://meet.google.com/new"
	if urlParams != nil && len(urlParams) > 0 {
		values := url.Values{}
		for k, v := range urlParams {
			values.Add(k, v)
		}
		newMeetingURL = newMeetingURL + "?" + values.Encode()
	}

	conn, _, closeBrowser, err := browserfixt.SetUpWithURL(ctx, cr, browserType, newMeetingURL)
	if err != nil {
		return nil, nil, err
	}

	if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
		return nil, closeBrowser, errors.Wrapf(err, "failed to wait for %q to be loaded and achieve quiescence", newMeetingURL)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, closeBrowser, err
	}

	gm := New(conn, tconn)

	// Close the "Your meeting's ready" dialog.
	if err := gm.ClearPromptsForNewMeeting(ctx); err != nil {
		return nil, closeBrowser, err
	}

	return gm, closeBrowser, nil
}

// Conn returns the connection to the Meet page target.
func (gm *GoogleMeet) Conn() *chrome.Conn {
	return gm.conn
}

// EnterFullScreen changes setting to turn full screen mode.
func (gm *GoogleMeet) EnterFullScreen(ctx context.Context) error {
	fullScreenButton := nodewith.Name("Full screen").Role(role.MenuItem).Ancestor(meetingWebview)

	return uiauto.Combine("enter full screen",
		gm.ui.DoDefault(moreOptionsButton),
		gm.ui.DoDefault(fullScreenButton),
	)(ctx)
}

// SwitchMicrophone turns on / off the microphone on main screen.
// It assumes the microphone is turned off if not available.
func (gm *GoogleMeet) SwitchMicrophone(value bool) action.Action {
	microphoneButton := nodewith.NameRegex(regexp.MustCompile("Turn (on|off) microphone.*")).Role(role.Button)
	actionDesc := "switch off microphone"
	if value {
		actionDesc = "switch on microphone"
	}

	return func(ctx context.Context) error {
		info, err := gm.ui.WithTimeout(shortUITimeout).Info(ctx, microphoneButton)
		if err != nil {
			return errors.Wrap(err, "failed to wait for the meet microphone switch button to show")
		}
		// Skip action if the microphone is already as expected.
		if (strings.HasPrefix(info.Name, "Turn on") && value) || (strings.HasPrefix(info.Name, "Turn off") && !value) {
			microphoneButton = nodewith.Name(info.Name).Role(role.Button)
			if err := gm.ui.DoDefaultUntil(
				microphoneButton,
				gm.ui.WithTimeout(shortUITimeout).WaitUntilGone(microphoneButton),
			)(ctx); err != nil {
				return errors.Wrapf(err, "failed to %q", actionDesc)
			}
		}

		return nil
	}
}

// MuteIfMicAvailable turns off the microphone on main screen if it is available.
func (gm *GoogleMeet) MuteIfMicAvailable(ctx context.Context) error {
	err := gm.SwitchMicrophone(false)(ctx)
	if err == nil {
		return nil
	}

	// Check if microphone is unavailable.
	microphoneProblemButton := nodewith.NameStartingWith("Microphone problem").Role(role.Button).First()
	if err := gm.ui.Exists(microphoneProblemButton)(ctx); err == nil {
		testing.ContextLog(ctx, "Microphone is unavailable. Skip mute action")
		return nil
	}
	return err
}

// ChangeSettings changes one more settings from main screen.
// It opens the settings page before and closes it after.
func (gm *GoogleMeet) ChangeSettings(actions ...action.Action) action.Action {
	actionsToPerform := []action.Action{gm.OpenSettings}
	actionsToPerform = append(actionsToPerform, actions...)
	actionsToPerform = append(actionsToPerform, gm.CloseSettings)
	return uiauto.NamedCombine("change settings",
		actionsToPerform...,
	)
}

// ApplyVideoEffects applies one more video effects from main screen.
// It opens the video effects page before and closes it after.
func (gm *GoogleMeet) ApplyVideoEffects(actions ...action.Action) action.Action {
	actionsToPerform := []action.Action{gm.OpenVideoEffects}
	actionsToPerform = append(actionsToPerform, actions...)
	actionsToPerform = append(actionsToPerform, gm.CloseVideoEffects)
	return uiauto.NamedCombine("apply video effects",
		actionsToPerform...,
	)
}
