// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/apps/zoom"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
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
	zm                         *zoom.Zoom
	uiHandler                  cuj.UIActionHandler
	displayAllParticipantsTime time.Duration
	tabletMode                 bool
	roomType                   RoomType
	networkLostCount           int
	account                    string
	outDir                     string
}

const zoomTitle = "Zoom"

// Join joins a new conference room.
func (conf *ZoomConference) Join(ctx context.Context, room string, toBlur bool) (err error) {
	ui := conf.ui
	conf.zm, err = zoom.JoinMeeting(ctx, conf.cr, conf.br, room)
	if err != nil {
		return errors.Wrap(err, "failed to join zoom meeting")
	}
	expectedParticipants := ZoomRoomParticipants[conf.roomType]

	return uiauto.Combine("wait participants and join audio",
		conf.zm.WaitParticipantsNum(expectedParticipants),
		ui.Retry(retryTimes, conf.zm.SetJoinAudio(true)),
	)(ctx)
}

// GetParticipants returns the number of meeting participants.
func (conf *ZoomConference) GetParticipants(ctx context.Context) (int, error) {
	return conf.zm.GetParticipantsNum(ctx)
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
func (conf *ZoomConference) SwitchTabs(ctx context.Context) error {
	testing.ContextLog(ctx, "Open wiki page")
	// Set newWindow to false to make the tab in the same Chrome window.
	wikiConn, err := conf.uiHandler.NewChromeTab(ctx, conf.br, cuj.WikipediaURL, false)
	if err != nil {
		return errors.Wrap(err, "failed to open the wiki url")
	}
	defer wikiConn.Close()

	if err := webutil.WaitForQuiescence(ctx, wikiConn, longUITimeout); err != nil {
		return errors.Wrap(err, "failed to wait for wiki page to finish loading")
	}
	return uiauto.Combine("switch tab",
		uiauto.NamedAction("stay wiki page for 3 seconds", uiauto.Sleep(3*time.Second)),
		uiauto.NamedAction("switch to zoom tab", conf.uiHandler.SwitchToChromeTabByName(zoomTitle)),
	)(ctx)
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
				takeScreenshot(conf.cr, conf.outDir, fmt.Sprintf("change-background-to-%q", string(backgroundOption))),
			),
			conf.zm.EnterFullScreen,
			// After applying new background, give it 5 seconds for viewing before applying next one.
			uiauto.Sleep(viewingTime),
			conf.zm.ExitFullScreen,
		)
	}
	if err := uiauto.Combine("background change",
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

// End ends the conference.
func (conf *ZoomConference) End(ctx context.Context) error {
	if err := conf.zm.Close(ctx); err != nil {
		return err
	}
	return cuj.CloseAllWindows(ctx, conf.tconn)
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
