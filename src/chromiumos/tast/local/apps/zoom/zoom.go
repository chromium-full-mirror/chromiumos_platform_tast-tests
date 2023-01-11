// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zoom

import (
	"context"
	"regexp"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/browser/browserui"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/prompts"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/crostini/faillog"
	"chromiumos/tast/testing"
)

const (
	longUITimeout   = time.Minute      // Used for situations where UI might take a long time to respond.
	mediumUITimeout = 30 * time.Second // Used for situations where UI response are slower.
	shortUITimeout  = 3 * time.Second  // Used for situations where UI response are faster.

	zoomWebsite        = "https://zoom.us"
	startNewMeetingURL = "https://zoom.us/start/videomeeting"
)

var (
	// Find the web view of Zoom window.
	zoomMainWebArea = nodewith.NameContaining("Zoom").Role(role.RootWebArea)

	// Below elements represent 4 stages of Zoom app.
	//     MY ACCOUNT / Profile picture: User logged in already.
	//     SIGN IN: User is not signed in yet.
	//     Agree to the Terms of Service: User is in the registration flow.
	//     Launch Meeting: Choose to "Join from Your Browser".
	myAccountLink       = nodewith.Name("MY ACCOUNT").Role(role.Link).Ancestor(zoomMainWebArea)
	myProfileImg        = nodewith.Name("Profile picture").Role(role.Image).Ancestor(zoomMainWebArea)
	signInLink          = nodewith.NameRegex(regexp.MustCompile("(?i)sign in")).Role(role.Link).Ancestor(zoomMainWebArea)
	agreeToTermsArea    = nodewith.NameContaining("Agree to the Terms of Service").Role(role.RootWebArea)
	launchMeetingWindow = nodewith.Name("Launch Meeting - Zoom").Role(role.Window)

	// The main canvas of the meeting, it can be used to identify whether if it is in a meeting.
	mainLayoutCanvas = nodewith.HasClass("main-layout__canvas").Role(role.Canvas)

	// End button in the meeting.
	endMenu                = nodewith.Name("End").Role(role.PopUpButton).Ancestor(zoomMainWebArea)
	endMeetingForAllButton = nodewith.Name("End Meeting for All").Role(role.MenuItem).Ancestor(zoomMainWebArea)

	// Toggle video buttons.
	startVideoButton = nodewith.NameRegex(regexp.MustCompile("Start Video|start sending my video|start my video")).Role(role.Button).Ancestor(zoomMainWebArea)
	stopVideoButton  = nodewith.NameRegex(regexp.MustCompile("Stop Video|stop sending my video|stop my video")).Role(role.Button).Ancestor(zoomMainWebArea)
)

// Zoom represents a type of Zoom meeting instance.
type Zoom struct {
	conn  *chrome.Conn
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

// New creates a new Zoom meeting instance.
func New(conn *chrome.Conn, tconn *chrome.TestConn) *Zoom {
	return &Zoom{conn, tconn, uiauto.New(tconn)}
}

// NewFromTarget creates a new Zoom meeting instance from an existing web target.
func NewFromTarget(ctx context.Context, cr *chrome.Chrome, tm chrome.TargetMatcher) (*Zoom, error) {
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

// StartNewMeeting starts a new Zoom meeting using given browser.
// It does not join audio by default.
// The caller should explicitly call cleanup function to release resources and close Chrome browser.
// Example:
//
//	gm, cleanup, err := zoom.StartNewMeeting(ctx, cr, browserType,nil)
//	if err != nil {
//	     s.Fatal("Failed to start meeting: ", err)
//	}
//	defer cleanup(cleanupCtx)
func StartNewMeeting(ctx context.Context, cr *chrome.Chrome, bt browser.Type) (zm *Zoom, cleanup action.Action, retErr error) {
	conn, cleanup, err := navigateToZoomAndSignIn(ctx, cr, bt)
	if err != nil {
		return nil, nil, errors.Wrap(err, "failed to navigate to Zoom or sign-in")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, nil, err
	}

	defer func(ctx context.Context) {
		if retErr != nil {
			faillog.DumpUITreeAndScreenshot(ctx, tconn, "start_new_meeting", retErr)
			if err := cleanup(ctx); err != nil {
				testing.ContextLog(ctx, "Failed to close browser in cleanup")
			}
		}
	}(ctx)

	if err := launchNewMeeting(ctx, conn, tconn); err != nil {
		return nil, nil, errors.Wrap(err, "failed to launch meeting")
	}

	ui := uiauto.New(tconn)
	zm = &Zoom{conn, tconn, ui}

	// New meeting is started. It should be cleaned up as well before close browser.
	cleanup = uiauto.NamedCombine("cleanup Zoom",
		zm.EndMeetingForAll,
		cleanup,
	)

	if err := prompts.ClearPotentialPrompts(tconn, shortUITimeout, prompts.ShowNotificationsPrompt)(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to clear notification prompt")
	}

	// Do not join audio by default by dismissing the dialog.
	// Assume the dialog is not shown up if not found in a certain time.
	if err := zm.SetJoinAudio(false, true)(ctx); err != nil {
		return nil, nil, errors.Wrap(err, "failed to choose not join audio")
	}

	return zm, cleanup, nil
}

// Conn returns the connection to the Meet page target.
func (zm *Zoom) Conn() *chrome.Conn {
	return zm.conn
}

// EnterFullScreen double clicks the screen to enter full screen.
// Browser address bar is used to determine whether Zoom is in full screen mode.
func (zm *Zoom) EnterFullScreen(ctx context.Context) error {
	return uiauto.IfSuccessThen(
		zm.ui.WithTimeout(1*time.Second).WaitUntilExists(browserui.AddressBarFinder),
		zm.ui.RetryUntil(
			zm.ui.DoubleClick(mainLayoutCanvas),
			zm.ui.WithTimeout(shortUITimeout).WaitUntilGone(browserui.AddressBarFinder),
		),
	)(ctx)
}

// ExitFullScreen double clicks the screen to exit full screen.
// Browser address bar is used to determine whether Zoom is in full screen mode.
func (zm *Zoom) ExitFullScreen(ctx context.Context) error {
	return uiauto.IfSuccessThen(
		zm.ui.WithTimeout(1*time.Second).WaitUntilGone(browserui.AddressBarFinder),
		zm.ui.RetryUntil(
			zm.ui.DoubleClick(mainLayoutCanvas),
			zm.ui.WithTimeout(shortUITimeout).WaitUntilExists(browserui.AddressBarFinder),
		),
	)(ctx)
}

// EndMeetingForAll select "End Meeting For All" in the "End" menu.
// This function should be called in cleanup function to ensure the user can host a new meeting successfully.
func (zm *Zoom) EndMeetingForAll(ctx context.Context) error {
	ui := zm.ui
	return uiauto.Combine("end meeting for all",
		zm.showInterface,
		ui.DoDefaultUntil(endMenu, ui.WithTimeout(shortUITimeout).WaitUntilExists(endMeetingForAllButton)),
		ui.DoDefaultUntil(endMeetingForAllButton, ui.WaitUntilGone(mainLayoutCanvas)),
	)(ctx)
}

// showInterface moves mouse or taps in web area in order to make the menu interface reappear.
func (zm *Zoom) showInterface(ctx context.Context) error {
	return zm.ui.LeftClickUntil(mainLayoutCanvas,
		zm.ui.WaitForLocation(endMenu))(ctx)
}

// hideInterface moves mouse to the center point of canvas.
func (zm *Zoom) hideInterface(ctx context.Context) error {
	isNodeFound, err := zm.ui.IsNodeFound(ctx, endMenu)
	if err != nil {
		return err
	} else if !isNodeFound {
		return nil
	}

	return zm.ui.RetryUntil(
		zm.ui.MouseMoveTo(mainLayoutCanvas, 10*time.Millisecond),
		zm.ui.WaitUntilGone(endMenu),
	)(ctx)
}
