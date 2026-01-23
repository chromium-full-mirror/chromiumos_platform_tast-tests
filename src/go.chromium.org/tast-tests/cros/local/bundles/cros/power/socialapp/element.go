// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package socialapp

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/thirdparty/element"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	testRoomID   = "#power_test:matrix.org"
	testRoomName = "Power test room"

	// There are different APK urls according to the system architecture.
	elementArmAPKVarName    = "power.element_arm_apk_url"
	elementArm64APKVarName  = "power.element_arm64_apk_url"
	elementX86APKVarName    = "power.element_x86_apk_url"
	elementX86_64APKVarName = "power.element_x86_64_apk_url"
	// defaultApkURLBase is the URL base for the Element APKs with the
	// default version 1.6.6 on the github.
	// The full links can be found under the Assets
	// on https://github.com/vector-im/element-android/releases/tag/v1.6.6.
	defaultApkURLBase = "https://github.com/element-hq/element-android/releases/download/v1.6.32/vector-gplay-"
)

var (
	// ElementAppName represents the name of the Element app.
	ElementAppName AppName = apps.Element.Name

	// ElementApkURLVars contains all variable names of the element apk.
	ElementApkURLVars = []string{
		elementArmAPKVarName,
		elementArm64APKVarName,
		elementX86APKVarName,
		elementX86_64APKVarName,
	}

	// Default to install the apk with version "1.6.6".
	defaultElementApkURLs = map[string]string{
		elementArmAPKVarName:    defaultApkURLBase + "armeabi-v7a-release-signed.apk",
		elementArm64APKVarName:  defaultApkURLBase + "arm64-v8a-release-signed.apk",
		elementX86APKVarName:    defaultApkURLBase + "x86-release-signed.apk",
		elementX86_64APKVarName: defaultApkURLBase + "x86_64-release-signed.apk",
	}
)

// Element implements the SocialApp interface with the Element app.
type Element struct {
	ele      *element.Element
	tconn    *chrome.TestConn
	creds    credconfig.Creds
	roomName string
}

// ParseElementAPKURL returns the element APK URL corresponding to the DUT architecture.
func ParseElementAPKURL(ctx context.Context, testCaseVar func(string) (string, bool)) (string, error) {
	_, arch, err := sysutil.KernelVersionAndArch()
	if err != nil {
		return "", errors.Wrap(err, "failed to get system arch")
	}

	var varName string
	switch arch {
	case "armv7l", "armv8l":
		varName = elementArmAPKVarName
	case "aarch64":
		varName = elementArm64APKVarName
	case "i686":
		varName = elementX86APKVarName
	case "x86_64":
		varName = elementX86_64APKVarName
	default:
		return "", errors.Errorf("unsupported arch: %s", arch)
	}

	if url, ok := testCaseVar(varName); ok {
		testing.ContextLog(ctx, "Element APK URL parsed from runtime variable: ", url)
		return url, nil
	}
	// Return default APK URL if the runtime variable is not set.
	url := defaultElementApkURLs[varName]
	testing.ContextLogf(ctx, "Runtime variable %q is not set, using default value: %s", varName, url)
	return url, nil
}

// NewElement returns a new Element object.
func NewElement(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *ui.Device, cr *chrome.Chrome, apkURL string) *Element {
	return &Element{
		ele:   element.New(tconn, kb, a, d, cr, apkURL),
		tconn: tconn,
		creds: cr.Creds(),
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
	if err := e.ele.Login(ctx, e.creds); err != nil {
		return errors.Wrap(err, "failed to login to the Element app")
	}

	roomName := fmt.Sprintf("Power test %d", time.Now().Nanosecond())
	if err := e.ele.CreateRoom(roomName)(ctx); err != nil {
		return errors.Wrap(err, "failed to create room")
	}
	e.roomName = roomName
	return nil
}

// CleanUp leaves the created room and signs out.
func (e *Element) CleanUp(ctx context.Context) error {
	return uiauto.NamedCombine("leave room and sign out",
		// Ensure the user is inside the room before leaving.
		e.EnsureInRoom(),
		// Leaving the room as the last member would trigger an automatic room deletion.
		e.ele.LeaveRoom(e.roomName),
		e.ele.SignOut(),
	)(ctx)
}

// SetUIDevice associates the given the UI device to the Element object.
func (e *Element) SetUIDevice(d *ui.Device) {
	e.ele.SetUIDevice(d)
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

// EnsureInRoom checks if the user is in the room, and attempts to rejoin if not.
func (e *Element) EnsureInRoom() uiauto.Action {
	return uiauto.NamedAction("ensure in room",
		uiauto.IfFailThen(e.ele.CheckUserInRoom(e.roomName), e.ele.JoinRoom(e.roomName)),
	)
}

var _ SocialApp = (*Element)(nil)
