// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package fakehtml contains common data types and UI libraries for fake HTML.
package fakehtml

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/prompts"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
)

// UIObject represents the fake HTML page UI.
type UIObject struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
}

const (
	// PageTitle is the page title.
	PageTitle = "Simple Meeting"
	// PageURL is the page base URL.
	PageURL = "/effects_video_script.html?resolution="
)

var rootWebArea = nodewith.Role(role.RootWebArea).Name(PageTitle)

// VideoNode is the node showing camera.
var VideoNode = nodewith.Role(role.Video)

// NewUI creates a new instance to represent the fake HTML page UI.
func NewUI(tconn *chrome.TestConn) UIObject {
	return UIObject{
		tconn: tconn,
		ui:    uiauto.New(tconn),
	}
}

// MayBeAllowCameraAccess allows Camera access if the prompt is shown up.
func (htmlUI UIObject) MayBeAllowCameraAccess(ctx context.Context) error {
	return prompts.ClearPotentialPrompts(htmlUI.tconn, 3*time.Second, prompts.AllowAVPermissionPrompt)(ctx)
}

// EnterFullScreen double clicks video frame and waits for the window to be full screen.
func (htmlUI UIObject) EnterFullScreen(ctx context.Context) error {
	return htmlUI.ui.RetryUntil(
		htmlUI.ui.DoubleClick(VideoNode),
		ash.WaitForFullscreenConditionWithTitle(htmlUI.tconn, PageTitle, true, 5*time.Second),
	)(ctx)
}
