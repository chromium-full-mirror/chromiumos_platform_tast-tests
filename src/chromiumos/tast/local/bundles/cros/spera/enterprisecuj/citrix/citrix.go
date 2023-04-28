// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package citrix contains utils for all operations on Citrix Workspace app.
package citrix

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"path"
	"strings"
	"time"

	"chromiumos/tast/common/action"
	"chromiumos/tast/local/apps"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/cuj"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/uiauto/state"
	"chromiumos/tast/local/coords"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/uidetection"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// WindowsApp defines all apps used in Citrix Workspace app.
type WindowsApp string

const (
	// GoogleChrome defines name of Google Chrome app.
	GoogleChrome WindowsApp = "Google Chrome"
	// TaskManager defines name of Task Manager.
	TaskManager WindowsApp = "Task Manager"
	// Adobe defines name of Adobe Acrobat DC.
	Adobe WindowsApp = "Adobe Acrobat"
)

// TestMode indicates whether to run in normal/record/replay mode.
type TestMode string

const (
	// ReplayMode represents "replay" mode.
	ReplayMode TestMode = "replay"
	// RecordMode represents "record" mode.
	RecordMode TestMode = "record"
	// NormalMode represents "normal" mode.
	NormalMode TestMode = "normal"
)

const (
	// shortUITimeout used for situations where UI response might be faster.
	shortUITimeout = 5 * time.Second
	// defaultUITimeout is the default timeout for UI interactions.
	defaultUITimeout = 15 * time.Second
	// longUITimeout is the longer timeout for UI interactions.
	longUITimeout = time.Minute
	// viewingTime used to view the effect after clicking application.
	viewingTime = 2 * time.Second
	// uiWaitTimeFactor used in replay mode, the record waiting time for UI is divided by this number
	// as the time waiting for UI in replay mode.
	uiWaitTimeFactor = 3
	// uiVerifyInterval is used in replay mode, the real ud.WaitUntilExist is called when the number
	// of executions of Fake UI Detects reaches uiVerifyInterval.
	uiVerifyInterval = 5
	// retryTimes is the maximum number of times the action will be retried.
	retryTimes = 3
)

// Citrix defines the struct related to Citrix Workspace app.
type Citrix struct {
	tconn        *chrome.TestConn
	ui           *uiauto.Context
	ud           *uidetection.Context
	udi          *uidetection.Context
	kb           *input.KeyboardEventWriter
	dataPath     func(string) string
	desktopTitle string
	appStartTime int64
	tabletMode   bool
	// testMode represents which mode is currently executed.
	// There are three modes: normal, record and replay mode.
	testMode      TestMode
	coordinates   map[string]coords.Point
	uiWaitTime    map[string]int
	uiVerifyCount int
}

// NewCitrix creates an instance of Citrix.
func NewCitrix(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, dataPath func(string) string, desktopTitle string, tabletMode bool, testMode TestMode) *Citrix {
	ud := uidetection.NewDefault(tconn)
	return &Citrix{
		tconn:        tconn,
		ui:           uiauto.New(tconn),
		ud:           ud,
		udi:          ud.WithScreenshotStrategy(uidetection.ImmediateScreenshot),
		kb:           kb,
		dataPath:     dataPath,
		desktopTitle: desktopTitle,
		tabletMode:   tabletMode,
		testMode:     testMode,
		// coordinates and uiWaitTime used in record and replay test mode.
		// In record mode, the coordinates and ui wait times of picture and text detected by uidetection will be
		// recorded to coordinates and uiWaitTime.
		// In replay mode, the coordinates and uiWait file will be loaded to the coordinates and uiWaitTime
		// in order to click and wait for the picture and text.
		coordinates: make(map[string]coords.Point),
		uiWaitTime:  make(map[string]int),
		// uiVerifyCount used in replay mode, it records the number of executions of fake ui detection.
		uiVerifyCount: 0,
	}
}

// Open opens Citrix Workspace app.
func (c *Citrix) Open() action.Action {
	return func(ctx context.Context) error {
		if err := ash.WaitForChromeAppInstalled(ctx, c.tconn, apps.Citrix.ID, 2*time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait for Citrix to install")
		}
		startTime := time.Now()
		if err := apps.Launch(ctx, c.tconn, apps.Citrix.ID); err != nil {
			return errors.Wrap(err, "failed to launch Citrix")
		}
		if err := ash.WaitForApp(ctx, c.tconn, apps.Citrix.ID, time.Minute); err != nil {
			return errors.Wrap(err, "failed to wait Citrix appear in shelf after launch")
		}
		c.appStartTime = time.Since(startTime).Milliseconds()
		return nil
	}
}

// AppStartTime returns the startup time of Citrix Workspace app in milliseconds.
func (c *Citrix) AppStartTime() int64 {
	return c.appStartTime
}

// Login logs in to Citrix Workspace app.
func (c *Citrix) Login(serverURL, userName, password string) action.Action {
	searchWorkspace := nodewith.Name("Search Workspace").Role(role.Button)
	logOnAreaFinder := nodewith.HasClass("logon-area").Role(role.LayoutTable)
	editField := nodewith.State(state.Editable, true).Role(role.GenericContainer).Ancestor(logOnAreaFinder)
	serverURLField := nodewith.Name("Store URL or Email address").Role(role.TextField).Ancestor(logOnAreaFinder)
	userNameField := editField.Name("User name:").Role(role.TextField)
	passwordField := editField.Name("Password:").Role(role.TextField)
	connectBtn := nodewith.Name("Connect").HasClass("button").Role(role.Link).Ancestor(logOnAreaFinder)
	logOnBtn := nodewith.Name("Log On").HasClass("button").Role(role.Link).Ancestor(logOnAreaFinder)
	connectServer := uiauto.NamedCombine("connect to Citrix server",
		c.ui.LeftClick(editField.Ancestor(serverURLField)),
		c.kb.TypeAction(serverURL),
		c.ui.DoDefault(connectBtn))
	return uiauto.IfFailThen(c.ui.WithTimeout(shortUITimeout).WaitUntilExists(searchWorkspace),
		uiauto.NamedCombine("login to Citrix",
			uiauto.IfSuccessThen(c.ui.WithTimeout(shortUITimeout).WaitUntilExists(serverURLField), connectServer),
			c.ui.LeftClickUntil(userNameField, c.ui.Exists(userNameField.Focused())),
			c.kb.TypeAction(userName),
			c.ui.LeftClickUntil(passwordField, c.ui.Exists(passwordField.Focused())),
			c.kb.TypeAction(password),
			c.ui.DoDefault(logOnBtn),
			c.ui.WaitUntilExists(searchWorkspace),
		))
}

// Logout logouts Citrix Workspace app.
func (c *Citrix) Logout() action.Action {
	accountLogo := nodewith.Name("C").Role(role.StaticText)
	logOutText := nodewith.Name("Log Out").Role(role.StaticText)
	backToSignIn := nodewith.Name("Back to Sign In").Role(role.Button)
	return uiauto.NamedCombine("logout",
		uiauto.IfFailThen(c.ui.WithTimeout(shortUITimeout).WaitUntilExists(accountLogo), c.kb.AccelAction("Esc")),
		c.ui.LeftClick(accountLogo),
		c.ui.LeftClick(logOutText),
		c.ui.LeftClick(backToSignIn),
	)
}

// ConnectRemoteDesktop connects to the remote desktop by given desktop name.
func (c *Citrix) ConnectRemoteDesktop(desktop string) action.Action {
	uiCtx := uiContext("ConnectRemoteDesktop")
	searchWorkspace := nodewith.Name("Search Workspace").Role(role.Button)
	listBoxFinder := nodewith.Ancestor(nodewith.HasClass("ul_vabzqc").Role(role.ListBox))
	listApp := listBoxFinder.Name(desktop).Role(role.ListBoxOption)
	cancelButton := c.customIcon(IconTrackerCancel)
	return uiauto.NamedCombine("connect to remote desktop "+desktop+" in Citrix Workspace",
		c.ui.DoDefault(searchWorkspace),
		c.kb.TypeAction(desktop),
		c.ui.DoDefault(listApp),
		c.waitIcon(uiCtx, IconToolbar),
		uiauto.IfSuccessThen(c.ud.Exists(cancelButton), c.clickIcon(uiCtx, IconTrackerCancel)),
	)
}

// FocusOnDesktop focuses on desktop.
// Sometimes, some actions are performed on chrome os when Citrix Workplace is open without
// focus on it. If there is focus to the desktop, this problem can be solved.
func (c *Citrix) FocusOnDesktop() action.Action {
	uiCtx := uiContext("FocusOnDesktop")
	return c.clickIcon(uiCtx, IconDesktop)
}

// FullscreenDesktop sets Citrix remote desktop to fullscreen.
func (c *Citrix) FullscreenDesktop() action.Action {
	return func(ctx context.Context) error {
		if !c.tabletMode {
			// Wait for the citrix remote desktop window to open.
			if _, err := ash.WaitForAnyWindowWithTitle(ctx, c.tconn, c.desktopTitle); err != nil {
				return errors.Wrap(err, "could not find the citrix remote desktop window")
			}
			window, err := ash.FindWindow(ctx, c.tconn, func(w *ash.Window) bool {
				return w.WindowType == ash.WindowTypeExtension && strings.Contains(w.Title, c.desktopTitle)
			})
			if err != nil {
				return errors.Wrapf(err, "failed to find the %q window", c.desktopTitle)
			}
			if err := ash.SetWindowStateAndWait(ctx, c.tconn, window.ID, ash.WindowStateFullscreen); err != nil {
				// Just log the error and try to continue.
				testing.ContextLogf(ctx, "Try to continue the test even though fullscreen the %q window failed: %v", c.desktopTitle, err)
			}
		}
		return nil
	}
}

// ExitFullscreenDesktop sets Citrix remote desktop to exit fullscreen.
func (c *Citrix) ExitFullscreenDesktop() action.Action {
	return func(ctx context.Context) error {
		if !c.tabletMode {
			// Wait for the citrix remote desktop window to open.
			if _, err := ash.WaitForAnyWindowWithTitle(ctx, c.tconn, c.desktopTitle); err != nil {
				return errors.Wrap(err, "could not find the citrix remote desktop window")
			}
			window, err := ash.FindWindow(ctx, c.tconn, func(w *ash.Window) bool {
				return w.WindowType == ash.WindowTypeExtension && strings.Contains(w.Title, c.desktopTitle)
			})
			if err != nil {
				return errors.Wrapf(err, "failed to find the %q window", c.desktopTitle)
			}
			if err := ash.SetWindowStateAndWait(ctx, c.tconn, window.ID, ash.WindowStateNormal); err != nil {
				// Just log the error and try to continue.
				testing.ContextLogf(ctx, "Try to continue the test even though fullscreen the %q window failed: %v", c.desktopTitle, err)
			}
		}
		return nil
	}
}

// NewTab opens chrome broswer with url in the remote desktop.
func (c *Citrix) NewTab(url string, newWindow bool) action.Action {
	uiCtx := uiContext("NewTab")
	return func(ctx context.Context) error {
		newTabAction := c.kb.AccelAction("Ctrl+T")
		if newWindow {
			newTabAction = func(ctx context.Context) error {
				desktop := c.customIcon(IconDesktop)
				if err := c.ud.Exists(desktop)(ctx); err != nil {
					return c.kb.AccelAction("Ctrl+N")(ctx)
				}
				return c.searchToOpenApplication(GoogleChrome)(ctx)
			}
		}
		return uiauto.NamedCombine("open tab "+url,
			newTabAction,
			c.waitText(uiCtx, "Search Google"),
			c.kb.TypeAction(url),
			c.kb.AccelAction("Enter"),
		)(ctx)
	}
}

// OpenChromeWithURLs opens chrome broswer with urls in the remote desktop.
func (c *Citrix) OpenChromeWithURLs(urls []string) action.Action {
	return func(ctx context.Context) error {
		for i, url := range urls {
			if err := c.NewTab(url, i == 0)(ctx); err != nil {
				return err
			}
		}
		return nil
	}
}

// Navigate navigates page to url.
func (c *Citrix) Navigate(url string) action.Action {
	return uiauto.NamedCombine("navigate to "+url,
		c.kb.AccelAction("Ctrl+l"),
		c.kb.TypeAction(url),
		c.kb.AccelAction("Enter"),
	)
}

// searchToOpenApplication searchs and open the application in the remote desktop.
func (c *Citrix) searchToOpenApplication(appName WindowsApp) action.Action {
	uiCtx := uiContext("searchToOpenApplication")
	return uiauto.NamedCombine("search to open application "+string(appName),
		c.clickIcon(uiCtx, IconSearch),
		c.waitText(uiCtx, "Type here to search"),
		c.kb.TypeAction(string(appName)),
		c.clickText(uiCtx, "Desktop app"))
}

// SearchFromWiki searchs from wiki.
func (c *Citrix) SearchFromWiki(text string) action.Action {
	uiCtx := uiContext("SearchFromWiki")
	return uiauto.NamedCombine("search '"+text+"' from wiki",
		c.waitIcon(uiCtx, IconChromeWikiSearch),
		c.kb.TypeAction(text),
		c.kb.AccelAction("Enter"),
		uiauto.Sleep(viewingTime),
	)
}

// SearchFromGoogle searchs from Google.
func (c *Citrix) SearchFromGoogle(text string) action.Action {
	googleSearchText := uidetection.TextBlockFromSentence("Google Search").First()
	return uiauto.NamedCombine("search '"+text+"' from Google",
		c.ud.WaitUntilExists(googleSearchText),
		c.kb.TypeAction(text),
		c.kb.AccelAction("Enter"),
		uiauto.Sleep(viewingTime),
	)
}

// SwitchWindow switches window to next window.
func (c *Citrix) SwitchWindow() action.Action {
	return uiauto.NamedCombine("swich window",
		c.kb.AccelAction("Alt+Esc"),
		uiauto.Sleep(viewingTime),
	)
}

// SwitchTab switches tab to next tab.
func (c *Citrix) SwitchTab() action.Action {
	return uiauto.NamedCombine("swich tab",
		c.kb.AccelAction("Ctrl+Tab"),
		uiauto.Sleep(viewingTime),
	)
}

// CreateGoogleKeepNote opens Google keep and create new note.
func (c *Citrix) CreateGoogleKeepNote(text string) action.Action {
	uiCtx := uiContext("CreateGoogleKeepNote")
	return uiauto.NamedCombine("open google keep and create new note",
		c.NewTab(cuj.GoogleKeepURL, true),
		c.clickText(uiCtx, "Take a note..."),
		c.kb.TypeAction(text),
		c.kb.AccelAction("Esc"), // Save note.
		c.waitText(uiCtx, text),
	)
}

// DeleteGoogleKeepNote deletes note from Google keep.
func (c *Citrix) DeleteGoogleKeepNote(text string) action.Action {
	noteText := uidetection.TextBlockFromSentence(text).First()
	return uiauto.Retry(retryTimes,
		uiauto.NamedCombine("delete note from google keep",
			c.kb.TypeAction("k"),        // Select note.
			c.kb.AccelAction("Shift+3"), // Delete note.
			c.ud.WithTimeout(shortUITimeout).WaitUntilGone(noteText),
		))
}

// UploadPhoto uploads photo to Google photo.
func (c *Citrix) UploadPhoto(filename string) action.Action {
	uiCtx := uiContext("UploadPhoto")
	uploadButton := c.customIcon(IconPhotosUpload)
	downloadButton := c.customIcon(IconPhotosDownload)
	fileFinder := uidetection.Word(filename).Above(uidetection.Word("Cancel"))

	verifiedAndMeasureUploadTime := func(ctx context.Context) error {
		itemUploadedText := uidetection.TextBlockFromSentence("1 item uploaded")
		storageText := uidetection.TextBlockFromSentence("Storage").First()
		addToAlbumText := uidetection.TextBlockFromSentence("Add to album").First()
		if err := c.ud.WithTimeout(shortUITimeout).WaitUntilExists(storageText)(ctx); err == nil {
			itemUploadedText = itemUploadedText.Below(storageText)
		} else {
			itemUploadedText = itemUploadedText.Above(addToAlbumText)
		}
		startTime := time.Now()
		if err := c.ud.WaitUntilExists(itemUploadedText)(ctx); err != nil {
			return err
		}
		uploadTime := time.Now().Sub(startTime)
		testing.ContextLog(ctx, "Upload photo to Google photo took ", uploadTime)
		return nil
	}

	return uiauto.NamedCombine("upload photo to Google photo",
		c.NewTab(cuj.GooglePhotosURL, true),
		uiauto.IfFailThen(c.ud.WithTimeout(shortUITimeout).LeftClick(uploadButton),
			c.clickIcon(uiCtx, IconPhotosUploadSmall)),
		c.clickIcon(uiCtx, IconPhotosComputer),
		uiauto.IfSuccessThen(c.ud.WithTimeout(shortUITimeout).WaitUntilExists(downloadButton),
			c.clickIcon(uiCtx, IconPhotosDownload)),
		c.ud.LeftClick(fileFinder),
		c.kb.AccelAction("Enter"),
		verifiedAndMeasureUploadTime,
	)
}

// DeletePhoto deletes photo from Google photo.
func (c *Citrix) DeletePhoto() action.Action {
	uiCtx := uiContext("DeletePhoto")
	return uiauto.NamedCombine("delete photo from Google photo",
		c.kb.AccelAction("Right"),
		c.kb.AccelAction("Enter"),
		c.clickIcon(uiCtx, IconPhotosDelete),
		c.clickText(uiCtx, "Move to trash"),
	)
}

// OpenZoom opens the zoom website, log in and enter the room.
func (c *Citrix) OpenZoom(room, account, password string) action.Action {
	uiCtx := uiContext("OpenZoom")
	clickJoinFromBrowser := func(ctx context.Context) error {
		// uidetection.TextBlockFromSentence get the center point of the whole text line.
		// But need to click the link on the right, so correct the coordinates to click on the target link.
		downloadNowText := uidetection.TextBlockFromSentence("Download Now").First()
		joinFromBrowserButton := uidetection.TextBlockFromSentence("Join from Your Browser").Below(downloadNowText)
		l, err := c.ud.Location(ctx, joinFromBrowserButton)
		if err != nil {
			return err
		}
		expectedlocation := coords.Point{X: l.Right() - 5, Y: l.CenterY()}
		return c.ui.MouseClickAtLocation(0, expectedlocation)(ctx)
	}
	login := func(ctx context.Context) error {
		scheduleWord := uidetection.Word("Schedule").First()
		accountFinder := uidetection.TextBlockFromSentence(account).First()
		waitForScheduleWord := c.ud.WithTimeout(10 * time.Second).WaitUntilExists(scheduleWord)
		enterPassword := uiauto.NamedCombine("enter password",
			c.kb.TypeAction(password),
			c.kb.AccelAction("Enter"),
			c.waitText(uiCtx, "Schedule"),
		)
		return uiauto.NamedCombine("login zoom",
			c.clickFinder(uiCtx+"Account", accountFinder),
			uiauto.IfFailThen(waitForScheduleWord, enterPassword),
		)(ctx)
	}
	clickCancelButton := func(ctx context.Context) error {
		yourBroswerText := uidetection.TextBlockFromSentence("your browser").First()
		cancelButton := uidetection.Word("Cancel").Above(yourBroswerText)
		return c.ud.LeftClick(cancelButton)(ctx)
	}
	clicklaunchMeetingButton := func(ctx context.Context) error {
		termsOfServicesText := uidetection.TextBlockFromSentence("Terms of Services").First()
		launchMeetingButton := uidetection.TextBlockFromSentence("Launch Meeting").Below(termsOfServicesText)
		return c.ud.LeftClick(launchMeetingButton)(ctx)
	}
	openCamera := func(ctx context.Context) error {
		startVideoButton := uidetection.TextBlockFromSentence("Start Video").First()
		stopVideoButton := uidetection.TextBlockFromSentence("Stop Video").First()
		return uiauto.NamedCombine("open camera",
			uiauto.IfSuccessThen(c.udi.WithTimeout(shortUITimeout).WaitUntilExists(startVideoButton), c.udi.LeftClick(startVideoButton)),
			c.udi.WaitUntilExists(stopVideoButton))(ctx)
	}
	joinButton := c.customIcon(IconZoomJoin)
	joinMeeting := uiauto.NamedCombine("join meeting",
		clickCancelButton,
		clicklaunchMeetingButton,
		clickCancelButton,
		clickJoinFromBrowser,
		openCamera,
		c.udi.LeftClick(joinButton))
	return uiauto.NamedCombine("open zoom web",
		c.NewTab(cuj.ZoomSignInURL, true),
		login,
		c.Navigate(room),
		joinMeeting,
	)
}

// PresentWindowFromZoom presents a window from zoom web.
func (c *Citrix) PresentWindowFromZoom() action.Action {
	shareScreenButton := uidetection.TextBlockFromSentence("Share Screen").First()
	windowWord := uidetection.Word("Window").First()
	photosWord := uidetection.Word("Photos").First()
	shareButton := uidetection.Word("Share").Below(photosWord)
	stopSharingButton := uidetection.TextBlockFromSentence("Stop sharing").First()
	return uiauto.NamedCombine("share window",
		c.udi.LeftClick(shareScreenButton),
		c.udi.LeftClick(windowWord),
		c.udi.LeftClick(photosWord),
		c.udi.LeftClick(shareButton),
		c.udi.WaitUntilExists(stopSharingButton),
	)
}

// StopSharing stops sharing from zoom web.
func (c *Citrix) StopSharing() action.Action {
	stopSharingButton := uidetection.TextBlockFromSentence("Stop sharing").First()
	return uiauto.NamedAction("stop sharing", c.udi.LeftClick(stopSharingButton))
}

// OpenPDF opens Adobe app and the welcome PDF.
func (c *Citrix) OpenPDF() action.Action {
	uiCtx := uiContext("OpenPDF")
	nameWord := uidetection.Word("NAME").First()
	welcomeWord := uidetection.Word("Welcome").Below(nameWord).First()
	return uiauto.NamedCombine("open PDF",
		c.searchToOpenApplication(Adobe),
		c.clickFinder(uiCtx+"Welcome", welcomeWord),
		c.waitText(uiCtx, "Remove From Recent"),
		c.kb.AccelAction("Enter"),
		c.waitText(uiCtx, "Welcome to"),
	)
}

// ShowTheDesktop minimizes all windows.
func (c *Citrix) ShowTheDesktop() action.Action {
	uiCtx := uiContext("ShowTheDesktop")
	toolBar := c.customIcon(IconToolbar)
	return uiauto.NamedCombine("minimize and redisplay all browser windows",
		c.ud.RightClick(toolBar),
		c.clickText(uiCtx, "Show the desktop"),
		c.ud.RightClick(toolBar),
		c.clickText(uiCtx, "Show open windows"),
	)
}

// CloseApplications closes applications by task mangaer in the remote desktop.
func (c *Citrix) CloseApplications(appNames []WindowsApp) action.Action {
	closeApp := func(appName string) action.Action {
		appText := uidetection.Word("Apps").First()
		appNameFinder := uidetection.TextBlockFromSentence(appName).Below(appText).First()
		endTask := uidetection.TextBlockFromSentence("End Task").First()
		return uiauto.IfSuccessThen(
			c.udi.WithTimeout(defaultUITimeout).WaitUntilExists(appNameFinder),
			uiauto.NamedCombine("close app "+appName,
				c.udi.WithTimeout(defaultUITimeout).LeftClick(appNameFinder),
				c.udi.WithTimeout(shortUITimeout).LeftClick(endTask)))
	}
	closeApps := func(ctx context.Context) error {
		for _, appName := range appNames {
			if err := closeApp(string(appName))(ctx); err != nil {
				return errors.Wrap(err, "failed to close app")
			}
		}
		return nil
	}
	desktop := c.customIcon(IconDesktop)
	appsCount := uidetection.TextBlockFromSentence("Apps (1)").First()
	return uiauto.IfSuccessThen(c.ud.Gone(desktop),
		uiauto.NamedCombine("close applications by task mangaer",
			c.searchToOpenApplication(TaskManager),
			c.ui.WithTimeout(longUITimeout).RetryUntil(closeApps, c.udi.WithTimeout(shortUITimeout).WaitUntilExists(appsCount)),
			c.kb.AccelAction("Esc"),
		))
}

// CloseAllChromeBrowsers closes all chrome browsers in the remote desktop.
func (c *Citrix) CloseAllChromeBrowsers() action.Action {
	desktop := c.customIcon(IconDesktop)
	chromeActiveIcon := c.customIcon(IconChromeActive)
	cancelButton := uidetection.Word("Cancel").First()
	return uiauto.Retry(retryTimes, uiauto.IfSuccessThen(c.ud.Gone(desktop),
		uiauto.NamedCombine("close chrome browser",
			uiauto.IfSuccessThen(c.ud.Exists(cancelButton), c.ud.LeftClick(cancelButton)),
			c.ud.RightClick(chromeActiveIcon),
			uiauto.Sleep(2*time.Second), // Sleep to wait for the menu to pop up.
			c.kb.AccelAction("Up"),
			uiauto.Sleep(time.Second), // Sleep to wait to focus on closing option.
			c.kb.AccelAction("Enter"),
			c.ud.WithTimeout(defaultUITimeout).WaitUntilExists(desktop),
		)))
}

// Close closes Citrix app and remote desktop.
func (c *Citrix) Close(ctx context.Context) error {
	w, err := ash.GetActiveWindow(ctx, c.tconn)
	if err != nil {
		return errors.Wrap(err, "failed to obtain the active window")
	}
	if err := w.CloseWindow(ctx, c.tconn); err != nil {
		return errors.Wrap(err, "failed to close the active window")
	}
	if err := apps.Close(ctx, c.tconn, apps.Citrix.ID); err != nil {
		return errors.Wrap(err, "failed to close Citrix app")
	}
	return nil
}

// SaveRecordFile saves the set of coordinates of finders to the file "coordinates.json" and
// the set of ui wait time to the file "uiwait.json".
func (c *Citrix) SaveRecordFile(ctx context.Context, outDir string) error {
	const (
		coordFileName  = "coordinates.json"
		uiWaitFileName = "uiwait.json"
	)
	filePath := path.Join(outDir, coordFileName)
	j, err := json.MarshalIndent(c.coordinates, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshall data for %s json file: %v", coordFileName, c.coordinates)
	}
	if err := ioutil.WriteFile(filePath, j, 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s json file", coordFileName)
	}

	filePath = path.Join(outDir, uiWaitFileName)
	j, err = json.MarshalIndent(c.uiWaitTime, "", "  ")
	if err != nil {
		return errors.Wrapf(err, "failed to marshall data for %s json file: %v", uiWaitFileName, c.uiWaitTime)
	}
	if err := ioutil.WriteFile(filePath, j, 0644); err != nil {
		return errors.Wrapf(err, "failed to write %s json file", uiWaitFileName)
	}
	return nil
}

// LoadRecordFile loads the set of coordinates of finders and the set of ui wait time from the json files.
func (c *Citrix) LoadRecordFile(coordFileName, uiWaitFileName string) error {
	byteValue, err := ioutil.ReadFile(c.dataPath(coordFileName))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(byteValue, &c.coordinates); err != nil {
		return err
	}
	byteValue, err = ioutil.ReadFile(c.dataPath(uiWaitFileName))
	if err != nil {
		return err
	}
	if err = json.Unmarshal(byteValue, &c.uiWaitTime); err != nil {
		return err
	}
	return nil
}

// customIcon returns uidetection finder with file name.
func (c *Citrix) customIcon(name string) *uidetection.Finder {
	return uidetection.CustomIcon(c.dataPath(name))
}

// clickIcon returns action to click the icon.
// The prefix parameter is used to distinguish which function the action is executed in.
// To avoid encountering the situation that different functions have the same picture and text with different coordinates.
func (c *Citrix) clickIcon(prefix, name string) action.Action {
	finder := c.customIcon(name)
	return c.clickFinder(prefix+name, finder)
}

// clickText returns action to click the text.
// The prefix parameter is used to distinguish which function the action is executed in.
// To avoid encountering the situation that different functions have the same picture and text with different coordinates.
func (c *Citrix) clickText(prefix, name string) action.Action {
	finder := uidetection.TextBlockFromSentence(name).First()
	return c.clickFinder(prefix+name, finder)
}

// clickFinder returns action to click the finder.
func (c *Citrix) clickFinder(name string, finder *uidetection.Finder) action.Action {
	return func(ctx context.Context) error {
		if c.testMode == RecordMode {
			startTime := time.Now()
			l, err := c.ud.Location(ctx, finder)
			if err != nil {
				return err
			}
			c.uiWaitTime[name] = int(time.Now().Sub(startTime).Milliseconds())
			c.coordinates[name] = l.Rect.CenterPoint()
			testing.ContextLogf(ctx, "Mouse click at %s with location %v", name, l.Rect.CenterPoint())
			return c.ui.MouseClickAtLocation(0, l.Rect.CenterPoint())(ctx)
		} else if c.testMode == ReplayMode {
			empty := coords.Point{}
			if c.coordinates[name] != empty {
				actionName := fmt.Sprintf("mouse click at %s with location %v", name, c.coordinates[name])
				return uiauto.NamedCombine(actionName,
					uiauto.Sleep(time.Duration(float64(c.uiWaitTime[name]/uiWaitTimeFactor)*float64(time.Millisecond))),
					c.ui.MouseClickAtLocation(0, c.coordinates[name]),
				)(ctx)
			}
		}
		return c.ud.LeftClick(finder)(ctx)
	}
}

// waitIcon returns action to wait the icon.
// The prefix parameter is used to distinguish which function the action is executed in.
// To avoid encountering the situation that different functions have the same picture and text with different wait time.
func (c *Citrix) waitIcon(prefix, name string) action.Action {
	finder := c.customIcon(name)
	return c.waitFinder(prefix+name, finder)
}

// waitText returns action to wait the text.
// The prefix parameter is used to distinguish which function the action is executed in.
// To avoid encountering the situation that different functions have the same picture and text with different wait time.
func (c *Citrix) waitText(prefix, name string) action.Action {
	finder := uidetection.TextBlockFromSentence(name).First()
	return c.waitFinder(prefix+name, finder)
}

// waitFinder returns action to wait the finder.
func (c *Citrix) waitFinder(name string, finder *uidetection.Finder) action.Action {
	return func(ctx context.Context) error {
		if c.testMode == RecordMode {
			startTime := time.Now()
			if err := c.ud.WaitUntilExists(finder)(ctx); err != nil {
				return err
			}
			c.uiWaitTime[name] = int(time.Now().Sub(startTime).Milliseconds())
			return nil
		} else if c.testMode == ReplayMode {
			// The real ud.WaitUntilExist is called when the number of executions of Fake UI Detects
			// reaches uiVerifyInterval.
			if c.uiVerifyCount%uiVerifyInterval == uiVerifyInterval-1 {
				c.uiVerifyCount = 0
			} else {
				if c.uiWaitTime[name] != 0 {
					c.uiVerifyCount++
					return uiauto.Sleep(time.Duration(float64(c.uiWaitTime[name]/uiWaitTimeFactor) * float64(time.Millisecond)))(ctx)
				}
			}
		}
		return c.ud.WaitUntilExists(finder)(ctx)
	}
}

// uiContext returns the string combination of dash symbol.
func uiContext(name string) string {
	return name + "-"
}
