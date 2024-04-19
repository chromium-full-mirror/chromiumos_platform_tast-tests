// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package webrtc

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast/core/errors"
)

const (
	// vcHTML is the HTML holding a video conference using WebRTC API.
	vcHTML = "webrtc/video_conference.html"
	// vcTitle is the html title of vcHTML.
	vcTitle = "WebRTC VideoConference"
	// presentHTML is the HTML changing the content in 30fps for presentation.
	presentHTML = "webrtc/presentation.html"
	// presentTitle is the html title of presentHTML.
	presentTitle = "presentation test"
	// textHTML is the HTML that has a text area.
	textHTML = "webrtc/text.html"
	// textTitle is the html title of presentHTML.
	textTitle = "text test"
	// mouseHTML is the HTML that has letters.
	mouseHTML = "webrtc/mouse.html"
	// mouseTitle is the html title of mouseHTML.
	mouseTitle = "mouse test"
)

// windowManager manages the main window (video conference) and sub window(text, mouse, presentation).
type windowManager struct {
	connSource ash.ConnSource
	// The URL in video conference page.
	vcWinURL string
	// The URL in the page in the sub window. This is empty if the sub window is not required.
	subWinURL string
	// The page title in the sub window. This is empty if the sub window is not required.
	subWinTitle string
	// The functions to be called to close connections that are established by openVCWindow() and openSubWindow().
	closeConnFuncs []func()
}

func newWindowManager(cs ash.ConnSource, serverURL string, present, text, mouse bool) *windowManager {
	var subWinURL, subWinTitle string
	if text {
		subWinURL = serverURL + "/" + textHTML
		subWinTitle = textTitle
	} else if mouse {
		subWinURL = serverURL + "/" + mouseHTML
		subWinTitle = mouseTitle
	} else if present {
		subWinURL = serverURL + "/" + presentHTML
		subWinTitle = presentTitle
	}
	return &windowManager{
		connSource:  cs,
		vcWinURL:    serverURL + "/" + vcHTML,
		subWinURL:   subWinURL,
		subWinTitle: subWinTitle,
	}
}

func (wm *windowManager) closeConns() {
	for i := len(wm.closeConnFuncs) - 1; i >= 0; i-- {
		wm.closeConnFuncs[i]()
	}
}

// hasSubWindow returns true if the sub window needs to be opened or already opens.
func (wm *windowManager) hasSubWindow() bool {
	return wm.subWinURL != ""
}

func (wm *windowManager) openVCWindow(ctx context.Context) (*chrome.Conn, error) {
	return wm.openWindow(ctx, wm.vcWinURL)
}

func (wm *windowManager) openSubWindow(ctx context.Context) (*chrome.Conn, error) {
	return wm.openWindow(ctx, wm.subWinURL, browser.WithNewWindow())
}

func (wm *windowManager) openWindow(ctx context.Context, url string, opts ...browser.CreateTargetOption) (*chrome.Conn, error) {
	conn, err := wm.connSource.NewConn(ctx, url, opts...)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to open %s", url)
	}
	wm.closeConnFuncs = append(wm.closeConnFuncs,
		func() {
			conn.CloseTarget(ctx)
			conn.Close()
		})
	if err := conn.WaitForExpr(ctx, "document.readyState === 'complete'"); err != nil {
		return nil, errors.Wrap(err, "timed out waiting for page loading")
	}
	return conn, nil
}

// setupWindowView maximizes the video conference window if only the window exists, or
// snap windows of the video conference window and the presentation/text window.
func (wm *windowManager) setupWindowView(ctx context.Context, tconn *chrome.TestConn) error {
	vcWin, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, vcTitle)
	if err != nil {
		return errors.Wrap(err, "failed to get the video conference window")
	}

	// Maximize the video conference window if we don't open a new window
	if !wm.hasSubWindow() {
		if err := ash.SetWindowStateAndWait(ctx, tconn, vcWin.ID, ash.WindowStateMaximized); err != nil {
			return errors.Wrap(err, "failed to maximize the video conference window")
		}
		return nil
	}

	// Snap a video conference window and a presentation/text window.
	if err := ash.SetWindowStateAndWait(ctx, tconn, vcWin.ID, ash.WindowStatePrimarySnapped); err != nil {
		return errors.Wrap(err, "failed to set up the video conference window")
	}

	newWin, err := ash.WaitForAnyWindowWithTitle(ctx, tconn, wm.subWinTitle)
	if err != nil {
		return errors.Wrap(err, "failed to get the sub window")
	}
	if err := ash.SetWindowStateAndWait(ctx, tconn, newWin.ID, ash.WindowStateSecondarySnapped); err != nil {
		return errors.Wrap(err, "failed to set up the sub window")
	}
	return nil
}

// activateSubWindow activates the sub window, in other words, the window is focused
// so the mouse event and key input event is performed in the sub window.
func (wm *windowManager) activateSubWindow(ctx context.Context, bTconn *chrome.TestConn) error {
	if err := browser.ActivateTabByTitle(ctx, bTconn, wm.subWinTitle); err != nil {
		return errors.Wrap(err, "failed activating video conference window")
	}
	return nil
}
