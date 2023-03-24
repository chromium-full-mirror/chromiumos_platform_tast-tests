// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"image"
	"net/url"
	"regexp"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/prompts"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/local/screenshot"
	"chromiumos/tast/testing"
)

const (
	longUITimeout   = time.Minute      // Used for situations where UI might take a long time to respond.
	mediumUITimeout = 30 * time.Second // Used for situations where UI response are slower.
	shortUITimeout  = 3 * time.Second  // Used for situations where UI response are faster.

	newMeetingURL = "http://meet.google.com/new"
	homePageURL   = "https://meet.google.com/"

	appName = "Meet"
)

var (
	// Find the web view of Meet window.
	meetRootWebArea = nodewith.NameContaining(appName).Role(role.RootWebArea)

	moreOptionsButton = nodewith.Name("More options").Role(role.PopUpButton).Ancestor(meetRootWebArea)

	endMeetingButton = nodewith.Name("Leave call").Role(role.Button).Ancestor(meetRootWebArea)
	// Use end meeting button to identify whether it is currently in a meeting.
	inMeetingIdentifier = endMeetingButton
)

// VideoNode represents the first video node in the meeting.
var VideoNode = nodewith.Role(role.Video).First().Ancestor(meetRootWebArea)

// GoogleMeet represents a type of GoogleMeet meeting instance.
type GoogleMeet struct {
	br    *browser.Browser
	conn  *chrome.Conn
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

// PermissionOption represents the option of audio/video permission setup.
type PermissionOption int

const (
	// WithDefaultPermissions uses existing permissions to start the meeting app.
	WithDefaultPermissions PermissionOption = iota

	// WithAllPermissions option grants auido, video and notification permissions before start the meeting app.
	WithAllPermissions
)

// New creates a new GoogleMeet meeting instance.
func New(br *browser.Browser, conn *chrome.Conn, tconn *chrome.TestConn) *GoogleMeet {
	return &GoogleMeet{br, conn, tconn, uiauto.New(tconn)}
}

// NewFromTarget creates a new GoogleMeet meeting instance from an existing web target.
func NewFromTarget(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, tm chrome.TargetMatcher) (*GoogleMeet, error) {
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

// StartNewMeeting starts a new Google Meeting using given browser.
// The caller should explicitly call cleanup function to release resources and close Chrome browser.
// Example:
//
//	gm, cleanup, err := googlemeet.StartNewMeeting(ctx, cr, browserType, nil, true)
//	if err != nil {
//	     s.Fatal("Failed to start meeting: ", err)
//	}
//	defer cleanup(cleanupCtx)
func StartNewMeeting(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, urlParams map[string]string, permissionOption PermissionOption) (*GoogleMeet, error) {
	gm, err := startMeeting(ctx, cr, br, newMeetingURL, urlParams, permissionOption)
	if err != nil {
		return gm, err
	}
	return gm, gm.waitUntilInMeeting(ctx)
}

// JoinMeeting joins an existing meeting using given browser.
func JoinMeeting(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, meetingCode string, urlParams map[string]string, permissionOption PermissionOption) (*GoogleMeet, error) {
	gm, err := startMeeting(ctx, cr, br, homePageURL+meetingCode, urlParams, permissionOption)
	if err != nil {
		return gm, err
	}
	return gm, gm.joinConference(ctx)
}

func startMeeting(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, meetingURL string, urlParams map[string]string, permissionOption PermissionOption) (*GoogleMeet, error) {
	if permissionOption == WithAllPermissions {
		if err := GrantPermissions(ctx, br); err != nil {
			return nil, errors.Wrap(err, "failed to grant permissions to Meet")
		}
	}

	if urlParams != nil && len(urlParams) > 0 {
		values := url.Values{}
		for k, v := range urlParams {
			values.Add(k, v)
		}
		meetingURL = meetingURL + "?" + values.Encode()
	}

	conn, err := br.NewTab(ctx, meetingURL)
	if err != nil {
		return nil, err
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	gm := New(br, conn, tconn)

	if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
		return nil, errors.Wrapf(err, "failed to wait for %q to be loaded and achieve quiescence", newMeetingURL)
	}

	if err := gm.ClearPromptsForNewMeeting(ctx); err != nil {
		return nil, err
	}

	return gm, nil
}

// StartNewMeetingUsingPWA starts a new Google Meeting in PWA mode.
// It automatically installs PWA if it is not installed yet.
// The caller should explicitly call cleanup function to release resources and close the app.
// Example:
//
//	gm, cleanup, err := googlemeet.StartNewMeetingUsingPWA(ctx, cr, browserType, true)
//	if err != nil {
//	     s.Fatal("Failed to start meeting: ", err)
//	}
//	defer cleanup(cleanupCtx)
func StartNewMeetingUsingPWA(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, permissionOption PermissionOption) (*GoogleMeet, error) {
	gm, err := startMeetingUsingPWA(ctx, cr, br, newMeetingURL, permissionOption)
	if err != nil {
		return gm, err
	}
	return gm, gm.waitUntilInMeeting(ctx)
}

// JoinMeetingUsingPWA joins an existing meeting using PWA.
func JoinMeetingUsingPWA(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, meetingCode string, permissionOption PermissionOption) (*GoogleMeet, error) {
	gm, err := startMeetingUsingPWA(ctx, cr, br, homePageURL+meetingCode, permissionOption)
	if err != nil {
		return gm, err
	}
	return gm, gm.joinConference(ctx)
}

func startMeetingUsingPWA(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, meetingURL string, permissionOption PermissionOption) (*GoogleMeet, error) {
	if permissionOption == WithAllPermissions {
		if err := GrantPermissions(ctx, br); err != nil {
			return nil, errors.Wrap(err, "failed to grant permissions to Meet")
		}
	}

	if err := InstallPWA(ctx, cr, br); err != nil {
		return nil, err
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

	pwaTitle := "Google Meet"
	pwaTargetMatcher := func(t *chrome.Target) bool {
		return t.Title == pwaTitle
	}

	// PWA is automatically launched after installation.
	// Check if app is already running to avoid double launch.
	if isAppShownOnShelf, err := ash.AppShown(ctx, tconn, apps.Meet.ID); err != nil {
		return nil, errors.Wrap(err, "failed to check whether Meet is shown on shelf")
	} else if isAppShownOnShelf {
		if isRunning, err := ash.AppRunning(ctx, tconn, apps.Meet.ID); err != nil {
			return nil, errors.Wrap(err, "failed to check whether Meet is already running")
		} else if isRunning {
			// Bring existing Meet PWA to front.
			if _, err := ash.BringWindowToForeground(ctx, tconn, pwaTitle); err != nil {
				return nil, errors.Wrap(err, "failed to bring Meet PWA to front")
			}
		}
	} else {
		if err := apps.Launch(ctx, tconn, apps.Meet.ID); err != nil {
			return nil, err
		}
	}

	gm, err := NewFromTarget(ctx, cr, br, pwaTargetMatcher)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to Meet PWA")
	}

	if err := gm.conn.Navigate(ctx, meetingURL); err != nil {
		return nil, errors.Wrap(err, "failed to start new meeting")
	}
	if err := webutil.WaitForQuiescence(ctx, gm.conn, longUITimeout); err != nil {
		return nil, errors.Wrapf(err, "failed to wait for %q to be loaded and achieve quiescence", newMeetingURL)
	}

	if err := gm.ClearPromptsForNewMeeting(ctx); err != nil {
		return nil, err
	}

	return gm, nil
}

// Conn returns the connection to the Meet page target.
func (gm *GoogleMeet) Conn() *chrome.Conn {
	return gm.conn
}

// Close closes the Meeting browser or PWA app.
func (gm *GoogleMeet) Close(ctx context.Context) error {
	return func(ctx context.Context) error {
		if err := gm.conn.CloseTarget(ctx); err != nil {
			return err
		}
		return gm.conn.Close()
	}(ctx)
}

// EnterFullScreen changes setting to turn full screen mode.
func (gm *GoogleMeet) EnterFullScreen(ctx context.Context) error {
	fullScreenButton := nodewith.Name("Full screen").Role(role.MenuItem).Ancestor(meetRootWebArea)

	return uiauto.Combine("enter full screen",
		gm.ui.DoDefault(moreOptionsButton),
		gm.ui.DoDefault(fullScreenButton),
		ash.WaitForFullscreenConditionWithTitle(gm.tconn, appName, true, 5*time.Second),
	)(ctx)
}

// SwitchMicrophone turns on/off the microphone on main screen.
// It assumes the microphone is turned off if not available.
func (gm *GoogleMeet) SwitchMicrophone(expectedOn bool) action.Action {
	microphoneButton := nodewith.NameRegex(regexp.MustCompile("Turn (on|off) microphone.*")).Role(role.Button)
	actionDesc := "switch off microphone"
	if expectedOn {
		actionDesc = "switch on microphone"
	}

	return func(ctx context.Context) error {
		info, err := gm.ui.WithTimeout(shortUITimeout).Info(ctx, microphoneButton)
		if err != nil {
			return errors.Wrap(err, "failed to wait for the meet microphone switch button to show")
		}
		// Skip action if the microphone is already as expected.
		if (strings.HasPrefix(info.Name, "Turn on") && !expectedOn) || (strings.HasPrefix(info.Name, "Turn off") && expectedOn) {
			return nil
		}

		// If microphone permission is not yet granted,
		// switching on microphone should handle permission prompt.
		// Otherwise it is straightforward to switch on/off.
		microphoneButton = nodewith.Name(info.Name).Role(role.Button)
		if expectedOn {
			return prompts.ActionAndGrantPermissionIfRequired(
				gm.tconn, gm.conn, gm.ui.DoDefault(microphoneButton), webutil.PermissionMicrophone)(ctx)
		}

		if err := gm.ui.DoDefaultUntil(
			microphoneButton,
			gm.ui.WithTimeout(shortUITimeout).WaitUntilGone(microphoneButton),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to %q", actionDesc)
		}
		return nil
	}
}

// SwitchVideo turns on/off the camera on main screen.
// It assumes the camera is turned off if not available.
func (gm *GoogleMeet) SwitchVideo(expectedOn bool) action.Action {
	cameraButton := nodewith.NameRegex(regexp.MustCompile("Turn (on|off) camera.*")).Role(role.Button)
	actionDesc := "switch off camera"
	if expectedOn {
		actionDesc = "switch on camera"
	}

	return func(ctx context.Context) error {
		info, err := gm.ui.WithTimeout(shortUITimeout).Info(ctx, cameraButton)
		if err != nil {
			return errors.Wrap(err, "failed to wait for the camera switch button to show")
		}
		// Skip action if the camera is already as expected.
		if (strings.HasPrefix(info.Name, "Turn on") && !expectedOn) || (strings.HasPrefix(info.Name, "Turn off") && expectedOn) {
			return nil
		}

		cameraButton = nodewith.Name(info.Name).Role(role.Button)
		// Switch on video should check permission status to decide whether need to handle permission prompt.
		if expectedOn {
			return uiauto.RetrySilently(5, uiauto.Combine("turn on the camera",
				prompts.ActionAndGrantPermissionIfRequired(
					gm.tconn, gm.conn, gm.ui.DoDefault(cameraButton), webutil.PermissionCamera),
				gm.ui.WithTimeout(5*time.Second).WaitUntilGone(cameraButton)),
			)(ctx)
		}

		// Otherwise it is straightforward to switch video.
		if err := gm.ui.DoDefaultUntil(
			cameraButton,
			gm.ui.WithTimeout(shortUITimeout).WaitUntilGone(cameraButton),
		)(ctx); err != nil {
			return errors.Wrapf(err, "failed to %s", actionDesc)
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

// InstallPWA installs Google Meet PWA.
func InstallPWA(ctx context.Context, cr *chrome.Chrome, br *browser.Browser) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return err
	}

	if alreadyInstalled, err := ash.ChromeAppInstalled(ctx, tconn, apps.Meet.ID); err != nil {
		return errors.Wrap(err, "failed to check whether Meet PWA has already been installed")
	} else if alreadyInstalled {
		return nil
	}

	// Install Meet PWA.
	if err := apps.InstallPWAForURL(ctx, tconn, br, homePageURL, 30*time.Second); err != nil {
		return errors.Wrap(err, "failed to install Meet PWA")
	}
	return ash.WaitForChromeAppInstalled(ctx, tconn, apps.Meet.ID, time.Minute)
}

// ScreenshotCanvas takes screenshot of the canvas via Javascript.
func (gm *GoogleMeet) ScreenshotCanvas(ctx context.Context, cr *chrome.Chrome) (image.Image, error) {
	videoNodeInfo, err := gm.ui.Info(ctx, VideoNode)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get video node info")
	}

	return screenshot.GrabAndCropScreenshot(ctx, cr, videoNodeInfo.Location)
}

// joinConference joins the conference from the home screen.
// It deals with a few scenarios to enter google meet room:
// 1. If joins the meeting automatically skipping the home sreen, do nothing.
// 2. If there is a "Join now" or "Ask for join" button, click it.
func (gm *GoogleMeet) joinConference(ctx context.Context) error {
	joinNowButton := nodewith.Name("Join now").Role(role.Button)
	askToJoinButton := nodewith.Name("Ask to join").Role(role.Button)

	nodeFinder, err := gm.ui.FindAnyExists(ctx, joinNowButton, askToJoinButton, inMeetingIdentifier)
	if err != nil {
		return err
	}

	// Do nothing if the user joins the meeting automatically.
	if nodeFinder == inMeetingIdentifier {
		return nil
	}

	return uiauto.Combine("join meeting",
		gm.ui.WithTimeout(longUITimeout).DoDefaultUntil(
			nodeFinder,
			// The joining process takes a while. Using default timeout here.
			gm.ui.WaitUntilGone(nodeFinder)),
		gm.waitUntilInMeeting,
		gm.ClearPromptsForNewMeeting,
	)(ctx)
}

func (gm *GoogleMeet) waitUntilInMeeting(ctx context.Context) error {
	return gm.ui.WithTimeout(mediumUITimeout).WaitUntilExists(inMeetingIdentifier)(ctx)
}

// GrantPermissions grants Microphone, Camera and Notifications permissions to Google Meet.
func GrantPermissions(ctx context.Context, br *browser.Browser) error {
	meetURLPatterns := []string{"*://meet.google.com/*"}
	return br.GrantPermissions(ctx, meetURLPatterns,
		browser.CameraContentSetting,
		browser.MicrophoneContentSetting,
		browser.NotificationsContentSetting,
	)
}
