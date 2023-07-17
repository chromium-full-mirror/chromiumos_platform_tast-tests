// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package zoom

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	longUITimeout   = time.Minute      // Used for situations where UI might take a long time to respond.
	mediumUITimeout = 30 * time.Second // Used for situations where UI response are slower.
	shortUITimeout  = 3 * time.Second  // Used for situations where UI response are faster.

	appName = "Zoom"

	zoomWebsite = "https://zoom.us"

	// 'from=pwa' URL param is used in PWA. It is ignored in Web mode.
	newMeetingURL = "https://zoom.us/start/videomeeting?from=pwa"
	pwaInstallURL = "https://pwa.zoom.us/wc"
)

var (
	// Find the web view of Zoom window.
	zoomMainWebArea = nodewith.NameContaining(appName).Role(role.RootWebArea)

	// Below elements represent 4 stages of Zoom app.
	//     MY ACCOUNT / Profile picture: User logged in already.
	//     SIGN IN: User is not signed in yet.
	//     Agree to the Terms of Service: User is in the registration flow.
	//     Launch Meeting: Choose to "Join from Your Browser".
	myAccountLink = nodewith.NameRegex(regexp.MustCompile("(?i)My Account")).Role(role.Link).Ancestor(zoomMainWebArea)
	myProfileImg  = nodewith.Name("Profile picture").Role(role.Image).Ancestor(zoomMainWebArea)
	// There may be multiple "sign in" links, so add First() here.
	signInLink          = nodewith.NameRegex(regexp.MustCompile("(?i)sign in")).Role(role.Link).Ancestor(zoomMainWebArea).First()
	agreeToTermsArea    = nodewith.NameContaining("Agree to the Terms of Service").Role(role.RootWebArea)
	launchMeetingWindow = nodewith.Name("Launch Meeting - Zoom").Role(role.Window)

	// The main canvas of the meeting, it can be used to identify whether if it is in a meeting.
	mainLayoutCanvas = nodewith.HasClass("main-layout__canvas").Role(role.Canvas)

	// End button in the meeting.
	endMenu                = nodewith.Name("End").Role(role.PopUpButton).Ancestor(zoomMainWebArea)
	endMeetingForAllButton = nodewith.Name("End Meeting for All").Role(role.MenuItem).Ancestor(zoomMainWebArea)
	leaveButton            = nodewith.Name("Leave").Role(role.Button).Ancestor(zoomMainWebArea)

	// Video toggle buttons.
	startVideoButton = nodewith.NameRegex(regexp.MustCompile("(Start Video|start sending my video|start my video)")).Role(role.Button).Ancestor(zoomMainWebArea)
	stopVideoButton  = nodewith.NameRegex(regexp.MustCompile("(Stop Video|stop sending my video|stop my video)")).Role(role.Button).Ancestor(zoomMainWebArea)

	// Audio toggle buttons.
	muteButton   = nodewith.NameRegex(regexp.MustCompile("^(?i)mute.*")).Role(role.Button).Ancestor(zoomMainWebArea)
	unmuteButton = nodewith.NameRegex(regexp.MustCompile("^(?i)unmute.*")).Role(role.Button).Ancestor(zoomMainWebArea)
)

// Zoom represents a type of Zoom meeting instance.
type Zoom struct {
	br    *browser.Browser
	conn  *chrome.Conn
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

// PermissionOption represents the option of audio/video permission setup.
type PermissionOption int

const (
	// WithDefaultPermissions uses existing permissions to start the Zoom meeting.
	WithDefaultPermissions PermissionOption = iota

	// WithAllPermissions option grants auido, video and notification permissions before start the Zoom meeting.
	WithAllPermissions
)

// New creates a new Zoom meeting instance.
func New(br *browser.Browser, conn *chrome.Conn, tconn *chrome.TestConn) *Zoom {
	return &Zoom{br, conn, tconn, uiauto.New(tconn)}
}

// NewFromTarget creates a new Zoom meeting instance from an existing web target.
func NewFromTarget(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, tm chrome.TargetMatcher) (*Zoom, error) {
	conn, err := br.NewConnForTarget(ctx, tm)
	if err != nil {
		return nil, err
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	return New(br, conn, tconn), nil
}

// StartNewMeeting starts a new Zoom meeting using given browser.
// It does not join audio by default.
// The caller should explicitly call Close function to release resources and close Chrome browser.
// Example:
//
//	zm, err := zoom.StartNewMeeting(ctx, cr, br, true)
//	if err != nil {
//	     s.Fatal("Failed to start meeting: ", err)
//	}
//	defer zm.Close(cleanupCtx)
func StartNewMeeting(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, conn *chrome.Conn, permissionsOption PermissionOption) (*Zoom, error) {
	if permissionsOption == WithAllPermissions {
		if err := GrantPermissions(ctx, br); err != nil {
			return nil, errors.Wrap(err, "failed to grant permissions")
		}
	}

	if err := navigateToZoomAndSignIn(ctx, cr, br, conn); err != nil {
		return nil, errors.Wrap(err, "failed to navigate to Zoom or sign-in")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	if err := launchNewMeeting(ctx, conn, tconn, newMeetingURL); err != nil {
		return nil, errors.Wrap(err, "failed to launch meeting")
	}

	zm := New(br, conn, tconn)

	if err := prompts.ClearPotentialPrompts(tconn, shortUITimeout, prompts.ShowNotificationsPrompt)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to clear notification prompt")
	}

	if err := webutil.WaitForQuiescence(ctx, conn, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to wait for page finish loading")
	}

	// Do not join audio by default by dismissing the dialog.
	// Assume the dialog is not shown up if not found in a certain time.
	if err := zm.SetJoinAudio(false)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to choose not join audio")
	}

	return zm, nil
}

// JoinMeeting joins a Zoom meeting via invite link.
// And make sure the camera and microphone are turned on before entering the meeting.
func JoinMeeting(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, conn *chrome.Conn, inviteLink string, permissionsOption PermissionOption) (*Zoom, error) {
	if permissionsOption == WithAllPermissions {
		if err := GrantPermissions(ctx, br); err != nil {
			return nil, errors.Wrap(err, "failed to grant permissions")
		}
	}

	if err := navigateToZoomAndSignIn(ctx, cr, br, conn); err != nil {
		return nil, errors.Wrap(err, "failed to navigate to Zoom or sign-in")
	}
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}
	if err := launchNewMeeting(ctx, conn, tconn, inviteLink); err != nil {
		return nil, errors.Wrap(err, "failed to launch meeting")
	}

	return New(br, conn, tconn), nil
}

// StartNewMeetingUsingPWA starts a new Zoom Meeting in PWA mode.
// It automatically installs PWA if it is not installed yet.
// The caller should explicitly call Close function to release resources and close the app.
// Example:
//
//	zm, err := zoom.StartNewMeetingUsingPWA(ctx, cr, br, true)
//	if err != nil {
//	     s.Fatal("Failed to start meeting: ", err)
//	}
//	defer zm.Close(cleanupCtx)
func StartNewMeetingUsingPWA(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, permissionsOption PermissionOption) (*Zoom, error) {
	if permissionsOption == WithAllPermissions {
		if err := GrantPermissions(ctx, br); err != nil {
			return nil, errors.Wrap(err, "failed to grant permissions")
		}
	}

	if err := InstallPWA(ctx, cr, br); err != nil {
		return nil, err
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	pwaTitle := "Zoom"
	pwaTargetMatcher := func(t *chrome.Target) bool {
		return t.Title == pwaTitle
	}

	// PWA is automatically launched after installation.
	// Check if app is already running to avoid double launch.
	if isAppShownOnShelf, err := ash.AppShown(ctx, tconn, apps.Zoom.ID); err != nil {
		return nil, errors.Wrap(err, "failed to check whether Zoom is shown on shelf")
	} else if isAppShownOnShelf {
		if isRunning, err := ash.AppRunning(ctx, tconn, apps.Zoom.ID); err != nil {
			return nil, errors.Wrap(err, "failed to check whether Zoom is already running")
		} else if isRunning {
			// Bring existing Zoom PWA to front.
			if _, err := ash.BringWindowToForeground(ctx, tconn, pwaTitle); err != nil {
				return nil, errors.Wrap(err, "failed to bring Zoom PWA to front")
			}
		}
	} else {
		if err := apps.Launch(ctx, tconn, apps.Zoom.ID); err != nil {
			return nil, err
		}
	}

	zm, err := NewFromTarget(ctx, cr, br, pwaTargetMatcher)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Zoom PWA")
	}

	if err := signIn(ctx, zm.conn, tconn); err != nil {
		return nil, errors.Wrap(err, "failed to sign in on Zoom PWA")
	}

	if err := launchNewMeeting(ctx, zm.conn, tconn, newMeetingURL); err != nil {
		return nil, errors.Wrap(err, "failed to launch meeting")
	}
	if err := prompts.ClearPotentialPrompts(tconn, shortUITimeout, prompts.ShowNotificationsPrompt)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to clear notification prompt")
	}

	if err := webutil.WaitForQuiescence(ctx, zm.conn, time.Minute); err != nil {
		return nil, errors.Wrap(err, "failed to wait for page finish loading")
	}

	// Do not join audio by default by dismissing the dialog.
	// Assume the dialog is not shown up if not found in a certain time.
	if err := zm.SetJoinAudio(false)(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to choose not join audio")
	}

	return zm, nil
}

// Close closes the Zoom meeting browser or PWA app and clean up resources.
func (zm *Zoom) Close(ctx context.Context) error {
	if err := zm.EndMeetingForAll(ctx); err != nil {
		return errors.Wrap(err, "failed to end meeting for all")
	}
	if err := zm.conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close Zoom")
	}
	return zm.conn.Close()
}

// Conn returns the connection to the Meet page target.
func (zm *Zoom) Conn() *chrome.Conn {
	return zm.conn
}

// EnterFullScreen double clicks the screen to enter full screen.
// It skips function if the window is already in full screen mode.
func (zm *Zoom) EnterFullScreen(ctx context.Context) error {
	if err := ash.WaitForFullscreenConditionWithTitle(zm.tconn, appName, true, time.Second); err == nil {
		return nil
	}

	return zm.ui.RetryUntil(
		zm.ui.DoubleClick(mainLayoutCanvas),
		ash.WaitForFullscreenConditionWithTitle(zm.tconn, appName, true, 5*time.Second),
	)(ctx)
}

// ExitFullScreen double clicks the screen to exit full screen.
// It skips function if the window is not in full screen mode.
func (zm *Zoom) ExitFullScreen(ctx context.Context) error {
	if err := ash.WaitForFullscreenConditionWithTitle(zm.tconn, appName, false, time.Second); err == nil {
		return nil
	}

	return zm.ui.RetryUntil(
		zm.ui.DoubleClick(mainLayoutCanvas),
		ash.WaitForFullscreenConditionWithTitle(zm.tconn, appName, false, 5*time.Second),
	)(ctx)
}

// EndMeetingForAll selects "End Meeting For All" in the "End" menu when the user is the host.
// If there is "Leave" button, do nothing.
// This function should be called in cleanup function to ensure the user can host a new meeting successfully.
func (zm *Zoom) EndMeetingForAll(ctx context.Context) error {
	ui := zm.ui
	if err := zm.ShowInterface(ctx); err != nil {
		return err
	}
	endButton, err := ui.FindAnyExists(ctx, endMenu, leaveButton)
	if err != nil {
		return err
	}
	if endButton == leaveButton {
		return nil
	}
	// Only the host needs to end the meeting.
	return uiauto.Combine("end meeting for all",
		ui.DoDefaultUntil(endMenu, ui.WithTimeout(shortUITimeout).WaitUntilExists(endMeetingForAllButton)),
		ui.RetrySilently(3, uiauto.Combine("confirm and wait for canvas gone",
			ui.DoDefault(endMeetingForAllButton),
			ui.WaitUntilGone(mainLayoutCanvas),
		)),
	)(ctx)
}

// InstallPWA installs Zoom PWA.
func InstallPWA(ctx context.Context, cr *chrome.Chrome, br *browser.Browser) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	if alreadyInstalled, err := ash.ChromeAppInstalled(ctx, tconn, apps.Zoom.ID); err != nil {
		return errors.Wrap(err, "failed to check whether Zoom PWA has already been installed")
	} else if alreadyInstalled {
		return nil
	}

	// Install Zoom PWA.
	if err := apps.InstallPWAForURL(ctx, tconn, br, pwaInstallURL, 30*time.Second); err != nil {
		return errors.Wrap(err, "failed to install Zoom PWA")
	}
	return ash.WaitForChromeAppInstalled(ctx, tconn, apps.Zoom.ID, time.Minute)
}

// ShowInterface moves mouse or taps in web area in order to make the menu interface reappear.
func (zm *Zoom) ShowInterface(ctx context.Context) error {
	return zm.ui.LeftClickUntil(zoomMainWebArea,
		zm.ui.WaitForLocation(moreOptionsButton))(ctx)
}

// hideInterface moves mouse to the center point of canvas.
func (zm *Zoom) hideInterface(ctx context.Context) error {
	isNodeFound, err := zm.ui.IsNodeFound(ctx, moreOptionsButton)
	if err != nil {
		return err
	} else if !isNodeFound {
		return nil
	}

	return zm.ui.RetryUntil(
		zm.ui.MouseMoveTo(mainLayoutCanvas, 10*time.Millisecond),
		zm.ui.WaitUntilGone(moreOptionsButton),
	)(ctx)
}

// WaitParticipantsNum waits for the number of participants to reach the expected number.
// When entering a meeting room for the first time, it may take some time to display the
// correct number of participants. Add retry to get the correct participants number.
func (zm *Zoom) WaitParticipantsNum(expectedNumber int) action.Action {
	checkParticipantsNum := func(ctx context.Context) error {
		number, err := zm.GetParticipantsNum(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get the number of the meeting participants")
		}
		if int(number) != expectedNumber {
			return errors.Wrapf(err, "participant number is %d but %d is expected", number, expectedNumber)
		}
		testing.ContextLog(ctx, "Current participants number: ", number)
		return nil
	}
	return zm.ui.WithInterval(time.Second).Retry(10, checkParticipantsNum)
}

// GetParticipantsNum returns the number of meeting participants.
func (zm *Zoom) GetParticipantsNum(ctx context.Context) (int, error) {
	ui := zm.ui
	participantButton := nodewith.NameContaining("open the participants list pane").Role(role.Button)
	noParticipantButton := nodewith.NameContaining("[0] particpants").Role(role.Button)

	if err := uiauto.NamedCombine("wait participant info",
		ui.WaitUntilExists(participantButton),
		ui.WithTimeout(mediumUITimeout).WaitUntilGone(noParticipantButton),
	)(ctx); err != nil {
		return 0, err
	}

	participantInfo, err := ui.Info(ctx, participantButton)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get participant info")
	}
	testing.ContextLog(ctx, "Get participant info: ", participantInfo.Name)

	re := regexp.MustCompile(`\[([0-9]+)\]`)
	match := re.FindStringSubmatch(participantInfo.Name)
	if len(match) != 2 {
		return 0, errors.Wrap(err, "failed to find number of participants")
	}
	number, err := strconv.ParseInt(match[1], 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse number of participants")
	}

	return int(number), nil
}
