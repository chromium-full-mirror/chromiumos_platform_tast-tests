// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package googlemeet

import (
	"context"
	"image"
	"net/url"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	chromeApps "go.chromium.org/tast-tests/cros/local/chrome/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/screenshot"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
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

	endMeetingButton = nodewith.Name("Leave call").Role(role.Button).Ancestor(meetRootWebArea)
	// Use end meeting button to identify whether it is currently in a meeting.
	inMeetingIdentifier = endMeetingButton

	// There may be multiple static text, just return first one under meet page.
	firstText = nodewith.Role(role.StaticText).Ancestor(meetRootWebArea).First()

	// There may be multiple "Stop sharing" buttons, so add First() here.
	stopSharing = nodewith.Name("Stop sharing").Role(role.Button).First()

	chatButton       = nodewith.Name("Chat with everyone").Role(role.ToggleButton).Ancestor(meetRootWebArea)
	chatPanelHeading = nodewith.Name("In-call messages").Role(role.Heading).Ancestor(meetRootWebArea)
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
func StartNewMeeting(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, conn *chrome.Conn, urlParams map[string]string, permissionOption PermissionOption, opts ...browser.CreateTargetOption) (*GoogleMeet, error) {
	gm, err := startMeeting(ctx, cr, br, conn, newMeetingURL, urlParams, permissionOption, opts...)
	if err != nil {
		return gm, err
	}

	return gm, gm.waitUntilInMeeting(ctx)
}

// JoinMeeting joins an existing meeting using given browser.
func JoinMeeting(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, conn *chrome.Conn, meetingCode string, urlParams map[string]string, permissionOption PermissionOption, opts ...browser.CreateTargetOption) (*GoogleMeet, error) {
	gm, err := startMeeting(ctx, cr, br, conn, homePageURL+meetingCode, urlParams, permissionOption, opts...)
	if err != nil {
		return gm, err
	}

	return gm, gm.joinConference(ctx)
}

// JoinMeetingWithEffect selects specific effect and joins an existing meeting using given browser.
func JoinMeetingWithEffect(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, conn *chrome.Conn, meetingURL string, effectOption EffectOption, urlParams map[string]string, permissionOption PermissionOption, opts ...browser.CreateTargetOption) (*GoogleMeet, error) {
	gm, err := startMeeting(ctx, cr, br, conn, meetingURL, urlParams, permissionOption, opts...)
	if err != nil {
		return gm, err
	}
	// If it automatically join meeting, change the effect on the meeting page.
	if gm.ui.Exists(inMeetingIdentifier)(ctx) == nil {
		if err := gm.ApplyVideoEffects(gm.SetEffect(effectOption))(ctx); err != nil {
			return gm, err
		}
	} else {
		if err := gm.SetEffectOnJoinPage(effectOption)(ctx); err != nil {
			return gm, err
		}
	}

	return gm, gm.joinConference(ctx)
}

func startMeeting(ctx context.Context, cr *chrome.Chrome, br *browser.Browser, conn *chrome.Conn, meetingURL string, urlParams map[string]string, permissionOption PermissionOption, opts ...browser.CreateTargetOption) (*GoogleMeet, error) {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, err
	}

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

	if err := conn.Navigate(ctx, meetingURL); err != nil {
		return nil, errors.Wrapf(err, "failed to nevigate to %s", meetingURL)
	}

	gm := New(br, conn, tconn)

	if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
		return nil, errors.Wrapf(err, "failed to wait for %q to be loaded and achieve quiescence", newMeetingURL)
	}

	if err := uiauto.Combine("allow permissions",
		uiauto.Retry(3, gm.ClearPromptsForNewMeeting),
		chromeApps.AllowPagePermissions(gm.tconn),
	)(ctx); err != nil {
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
	ui := gm.ui
	if err := ash.WaitForFullscreenConditionWithTitle(gm.tconn, appName, false, time.Second)(ctx); err != nil {
		testing.ContextLogf(ctx, "%q is already full screen", appName)
		return nil
	}

	return ui.Retry(3, uiauto.NamedCombine("enter full screen",
		// Double-click the first text in meet page to enter full screen.
		ui.DoubleClick(firstText),
		ash.WaitForFullscreenConditionWithTitle(gm.tconn, appName, true, 10*time.Second),
	))(ctx)
}

// ExitFullScreen changes setting to exit full screen mode.
func (gm *GoogleMeet) ExitFullScreen(ctx context.Context) error {
	ui := gm.ui
	if err := ash.WaitForFullscreenConditionWithTitle(gm.tconn, appName, true, time.Second)(ctx); err != nil {
		testing.ContextLogf(ctx, "%q has exited full screen", appName)
		return nil
	}

	return ui.Retry(3, uiauto.NamedCombine("exit full screen",
		// Double-click the first text in meet page to exit full screen.
		ui.DoubleClick(firstText),
		ash.WaitForFullscreenConditionWithTitle(gm.tconn, appName, false, 10*time.Second),
	))(ctx)
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
		info, err := gm.ui.WithTimeout(mediumUITimeout).Info(ctx, microphoneButton)
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
		switchAction := uiauto.NamedAction(actionDesc,
			gm.ui.WithTimeout(mediumUITimeout).DoDefaultUntil(
				microphoneButton,
				gm.ui.WaitUntilGone(microphoneButton)))

		if expectedOn {
			return prompts.ActionAndGrantPermissionIfRequired(
				gm.tconn, gm.conn, switchAction, webutil.PermissionMicrophone)(ctx)
		}

		return switchAction(ctx)
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
		info, err := gm.ui.WithTimeout(mediumUITimeout).Info(ctx, cameraButton)
		if err != nil {
			return errors.Wrap(err, "failed to wait for the camera switch button to show")
		}
		// Skip action if the camera is already as expected.
		if (strings.HasPrefix(info.Name, "Turn on") && !expectedOn) || (strings.HasPrefix(info.Name, "Turn off") && expectedOn) {
			return nil
		}

		cameraButton = nodewith.Name(info.Name).Role(role.Button)
		switchAction := uiauto.NamedAction(actionDesc,
			gm.ui.WithTimeout(mediumUITimeout).DoDefaultUntil(
				cameraButton,
				gm.ui.WaitUntilGone(cameraButton)))

		// Switch on video should check permission status to decide whether need to handle permission prompt.
		if expectedOn {
			return prompts.ActionAndGrantPermissionIfRequired(
				gm.tconn, gm.conn, switchAction, webutil.PermissionCamera)(ctx)
		}

		// Otherwise it is straightforward to switch video.
		return switchAction(ctx)
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
	return gm.ui.WithTimeout(longUITimeout).WaitUntilExists(inMeetingIdentifier)(ctx)
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

// GetParticipantsNum returns the number of meeting participants.
func (gm *GoogleMeet) GetParticipantsNum(ctx context.Context) (int, error) {
	ui := gm.ui
	participantText := nodewith.NameRegex(regexp.MustCompile(`^[\d]+$`)).Role(role.StaticText).Ancestor(meetRootWebArea)
	if err := uiauto.NamedAction("wait for the number of participants to be loaded",
		// Some DUT models have poor performance. When joining
		// a large conference (over 15 participants), it would take much time
		// to render DOM elements. Set a longer timer here.
		ui.WithTimeout(longUITimeout).WaitUntilExists(participantText),
	)(ctx); err != nil {
		return 0, errors.Wrap(err, "failed to wait for participant info")
	}

	node, err := ui.Info(ctx, participantText)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get participant info")
	}

	info := strings.Split(node.Name, " ")
	if len(info) < 1 {
		return 0, errors.New("failed to split node name")
	}

	number, err := strconv.ParseInt(info[0], 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "failed to parse number of participants")
	}

	return int(number), nil
}

// ShareScreen shares screen via "A tab" and select the tab to share.
func (gm *GoogleMeet) ShareScreen(tabName string) action.Action {
	ui := gm.ui

	clickPresentNowButton := func(ctx context.Context) error {
		presentNowButton := nodewith.Name("Present now").Role(role.Button)
		presentNowPopUpButton := nodewith.Name("Present now").Role(role.PopUpButton)
		presentingPopUpButton := nodewith.NameContaining("presenting").Role(role.PopUpButton)
		presentButton, err := ui.FindAnyExists(ctx,
			presentNowButton,
			presentNowPopUpButton,
			presentingPopUpButton)
		if err != nil {
			return errors.New("failed to find present button")
		}
		if presentButton == presentingPopUpButton {
			testing.ContextLog(ctx, "Another participant is presenting now")
			return errors.New("another participant is presenting now")
		}

		return ui.DoDefault(presentButton)(ctx)
	}

	presentMode := nodewith.NameContaining("A tab").Role(role.MenuItem)
	presentTab := nodewith.ClassName("AXVirtualView").Role(role.Cell).NameContaining(tabName)
	shareButton := nodewith.Name("Share").Role(role.Button)

	return ui.Retry(3, uiauto.NamedCombine("share screen",
		clickPresentNowButton,
		ui.WaitUntilAnyExists(presentMode, presentTab),
		uiauto.IfSuccessThen(ui.Exists(presentMode), ui.DoDefault(presentMode)),
		ui.LeftClickUntil(presentTab, ui.WithTimeout(shortUITimeout).WaitUntilExists(presentTab.Focused())),
		ui.LeftClickUntil(shareButton, ui.WithTimeout(shortUITimeout).WaitUntilGone(shareButton)),
		gm.closeShareScreenAlertDialog(),
		ui.WithTimeout(longUITimeout).WaitUntilExists(stopSharing),
	))
}

// StopShareScreen stops sharing screen.
func (gm *GoogleMeet) StopShareScreen() action.Action {
	ui := gm.ui

	return uiauto.NamedCombine("stop sharing screen",
		gm.closeShareScreenAlertDialog(),
		ui.WithTimeout(mediumUITimeout).DoDefaultUntil(stopSharing, ui.WaitUntilGone(stopSharing)),
	)
}

// closeShareScreenAlertDialog closes the alert dialog when sharing the screen.
func (gm *GoogleMeet) closeShareScreenAlertDialog() action.Action {
	ui := gm.ui
	alertDialog := nodewith.Name("Your screen is still visible to others").Role(role.Alert)
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(alertDialog)

	return uiauto.IfSuccessThen(
		ui.WithTimeout(shortUITimeout).WaitUntilExists(closeButton),
		uiauto.NamedAction("close alert dialog", ui.LeftClick(closeButton)))
}

// TypingInChat opens chat panel and types message.
func (gm *GoogleMeet) TypingInChat(kb *input.KeyboardEventWriter, message string) action.Action {
	const retryTimes = 3
	ui := gm.ui
	chatText := nodewith.NameContaining("Send a message")
	// There may be multiple "Send a message" fields, so add First() here.
	chatTextField := chatText.Role(role.TextField).First()
	openChatPanel := uiauto.NamedCombine("open chat panel",
		ui.LeftClick(firstText),
		ui.DoDefault(chatButton),
		// Some low end DUTs need very long time to load chat window in 49 tiles.
		ui.WithTimeout(2*time.Minute).WaitUntilExists(chatTextField.Focusable()),
	)

	// There may be multiple "Send a message" buttons, so add First() here.
	chatTextButton := chatText.Role(role.Button).First()
	// There may be multiple message texts, so add First() here.
	messageRe := regexp.MustCompile("(?i)" + message)
	messageText := nodewith.NameRegex(messageRe).Role(role.StaticText).First()
	enterText := uiauto.NamedCombine("type message",
		ui.WithTimeout(longUITimeout).DoDefaultUntil(chatTextButton,
			ui.WaitUntilExists(chatTextField.Editable().Focused())),
		kb.TypeAction(message),
		ui.WaitUntilExists(messageText),
		kb.AccelAction("enter"),
		ui.WithTimeout(longUITimeout).WaitUntilExists(messageText),
	)

	return ui.Retry(retryTimes, uiauto.Combine("open chat panel and type message",
		ui.Retry(retryTimes, uiauto.IfSuccessThen(ui.Gone(chatPanelHeading), openChatPanel)),
		ui.Retry(retryTimes, enterText)))
}

// CloseChatPanel closes chat panel.
func (gm *GoogleMeet) CloseChatPanel() action.Action {
	ui := gm.ui

	return uiauto.IfSuccessThen(ui.Exists(chatPanelHeading),
		uiauto.NamedCombine("close chat panel",
			ui.DoDefault(chatButton),
			ui.WithTimeout(longUITimeout).WaitUntilGone(chatPanelHeading),
		))
}
