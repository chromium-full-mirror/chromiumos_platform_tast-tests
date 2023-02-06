// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package dragdrop contains functionality shared by tests that
// exercise Drag and Drop restrictions of DLP.
package dragdrop

import (
	"context"
	"time"

	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/mouse"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"
	"chromiumos/tast/local/chrome/uiauto/state"
)

// AppName is used to identify source and target apps for drag and drop.
type AppName string

const (
	// Chrome to be used as source or destination.
	Chrome AppName = "chrome"
	// Settings to be used as source or destination.
	Settings AppName = "os-settings"
	//FileManager to be used as source or destination.
	FileManager AppName = "file-manager"
)

func (app AppName) String() string {
	return string(app)
}

// DragDrop drags the content specified from a source website to a text box.
func DragDrop(ctx context.Context, tconn *chrome.TestConn, content string, dstNode *nodewith.Finder) error {
	ui := uiauto.New(tconn)
	endLocation, err := ui.Location(ctx, dstNode)
	if err != nil {
		return errors.Wrap(err, "destination textbox not found")
	}

	contentNode := nodewith.Name(content).First()
	start, err := ui.Location(ctx, contentNode)
	if err != nil {
		return errors.Wrap(err, "failed to get location for content")
	}

	if err := uiauto.Combine("Drag and Drop",
		mouse.Drag(tconn, start.CenterPoint(), endLocation.CenterPoint(), time.Second*2))(ctx); err != nil {
		return errors.Wrap(err, "failed to verify content preview for")
	}

	return nil
}

// WaitForStableCoordinates waits for coordinates of the target text box to be stable.
func WaitForStableCoordinates(ctx context.Context, tconn *chrome.TestConn) error {
	ui := uiauto.New(tconn)

	textBoxNode := nodewith.Name("textarea").Role(role.TextField).State(state.Editable, true).First()
	// Getting the location also waits for it to be stable.
	_, err := ui.Location(ctx, textBoxNode)
	if err != nil {
		return errors.Wrap(err, "failed to get the location of destination text box")
	}

	return nil
}
