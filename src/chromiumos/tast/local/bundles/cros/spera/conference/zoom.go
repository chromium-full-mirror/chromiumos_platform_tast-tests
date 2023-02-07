// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"fmt"
	"strconv"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps/zoom"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
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
		conf.zm.ChangeLayout(zoom.GalleryView),
		// After applying new layout, give it 5 seconds for viewing before applying next one.
		uiauto.Sleep(viewingTime),
	)(ctx)
}

// SetLayoutMin sets the conference UI layout to minimal tiled grid.
func (conf *ZoomConference) SetLayoutMin(ctx context.Context) error {
	return uiauto.Combine("set layout to minimal",
		conf.zm.ChangeLayout(zoom.SpeakerView),
		// After applying new layout, give it 5 seconds for viewing before applying next one.
		uiauto.Sleep(viewingTime),
	)(ctx)
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

// TypingInChat opens chat window, type in chat room and close chat window.
func (conf *ZoomConference) TypingInChat(ctx context.Context) error {
	const message = "Hello! How are you?"
	return uiauto.Combine("typing in chat",
		conf.zm.TypingInChat(conf.kb, message),
		uiauto.Sleep(viewingTime), // After typing, wait 5 seconds for viewing.
		conf.zm.CloseChatPanel(),
	)(ctx)
}

// BackgroundChange changes the background to patterned background and reset to none.
//
// Zoom doesn't have background blur option for web version so changing background is used to fullfil
// the requirement.
func (conf *ZoomConference) BackgroundChange(ctx context.Context) error {
	changeBackgroundAndToggleFullScreen := func(backgroundOption zoom.BackgroundOption) action.Action {
		return uiauto.Combine("change background and toggle full screen",
			conf.zm.ChangeSettings(
				conf.zm.ChooseBackground(backgroundOption),
				takeScreenshot(conf.cr, conf.outDir, fmt.Sprintf("change-background-to-%q", backgroundOption)),
			),
			conf.zm.EnterFullScreen,
			// After applying new background, give it 5 seconds for viewing before applying next one.
			uiauto.Sleep(viewingTime),
			conf.zm.ExitFullScreen,
		)
	}
	if err := uiauto.Combine("change background",
		conf.uiHandler.SwitchToChromeTabByName(zoomTitle),
		changeBackgroundAndToggleFullScreen(zoom.StaticBackground),
		changeBackgroundAndToggleFullScreen(zoom.NoBackground),
	)(ctx); err != nil {
		return CheckSignedOutError(ctx, conf.tconn, err)
	}
	return nil
}

// Presenting creates Google Slides and Google Docs, shares screen and presents
// the specified application to the conference.
func (conf *ZoomConference) Presenting(ctx context.Context, application googleApplication) (err error) {
	appName := string(application)
	shareScreen := uiauto.Combine("share screen",
		conf.uiHandler.SwitchToChromeTabByName(zoomTitle),
		conf.zm.ShareScreen(appName))
	stopPresenting := conf.zm.StopShareScreen()

	// Present on internal display by default.
	presentOnExtendedDisplay := false
	if err := presentApps(ctx, conf.tconn, conf.uiHandler, conf.cr, conf.br, shareScreen, stopPresenting,
		application, conf.outDir, presentOnExtendedDisplay); err != nil {
		return errors.Wrapf(err, "failed to present %s", appName)
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
