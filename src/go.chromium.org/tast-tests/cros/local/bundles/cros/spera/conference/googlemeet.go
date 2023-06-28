// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package conference

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/googlemeet"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/cuj"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
	"go.chromium.org/tast-tests/cros/local/input"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// GoogleMeetConference implements the Conference interface.
type GoogleMeetConference struct {
	cr                         *chrome.Chrome
	br                         *browser.Browser
	tconn                      *chrome.TestConn
	kb                         *input.KeyboardEventWriter
	ui                         *uiauto.Context
	uiHandler                  cuj.UIActionHandler
	meetConn                   *chrome.Conn
	displayAllParticipantsTime time.Duration
	bt                         browser.Type
	roomType                   RoomType
	gm                         *googlemeet.GoogleMeet
	outDir                     string
	tabletMode                 bool
	networkLostCount           int
	participantsNumber         int
}

var _ Conference = (*GoogleMeetConference)(nil)

// NewGoogleMeetConference creates Google Meet conference room instance which implements Conference interface.
func NewGoogleMeetConference(cr *chrome.Chrome, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, uiHandler cuj.UIActionHandler,
	bt browser.Type, roomType RoomType, outDir string, tabletMode bool) *GoogleMeetConference {
	ui := uiauto.New(tconn)
	return &GoogleMeetConference{
		cr:         cr,
		tconn:      tconn,
		kb:         kb,
		ui:         ui,
		uiHandler:  uiHandler,
		bt:         bt,
		roomType:   roomType,
		tabletMode: tabletMode,
		outDir:     outDir,
	}
}

const (
	meetTitle  = "Meet"
	retryTimes = 3
)

var meetWebArea = nodewith.NameContaining(meetTitle).Role(role.RootWebArea)

// Join joins a new conference room.
func (conf *GoogleMeetConference) Join(ctx context.Context, room string) (err error) {
	cr, br, tconn, ui := conf.cr, conf.br, conf.tconn, conf.ui

	conn, err := br.NewTab(ctx, "")
	if err != nil {
		return errors.Wrap(err, "failed to create new tab")
	}

	conf.gm, err = googlemeet.JoinMeetingWithEffect(ctx, cr, br, conn, room, googlemeet.BlurEffect, nil, googlemeet.WithAllPermissions, browser.WithNewWindow())
	if err != nil {
		return CheckSignedOutError(ctx, tconn, errors.Wrap(err, "failed to join google meeting"))
	}

	// checkParticipantsNum checks the number of meeting participants.
	checkParticipantsNum := func(ctx context.Context) error {
		// Each room type has a different number of participants:
		// - Class size room: >= 35 participants
		// - Grid size room: 16 ~ 17 participants
		expectedNumber := GoogleMeetRoomParticipants[conf.roomType]
		number, err := conf.gm.GetParticipantsNum(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get the the number of meeting participants")
		}
		if number == 1 {
			return ParticipantError(errors.Wrapf(err, "there are no other participants in the meeting room; got %d; want %d", number, expectedNumber))
		}
		switch conf.roomType {
		case ClassRoomSize:
			if number < expectedNumber {
				return ParticipantError(errors.Wrapf(err, "the number of participants got %d; want at least %d", number, expectedNumber))
			}
		case GridRoomSize:
			if number != expectedNumber && number != expectedNumber+1 {
				return ParticipantError(errors.Wrapf(err, "the number of participants got %d; want %d ~ %d", number, expectedNumber, expectedNumber+1))
			}
		}
		testing.ContextLog(ctx, "Current participants number: ", number)
		conf.participantsNumber = number

		return nil
	}

	// Sometimes participants number caught at the beginning is wrong, it will be correct after a while.
	// Add retry to get the correct participants number.
	return ui.WithInterval(time.Second).Retry(5, checkParticipantsNum)(ctx)
}

// GetParticipants returns the number of meeting participants.
// If the participants already has a value, return the value directly.
func (conf *GoogleMeetConference) GetParticipants(ctx context.Context) (int, error) {
	if conf.participantsNumber != 0 {
		return conf.participantsNumber, nil
	}

	return conf.gm.GetParticipantsNum(ctx)
}

// VideoAudioControl controls the video and audio during conference.
func (conf *GoogleMeetConference) VideoAudioControl(ctx context.Context) error {
	gm := conf.gm

	return uiauto.Combine("switch video and audio",
		// Remain in the state for 5 seconds after each action.
		gm.SwitchVideo(false), uiauto.Sleep(viewingTime),
		gm.SwitchVideo(true), uiauto.Sleep(viewingTime),
		gm.SwitchMicrophone(true), uiauto.Sleep(viewingTime),
		gm.SwitchMicrophone(false), uiauto.Sleep(viewingTime),
	)(ctx)
}

// SwitchTabs switches the chrome tabs.
func (conf *GoogleMeetConference) SwitchTabs(url string) action.Action {
	return func(ctx context.Context) error {
		testing.ContextLog(ctx, "Open URL: ", url)
		// Set newWindow to false to make the tab in the same Chrome window.
		conn, err := conf.uiHandler.NewChromeTab(ctx, conf.br, url, false)
		if err != nil {
			return errors.Wrapf(err, "failed to open the url %s", url)
		}
		defer conn.Close()
		defer conn.CloseTarget(ctx)
		if err := webutil.WaitForQuiescence(ctx, conn, longUITimeout); err != nil {
			return errors.Wrap(err, "failed to wait for the page to finish loading")
		}
		return uiauto.Combine("switch tab",
			uiauto.NamedAction("stay page for 3 seconds", uiauto.Sleep(3*time.Second)),
			uiauto.NamedAction("switch to meet tab", conf.uiHandler.SwitchToChromeTabByName(meetTitle)),
			conf.checkLostNetwork,
		)(ctx)
	}
}

// TypingInChat opens chat window and type message.
func (conf *GoogleMeetConference) TypingInChat(ctx context.Context) error {
	const message = "Hello! How are you?"

	return uiauto.Combine("typing in chat",
		conf.gm.TypingInChat(conf.kb, message),
		uiauto.Sleep(viewingTime), // After typing, wait 5 seconds for viewing.
		conf.gm.CloseChatPanel(),
	)(ctx)
}

// SetLayoutMax sets the conference UI layout to max tiled grid.
func (conf *GoogleMeetConference) SetLayoutMax(ctx context.Context) error {
	return uiauto.Combine("set layout to max",
		conf.changeLayout(googlemeet.TiledLayout),
		// After applying new layout, give it 5 seconds for viewing before applying next one.
		uiauto.Sleep(viewingTime),
	)(ctx)
}

// SetLayoutMin sets the conference UI layout to minimal tiled grid.
func (conf *GoogleMeetConference) SetLayoutMin(ctx context.Context) error {
	return uiauto.Combine("set layout to minimal",
		conf.changeLayout(googlemeet.SpotlightLayout),
		// After applying new layout, give it 5 seconds for viewing before applying next one.
		uiauto.Sleep(viewingTime),
	)(ctx)
}

// getGrids returns the current tiled grids.
func (conf *GoogleMeetConference) getGrids(ctx context.Context) (grids []uiauto.NodeInfo, err error) {
	grid := nodewith.Role(role.Video).Ancestor(meetWebArea)
	grids, err = conf.ui.NodesInfo(ctx, grid)
	if err != nil {
		return grids, errors.Wrap(err, "failed to find grids")
	}
	return grids, nil
}

// getStableGrids returns stable tiled grids that take time to load.
// It calculates the displayAllParticipantsTime when the grid number doesn't change in a 5-second interval.
// The grids are not necessarily playing videos.
func (conf *GoogleMeetConference) getStableGrids(ctx context.Context) (grids []uiauto.NodeInfo, err error) {
	var loadingTime time.Duration
	lastQuantity := -1
	count := 0
	testing.ContextLog(ctx, "Wait for grids loading to stabilize")
	startTime := time.Now()
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		grids, err = conf.getGrids(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to find grids")
		}
		currentQuantity := len(grids)
		// Sometimes the number of grids may be unstable, especially on low end DUTs.
		// It may reduce 1~2 grids when exchanging participants videos.
		if lastQuantity-2 <= currentQuantity && currentQuantity <= lastQuantity+2 {
			count++
		} else {
			lastQuantity = currentQuantity
			loadingTime = time.Now().Sub(startTime)
			count = 0
		}
		if count > 5 {
			testing.ContextLogf(ctx, "There are currently %v grids displayed in %v", currentQuantity, loadingTime)
			return nil
		}
		return errors.New("Grids are still loading now")
	}, &testing.PollOptions{Interval: time.Second, Timeout: longUITimeout}); err != nil {
		return grids, err
	}
	conf.displayAllParticipantsTime = loadingTime
	return grids, nil
}

// changeLayout changes the conference UI layout.
func (conf *GoogleMeetConference) changeLayout(layoutOption googlemeet.LayoutOption) action.Action {
	return func(ctx context.Context) error {
		ui := conf.ui
		gm := conf.gm

		// Close all notifications to prevent them from covering the print button.
		if err := ash.CloseNotifications(ctx, conf.tconn); err != nil {
			return errors.Wrap(err, "failed to close notifications")
		}

		setToMaxTiles := func(layoutOption googlemeet.LayoutOption) action.Action {
			return func(ctx context.Context) error {
				if layoutOption != googlemeet.TiledLayout {
					return nil
				}
				slider := nodewith.Name("Tiles").Role(role.Slider).First()
				raiseTiles := conf.kb.AccelAction("Right")
				isMaxTiles := func(ctx context.Context) error {
					// "49 tiles" is the maximum number of layout tiles.
					// It has nothing to do with the number of participants.
					const expectedResult = "49 tiles"
					sliderInfo, err := ui.Info(ctx, slider)
					if err != nil {
						return errors.Wrap(err, "failed to get slider info")
					}
					value := sliderInfo.Value
					if value == expectedResult {
						testing.ContextLogf(ctx, "Get the expected tiles: got %q", value)
						return nil
					}
					testing.ContextLogf(ctx, "Tiles info: got %q; want %q", value, expectedResult)
					return errors.Errorf("wrong tiles: got %q; want %q", value, expectedResult)
				}

				return ui.Retry(retryTimes, uiauto.NamedCombine("set to max tiles",
					ui.LeftClick(slider),
					// Some Duts don't capture the correct tile number.
					// Turn down the number of tiles and try again.
					uiauto.IfFailThen(
						ui.WithInterval(500*time.Millisecond).RetryUntil(raiseTiles, isMaxTiles),
						conf.kb.AccelAction("Left"))))(ctx)
			}
		}

		checkLayoutChanged := func(ctx context.Context) error {
			return testing.Poll(ctx, func(ctx context.Context) error {
				grids, err := conf.getGrids(ctx)
				if err != nil {
					return errors.Wrap(err, "failed to find grids")
				}
				// Spotlight layout is expected to have at most 1~2 grids.
				if layoutOption == googlemeet.SpotlightLayout && len(grids) <= 2 {
					return nil
				}
				// Tiled layout expects more than 2 grids.
				if layoutOption == googlemeet.TiledLayout && len(grids) > 2 {
					return nil
				}
				return errors.New("Grids are still loading now")
			}, &testing.PollOptions{Interval: time.Second, Timeout: 10 * time.Second})
		}

		checkTiledGrids := func(layoutOption googlemeet.LayoutOption) action.Action {
			return func(ctx context.Context) error {
				if layoutOption != googlemeet.TiledLayout {
					return nil
				}

				// Make sure it shows in a tile.
				if err := gm.ShowInATile()(ctx); err != nil {
					return err
				}

				// Check if there is more than 1 grid after changing layout to Tiled.
				expectedGrid := 1
				if conf.roomType == ClassRoomSize {
					// Check classrooms to expect grids to be more than 10 grids.
					expectedGrid = 10
				}
				startTime := time.Now()
				if err := testing.Poll(ctx, func(ctx context.Context) error {
					grids, err := conf.getStableGrids(ctx)
					if err != nil {
						return errors.Wrap(err, "failed to get stable grids")
					}
					if len(grids) <= expectedGrid {
						return errors.Wrapf(err, "unexpected grids: got: %v; want more than %v grids", len(grids), expectedGrid)
					}
					return nil
				}, &testing.PollOptions{Timeout: longUITimeout}); err != nil {
					return errors.Wrapf(err, "failed to wait for grids more than %v grids within %v", expectedGrid, longUITimeout)
				}
				testing.ContextLogf(ctx, "Get stable grids took %v to appear", time.Now().Sub(startTime))
				return nil
			}
		}

		changeLayout := uiauto.NamedCombine(fmt.Sprintf("change layout to %q", layoutOption),
			gm.OpenChangeLayout,
			gm.SetLayout(layoutOption),
			setToMaxTiles(layoutOption),
			uiauto.Retry(3, gm.CloseChangeLayout),
			checkLayoutChanged)

		return uiauto.Combine("change layout",
			conf.closeNotifDialog(),
			ui.Retry(3, changeLayout),
			ui.Retry(5, checkTiledGrids(layoutOption)),
		)(ctx)
	}
}

// BackgroundChange will sequentially change the background to blur, sky picture and turn off background effects.
func (conf *GoogleMeetConference) BackgroundChange(ctx context.Context) error {
	gm := conf.gm
	pinToMainScreen := func(ctx context.Context) error {
		pinBtn := nodewith.NameContaining("Pin").Role(role.Button)
		if err := conf.ui.WaitUntilExists(pinBtn)(ctx); err != nil {
			// If there are no participants in the room, the pin button will not be displayed.
			return ParticipantError(errors.Wrap(err, "failed to find the button to pin to main screen; other participants might have left"))
		}
		return uiauto.NamedAction("to pin to main screen", conf.ui.LeftClick(pinBtn))(ctx)
	}
	setEffect := func(effectOption googlemeet.EffectOption) action.Action {
		return func(ctx context.Context) error {
			if err := gm.SetEffect(effectOption)(ctx); err != nil {
				// Some accounts have no dynamic effects to choose, print the log for this situation.
				if effectOption == googlemeet.DynamicEffect {
					testing.ContextLog(ctx, "This account has no dynamic effects to choose")
				} else {
					return err
				}
			}
			return nil
		}
	}
	setEffectAndEnterFullScreen := func(effectOption googlemeet.EffectOption) action.Action {
		return uiauto.Combine("set effect and enter full screen",
			gm.ApplyVideoEffects(setEffect(effectOption)),
			takeScreenshot(conf.cr, conf.outDir, fmt.Sprintf("set-effect-to-%q", effectOption)),
			gm.EnterFullScreen,
			// After applying new background, give it 5 seconds for viewing before applying next one.
			uiauto.Sleep(viewingTime),
			gm.ExitFullScreen,
		)
	}

	if err := uiauto.Combine("pin to main screen and change background",
		conf.uiHandler.SwitchToChromeTabByName(meetTitle),
		conf.closeNotifDialog(),
		pinToMainScreen,
		setEffectAndEnterFullScreen(googlemeet.StaticEffect),
		setEffectAndEnterFullScreen(googlemeet.DynamicEffect),
		setEffectAndEnterFullScreen(googlemeet.BlurEffect),
	)(ctx); err != nil {
		return CheckSignedOutError(ctx, conf.tconn, err)
	}
	return nil
}

// Presenting creates Google Slides and Google Docs, shares screen and presents
// the specified application to the conference.
func (conf *GoogleMeetConference) Presenting(ctx context.Context, application googleApplication) (err error) {
	tconn, uiHandler, gm := conf.tconn, conf.uiHandler, conf.gm

	switchToTab := func(tabName string) action.Action {
		return uiauto.NamedAction("switch tab to "+tabName,
			uiHandler.SwitchToChromeTabByName(tabName))
	}

	shareScreen := uiauto.Combine("share screen",
		switchToTab(meetTitle),
		gm.ShareScreen(string(application)))

	stopPresenting := gm.StopShareScreen()

	if err := presentApps(ctx, tconn, uiHandler, conf.cr, conf.br, shareScreen, stopPresenting,
		application, conf.outDir); err != nil {
		return errors.Wrapf(err, "failed to present %q", application)
	}

	return nil
}

// End closes all windows in the end.
func (conf *GoogleMeetConference) End(ctx context.Context) error {
	return cuj.CloseAllWindows(ctx, conf.tconn)
}

// CloseConference closes the conference.
func (conf *GoogleMeetConference) CloseConference(ctx context.Context) error {
	return conf.gm.Close(ctx)
}

// SetBrowser sets browser to chrome or lacros.
func (conf *GoogleMeetConference) SetBrowser(br *browser.Browser) {
	conf.br = br
}

// checkLostNetwork checks for lost network connections.
func (conf *GoogleMeetConference) checkLostNetwork(ctx context.Context) error {
	const lostConnectionText = "You lost your network connection."
	lostConnection := nodewith.NameContaining(lostConnectionText).Role(role.StaticText)
	testing.ContextLog(ctx, "Check for lost network connection")
	if err := conf.ui.WithTimeout(5 * time.Second).WaitUntilExists(lostConnection)(ctx); err == nil {
		testing.ContextLog(ctx, "Lost network message: ", lostConnectionText)
		conf.networkLostCount++
		if err := takeScreenshot(conf.cr, conf.outDir, "lost-connection")(ctx); err == nil {
			testing.ContextLog(ctx, "Take screenshot for lost network connection")
		}
	}
	return nil
}

// LostNetworkCount returns the count of lost network connections.
func (conf *GoogleMeetConference) LostNetworkCount() int {
	return conf.networkLostCount
}

// DisplayAllParticipantsTime returns the loading time for displaying all participants.
func (conf *GoogleMeetConference) DisplayAllParticipantsTime() time.Duration {
	return conf.displayAllParticipantsTime
}

func (conf *GoogleMeetConference) closeNotifDialog() action.Action {
	gotItButton := nodewith.Name("Got it").Role(role.Button).Ancestor(meetWebArea)
	gotItPrompt := prompts.Prompt{
		Name:              "Got it",
		PromptFinder:      gotItButton,
		ClearButtonFinder: gotItButton,
	}
	// There may be multiple close buttons, so add First() here.
	closeButton := nodewith.Name("Close").Role(role.Button).Ancestor(meetWebArea).First()
	closeButtonPrompt := prompts.Prompt{
		Name:              "Close dialog",
		PromptFinder:      closeButton,
		ClearButtonFinder: closeButton,
	}

	return uiauto.Retry(retryTimes,
		prompts.ClearPotentialPrompts(
			conf.tconn,
			shortUITimeout,
			prompts.ShowNotificationsPrompt,
			gotItPrompt,
			closeButtonPrompt))
}
