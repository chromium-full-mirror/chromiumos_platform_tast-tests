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
	// There are different APK urls according to the system architecture.
	elementArmAPKVarName    = "power.element_arm_apk_url"
	elementArm64APKVarName  = "power.element_arm64_apk_url"
	elementX86APKVarName    = "power.element_x86_apk_url"
	elementX86_64APKVarName = "power.element_x86_64_apk_url"
	// defaultApkURLBase is the URL base for the Element APKs with the
	// default version 1.6.50 on the google cloud bucket.
	// The official apks can be found under the Assets
	// on https://github.com/vector-im/element-android/releases/tag/v1.6.50.
	defaultApkURLBase = "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/social-app/v1.6.50/vector-gplay-"
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

	// Default to install the apk with version "1.6.50".
	defaultElementApkURLs = map[string]string{
		elementArmAPKVarName:    defaultApkURLBase + "armeabi-v7a-release-signed.apk",
		elementArm64APKVarName:  defaultApkURLBase + "arm64-v8a-release-signed.apk",
		elementX86APKVarName:    defaultApkURLBase + "x86-release-signed.apk",
		elementX86_64APKVarName: defaultApkURLBase + "x86_64-release-signed.apk",
	}
)

// Element implements the SocialApp interface with the Element app.
type Element struct {
	ele             *element.Element
	tconn           *chrome.TestConn
	creds           credconfig.Creds
	tcpCleanup      func(context.Context) error
	privateRoomName string
	publicRoomID    string
	publicRoomName  string
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

// NewElement sets up a reverse TCP proxy to the local server
// and returns a new Element object.
func NewElement(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *ui.Device, creds credconfig.Creds, apkURL string) *Element {
	return &Element{
		ele:        element.New(tconn, kb, a, d, apkURL),
		tconn:      tconn,
		creds:      creds,
		tcpCleanup: func(context.Context) error { return nil },
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
	server, tcpCleanup, err := e.ele.ReverseTCPForLocalServer(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to set up reverse TCP proxy")
	}
	e.tcpCleanup = tcpCleanup
	if err := e.ele.Login(server)(ctx); err != nil {
		return errors.Wrap(err, "failed to login to the Element app")
	}

	timeStamp := time.Now().UnixNano()
	publicRoomID := fmt.Sprintf("public_room_%d", timeStamp)
	publicRoomName := fmt.Sprintf("Public room %d", timeStamp)
	// Create a public room for the SearchPublicRoom action.
	if err := e.ele.CreatePublicRoom(ctx, publicRoomName, publicRoomID); err != nil {
		return errors.Wrap(err, "failed to create public room")
	}
	if err := e.ele.LeaveRoom(publicRoomName)(ctx); err != nil {
		return errors.Wrap(err, "failed to leave public room")
	}
	e.publicRoomID = publicRoomID
	e.publicRoomName = publicRoomName

	privateRoomName := fmt.Sprintf("Power test %d", timeStamp)
	if err := e.ele.CreatePrivateRoom(ctx, privateRoomName); err != nil {
		return errors.Wrap(err, "failed to create room")
	}
	e.privateRoomName = privateRoomName
	return nil
}

// CleanUp signs out and removes reverse tcp.
func (e *Element) CleanUp(ctx context.Context) error {
	return uiauto.NamedCombine("sign out and remove reverse tcp",
		e.ele.SignOut(),
		e.tcpCleanup,
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
		e.ele.RenameCurrentRoom(e.privateRoomName),
		e.ele.SearchPublicRoom(e.publicRoomID, e.publicRoomName),
		e.ele.JoinRoom(e.privateRoomName),
	)(ctx)
}

// EnsureInRoom checks if the user is in the room, and attempts to rejoin if not.
func (e *Element) EnsureInRoom() uiauto.Action {
	return uiauto.NamedAction("ensure in room",
		uiauto.IfFailThen(
			e.ele.CheckUserInRoom(e.privateRoomName),
			e.ele.JoinRoom(e.privateRoomName),
		),
	)
}

var _ SocialApp = (*Element)(nil)
