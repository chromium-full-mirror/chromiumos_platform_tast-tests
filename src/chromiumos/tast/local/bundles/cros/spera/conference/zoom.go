// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"fmt"
	"regexp"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps/zoom"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/mouse"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/webutil"
	"chromiumos/tast/local/input"
	"chromiumos/tast/testing"
)

// ZoomConference implements the Conference interface.
type ZoomConference struct {
	cr                         *chrome.Chrome
	br                         *browser.Browser
	tconn                      *chrome.TestConn
	kb                         *input.KeyboardEventWriter
	ui                         *uiauto.Context
	uiHandler                  cuj.UIActionHandler
	zoomConn                   *chrome.Conn
	zm                         *zoom.Zoom
	displayAllParticipantsTime time.Duration
	tabletMode                 bool
	roomType                   RoomType
	networkLostCount           int
	account                    string
	outDir                     string
}

const zoomTitle = "Zoom"

var zoomWebArea = nodewith.NameContaining("Zoom Meeting").Role(role.RootWebArea)

// Join joins a new conference room.
func (conf *ZoomConference) Join(ctx context.Context, room string) (err error) {
	ui := conf.ui

	conf.zm, err = zoom.JoinMeeting(ctx, conf.cr, conf.br, room)
	if err != nil {
		return errors.Wrap(err, "failed to join zoom meeting")
	}

	// Checks the number of participants in the conference that
	// for different tiers testing would ask for different size
	checkParticipantsNum := func(ctx context.Context) error {
		expectedParticipants := ZoomRoomParticipants[conf.roomType]
		participants, err := conf.GetParticipants(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get the the number of meeting participants")
		}
		if int(participants) != expectedParticipants {
			return errors.Wrapf(err, "meeting participant number is %d but %d is expected", participants, expectedParticipants)
		}
		testing.ContextLog(ctx, "Current participants: ", participants)
		return nil
	}
	return uiauto.Combine("check participants and join audio",
		// Sometimes participants number caught at the beginning is wrong, it will be correct after a while.
		// Add retry to get the correct participants number.
		ui.WithInterval(time.Second).Retry(10, checkParticipantsNum),
		ui.Retry(retryTimes, conf.zm.SetJoinAudio(true)),
	)(ctx)
}

// GetParticipants returns the number of meeting participants.
func (conf *ZoomConference) GetParticipants(ctx context.Context) (int, error) {
	ui := conf.ui

	participant := nodewith.NameContaining("open the participants list pane").Role(role.Button)
	noParticipant := nodewith.NameContaining("[0] particpants").Role(role.Button)
	if err := uiauto.NamedCombine("wait participants",
		ui.WaitUntilExists(participant),
		ui.WithTimeout(mediumUITimeout).WaitUntilGone(noParticipant),
	)(ctx); err != nil {
		return 0, errors.Wrap(err, "failed to wait participant info")
	}

	node, err := ui.Info(ctx, participant)
	if err != nil {
		return 0, errors.Wrap(err, "failed to get participant info")
	}
	testing.ContextLog(ctx, "Get participant info: ", node.Name)
	info := strings.Split(node.Name, "[")
	info = strings.Split(info[1], "]")
	participants, err := strconv.ParseInt(info[0], 10, 64)
	if err != nil {
		return 0, errors.Wrap(err, "cannot parse number of participants")
	}

	return int(participants), nil
}

// SetLayoutMax sets the conference UI layout to max tiled grid.
func (conf *ZoomConference) SetLayoutMax(ctx context.Context) error {
	return uiauto.Combine("set layout to max",
		conf.changeLayout("Gallery View"),
		uiauto.Sleep(viewingTime), // After applying new layout, give it 5 seconds for viewing before applying next one.
	)(ctx)
}

// SetLayoutMin sets the conference UI layout to minimal tiled grid.
func (conf *ZoomConference) SetLayoutMin(ctx context.Context) error {
	return uiauto.Combine("set layout to minimal",
		conf.changeLayout("Speaker View"),
		uiauto.Sleep(viewingTime), // After applying new layout, give it 5 seconds for viewing before applying next one.
	)(ctx)
}

// changeLayout changes the conference UI layout.
func (conf *ZoomConference) changeLayout(mode string) action.Action {
	return func(ctx context.Context) error {
		ui := conf.ui
		viewButton := nodewith.Name("View").Role(role.Button)
		viewMenu := nodewith.Role(role.Menu).HasClass("dropdown-menu")
		speakerNode := nodewith.Name("Speaker View").Role(role.MenuItem)
		// Sometimes the zoom's menu disappears too fast. Add retry to check whether the device supports
		// speaker and gallery view.
		if err := uiauto.Combine("check view button",
			conf.showInterface,
			ui.LeftClickUntil(viewButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(viewMenu)),
			ui.WithTimeout(shortUITimeout).WaitUntilExists(speakerNode),
		)(ctx); err != nil {
			// Some DUTs don't support 'Speacker View' and 'Gallery View'.
			testing.ContextLog(ctx, "Speaker and Gallery View is not supported on this device, ignore changing the layout")
			return nil
		}

		modeNode := nodewith.Name(mode).Role(role.MenuItem)
		actionName := "Change layout to " + mode
		return ui.Retry(retryTimes, uiauto.NamedCombine(actionName,
			conf.showInterface,
			uiauto.IfSuccessThen(ui.Gone(modeNode), ui.LeftClick(viewButton)),
			ui.LeftClick(modeNode),
		))(ctx)
	}
}

// VideoAudioControl controls the video and audio during conference.
func (conf *ZoomConference) VideoAudioControl(ctx context.Context) error {
	return uiauto.Combine("switch video and audio",
		// Remain in the state for 5 seconds after each action.
		conf.zm.SwitchVideo(false), uiauto.Sleep(viewingTime),
		conf.zm.SwitchVideo(true), uiauto.Sleep(viewingTime),
		conf.zm.SwitchAudio(false), uiauto.Sleep(viewingTime),
		conf.zm.SwitchAudio(true), uiauto.Sleep(viewingTime),
	)(ctx)
}

// SwitchTabs switches the chrome tabs.
func (conf *ZoomConference) SwitchTabs(url string) action.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Open URL: ", url)
		// Set newWindow to false to make the tab in the same Chrome window.
		conn, err := conf.uiHandler.NewChromeTab(ctx, conf.br, url, false)
		if err != nil {
			return errors.Wrapf(err, "failed to open the URL %s", url)
		}
		defer conn.Close()

		if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
			return errors.Wrap(err, "failed to wait for the page to finish loading")
		}
		return uiauto.Combine("switch tab",
			uiauto.NamedAction("stay page for 3 seconds", uiauto.Sleep(3*time.Second)),
			uiauto.NamedAction("switch to zoom tab", conf.uiHandler.SwitchToChromeTabByName(zoomTitle)),
		)(ctx)
	}
}

// TypingInChat opens chat window and type.
func (conf *ZoomConference) TypingInChat(ctx context.Context) error {
	const message = "Hello! How are you?"
	// Close all notifications to prevent them from covering the chat text field.
	if err := ash.CloseNotifications(ctx, conf.tconn); err != nil {
		return errors.Wrap(err, "failed to close otifications")
	}
	chatButton := nodewith.Name("open the chat pane").Role(role.Button)
	chatTextRe := regexp.MustCompile("(Type message here ...|chat message)")
	chatTextField := nodewith.NameRegex(chatTextRe).Role(role.TextField)
	messageText := nodewith.Name(message).Role(role.StaticText).First()
	manageChatPanel := nodewith.Name("Manage Chat Panel").Role(role.PopUpButton)
	manageChatPanelMenu := nodewith.Name("Manage Chat Panel").Role(role.Menu)
	closeButton := nodewith.Name("Close").Role(role.MenuItem).Ancestor(manageChatPanelMenu)
	typeMessage := uiauto.NamedCombine("type message : "+message,
		conf.ui.LeftClickUntil(chatTextField, conf.ui.WithTimeout(shortUITimeout).WaitUntilExists(chatTextField.Focused())),
		conf.kb.AccelAction("Ctrl+A"),
		conf.kb.TypeAction(message),
		conf.kb.AccelAction("enter"),
		conf.ui.WaitUntilExists(messageText))
	return uiauto.NamedCombine("open chat window and type",
		conf.ui.DoDefault(chatButton),
		conf.ui.WaitUntilExists(chatTextField),
		conf.ui.Retry(retryTimes, typeMessage),
		uiauto.Sleep(viewingTime), // After typing, wait 5 seconds for viewing.
		conf.ui.LeftClick(manageChatPanel),
		conf.ui.LeftClick(closeButton),
	)(ctx)
}

// BackgroundChange changes the background to patterned background and reset to none.
//
// Zoom doesn't have background blur option for web version so changing background is used to fullfil
// the requirement.
func (conf *ZoomConference) BackgroundChange(ctx context.Context) error {
	const (
		noneBackground   = "None"
		staticBackground = "San Francisco.jpg"
	)
	ui := conf.ui
	changeBackground := func(backgroundOption string) error {
		settingsButton := nodewith.Name("Settings").Role(role.Button).Ancestor(zoomWebArea)
		settingsWindow := nodewith.Name("settings dialog window").Role(role.Application).Ancestor(zoomWebArea)
		backgroundTab := nodewith.Name("Background").Role(role.Tab).Ancestor(settingsWindow)
		backgroundItem := nodewith.NameContaining(backgroundOption).Role(role.ListBoxOption).Ancestor(settingsWindow)
		closeButton := nodewith.Role(role.Button).HasClass("settings-dialog__close").Ancestor(settingsWindow)
		openBackgroundPanel := func(ctx context.Context) error {
			var actions []action.Action
			if err := conf.showInterface(ctx); err != nil {
				return err
			}
			if err := ui.Exists(settingsButton)(ctx); err == nil {
				actions = append(actions,
					uiauto.NamedAction("click settings button",
						ui.WithTimeout(longUITimeout).DoDefaultUntil(settingsButton, ui.WaitUntilExists(backgroundTab)),
					))
			} else {
				// If the screen width is not enough, the settings button will be moved to more options.
				moreOptions := nodewith.Name("More meeting control").Ancestor(zoomWebArea)
				moreSettingsButton := nodewith.Name("Settings").Role(role.MenuItem).Ancestor(zoomWebArea)
				actions = append(actions,
					uiauto.NamedAction("click more option", ui.LeftClick(moreOptions)),
					uiauto.NamedAction("click settings menu item", ui.LeftClick(moreSettingsButton)),
				)
			}
			actions = append(actions, ui.LeftClick(backgroundTab))
			if err := uiauto.Combine("open background panel", actions...)(ctx); err != nil {
				return errors.Wrap(err, "failed to background panel")
			}
			return nil
		}
		return uiauto.NamedCombine("change background to "+backgroundOption,
			ui.Retry(retryTimes, openBackgroundPanel), // Open "Background" panel.
			// Some low end DUTs need more time to load the background settings.
			ui.WithTimeout(longUITimeout).DoDefaultUntil(backgroundItem,
				ui.WithTimeout(shortUITimeout).WaitUntilExists(backgroundItem.Focused())),
			// After applying the new background, give it 3 seconds to load the new background before closing the settings.
			uiauto.Sleep(shortUITimeout),
			ui.LeftClick(closeButton), // Close "Background" panel.
			takeScreenshot(conf.cr, conf.outDir, fmt.Sprintf("change-background-to-background-%q", backgroundOption)),
			// Double click to enter full screen.
			doFullScreenAction(conf.tconn, ui.DoubleClick(zoomWebArea), zoomTitle, true),
			// After applying new background, give it 5 seconds for viewing before applying next one.
			uiauto.Sleep(viewingTime),
			// Double click to exit full screen.
			doFullScreenAction(conf.tconn, ui.DoubleClick(zoomWebArea), zoomTitle, false),
		)(ctx)
	}
	if err := conf.uiHandler.SwitchToChromeTabByName(zoomTitle)(ctx); err != nil {
		return CheckSignedOutError(ctx, conf.tconn, errors.Wrap(err, "failed to switch to zoom page"))
	}
	if err := changeBackground(staticBackground); err != nil {
		return errors.Wrap(err, "failed to change background to static background")
	}
	if err := changeBackground(noneBackground); err != nil {
		return errors.Wrap(err, "failed to change background to none")
	}
	return nil
}

// Presenting creates Google Slides and Google Docs, shares screen and presents
// the specified application to the conference.
func (conf *ZoomConference) Presenting(ctx context.Context, application googleApplication) (err error) {
	tconn := conf.tconn
	ui := uiauto.New(tconn)

	var appTabName string
	switch application {
	case googleSlides:
		appTabName = slideTabName
	case googleDocs:
		appTabName = docTabName
	}
	// shareScreen shares screen by "Chrome Tab" and selects the tab which is going to present.
	shareScreen := func(ctx context.Context) error {
		shareScreenButton := nodewith.Name("Share Screen").Role(role.StaticText)
		presenMode := nodewith.Name("Chrome Tab").Role(role.Tab)
		presentTab := nodewith.ClassName("AXVirtualView").Role(role.Cell).Name(appTabName)
		shareButton := nodewith.Name("Share").Role(role.Button)
		stopSharing := nodewith.Name("Stop sharing").Role(role.Button).First()
		return uiauto.NamedCombine("share Screen",
			conf.uiHandler.SwitchToChromeTabByName(zoomTitle),
			conf.showInterface,
			ui.LeftClickUntil(shareScreenButton, ui.WithTimeout(shortUITimeout).WaitUntilExists(presenMode)),
			ui.LeftClick(presenMode),
			ui.LeftClick(presentTab),
			ui.LeftClickUntil(shareButton, ui.WithTimeout(shortUITimeout).WaitUntilGone(shareButton)),
			ui.WithTimeout(mediumUITimeout).WaitUntilExists(stopSharing),
		)(ctx)
	}

	stopPresenting := func(ctx context.Context) error {
		stopSharing := nodewith.Name("Stop sharing").Role(role.Button).First()
		return ui.LeftClickUntil(stopSharing, ui.WithTimeout(shortUITimeout).WaitUntilGone(stopSharing))(ctx)
	}
	// Present on internal display by default.
	presentOnExtendedDisplay := false
	if err := presentApps(ctx, tconn, conf.uiHandler, conf.cr, conf.br, shareScreen, stopPresenting,
		application, conf.outDir, presentOnExtendedDisplay); err != nil {
		return errors.Wrapf(err, "failed to present %s", string(application))
	}
	return nil
}

// End closes all windows in the end.
func (conf *ZoomConference) End(ctx context.Context) error {
	return cuj.CloseAllWindows(ctx, conf.tconn)
}

// CloseConference closes the conference.
func (conf *ZoomConference) CloseConference(ctx context.Context) error {
	return conf.zm.Close(ctx)
}

var _ Conference = (*ZoomConference)(nil)

// showInterface moves mouse or taps in web area in order to make the menu interface reappear.
func (conf *ZoomConference) showInterface(ctx context.Context) error {
	ui := conf.ui
	information := nodewith.Name("Meeting information").Role(role.Button).Ancestor(zoomWebArea)

	return testing.Poll(ctx, func(ctx context.Context) error {
		if err := ui.Exists(information)(ctx); err == nil {
			return nil
		}

		if conf.tabletMode {
			testing.ContextLog(ctx, "Tap web area to show interface")
			if err := conf.uiHandler.Click(zoomWebArea)(ctx); err != nil {
				return errors.Wrap(err, "failed to click the web area")
			}
		} else {
			testing.ContextLog(ctx, "Mouse move to show interface")
			webAreaInfo, err := ui.Info(ctx, zoomWebArea)
			if err != nil {
				return err
			}
			if err := mouse.Move(conf.tconn, webAreaInfo.Location.TopLeft(), 200*time.Millisecond)(ctx); err != nil {
				return errors.Wrap(err, "failed to move mouse to top left corner of the web area")
			}
			if err := ui.MouseMoveTo(zoomWebArea, 200*time.Millisecond)(ctx); err != nil {
				return errors.Wrap(err, "failed to move mouse to the center of the web area")
			}
		}

		if err := ui.WaitUntilExists(information)(ctx); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: mediumUITimeout})
}

// SetBrowser sets browser to chrome or lacros.
func (conf *ZoomConference) SetBrowser(br *browser.Browser) {
	conf.br = br
}

// LostNetworkCount returns the count of lost network connections.
func (conf *ZoomConference) LostNetworkCount() int {
	return conf.networkLostCount
}

// DisplayAllParticipantsTime returns the loading time for displaying all participants.
func (conf *ZoomConference) DisplayAllParticipantsTime() time.Duration {
	return conf.displayAllParticipantsTime
}

// NewZoomConference creates Zoom conference room instance which implements Conference interface.
func NewZoomConference(cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter,
	uiHandler cuj.UIActionHandler, tabletMode bool, roomType RoomType, account, outDir string) *ZoomConference {
	ui := uiauto.New(tconn)
	return &ZoomConference{
		cr:         cr,
		tconn:      tconn,
		kb:         kb,
		ui:         ui,
		uiHandler:  uiHandler,
		tabletMode: tabletMode,
		roomType:   roomType,
		account:    account,
		outDir:     outDir,
	}
}
