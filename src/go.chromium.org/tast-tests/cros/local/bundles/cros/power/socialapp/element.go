// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package socialapp

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/element"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
)

const (
	testRoomID   = "#power_test:matrix.org"
	testRoomName = "Power test room"
)

var (
	// ElementAppName represents the name of the Element app.
	ElementAppName AppName = apps.Element.Name
	elementID              = apps.Element.ID
)

// Element implements the SocialApp interface with the Element app.
type Element struct {
	ele      *element.Element
	tconn    *chrome.TestConn
	username string
	roomName string
}

// NewElement returns a new Element object.
func NewElement(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *ui.Device, username string) *Element {
	return &Element{
		ele:      element.New(tconn, kb, a, d),
		tconn:    tconn,
		username: username,
	}
}

// Install installs or updates the Element app through Play Store.
func (e *Element) Install(ctx context.Context) error {
	return e.ele.Install(ctx)
}

// Uninstall uninstalls the Element app if it is installed during the test.
func (e *Element) Uninstall(ctx context.Context) error {
	return e.ele.Uninstall(ctx)
}

// Launch launches the Element app.
func (e *Element) Launch(ctx context.Context) error {
	return e.ele.Launch(ctx, appLaunchTimeout)
}

// Close closes the Element app.
func (e *Element) Close(ctx context.Context) error {
	return e.ele.Close(ctx)
}

// SetUp logs in to the Element app and creates a new room.
func (e *Element) SetUp(ctx context.Context) error {
	if err := apputil.DismissMobilePrompt(ctx, e.tconn); err != nil {
		return errors.Wrap(err, "failed to dismiss mobile prompt")
	}
	if err := e.ele.Login(ctx, e.username); err != nil {
		return errors.Wrap(err, "failed to login to the Element app")
	}

	roomName := fmt.Sprintf("Power test %d", time.Now().Nanosecond())
	if err := e.ele.CreateRoom(roomName)(ctx); err != nil {
		return errors.Wrap(err, "failed to create room")
	}
	e.roomName = roomName
	return nil
}

// CleanUp leaves the current room and signs out.
func (e *Element) CleanUp(ctx context.Context) error {
	return uiauto.NamedCombine("leave room and sign out",
		// Leaving the room as the last member would trigger an automatic room deletion.
		e.ele.LeaveCurrentRoom(),
		e.ele.SignOut(),
	)(ctx)
}

// SendMessages sends the text and emoji messages to the room.
// It put time information into message to differentiate it from the previous.
func (e *Element) SendMessages(ctx context.Context) error {
	timeInfo := time.Now().Nanosecond()
	textMessage := fmt.Sprintf("text test %d", timeInfo)
	emojiMessage := fmt.Sprintf("emoji test %d", timeInfo)
	return uiauto.NamedCombine("send text and emoji messages",
		e.ele.SendTextMessage(textMessage),
		e.ele.SendEmojiMessage(emojiMessage, element.Smile, element.Angry, element.Sad),
	)(ctx)
}

// RunExtraOperations runs the following operations:
// 1. Rename the room as "new room" to test the renaming functionality.
// 2. Rename the room back to the unique room name.
// 3. Go to "Explore room" page and search for the test room.
// 4. Go back to the original room.
// The rename in step 2 is required for correctly joining the original room in step 4.
func (e *Element) RunExtraOperations(ctx context.Context) error {
	return uiauto.NamedCombine("run extra operations",
		e.ele.RenameCurrentRoom("new room"),
		e.ele.RenameCurrentRoom(e.roomName),
		e.ele.SearchPublicRoom(testRoomID, testRoomName),
		e.ele.JoinRoom(e.roomName),
	)(ctx)
}

var _ SocialApp = (*Element)(nil)
