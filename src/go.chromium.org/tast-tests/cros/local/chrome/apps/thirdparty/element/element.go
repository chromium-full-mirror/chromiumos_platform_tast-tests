// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package element contains UI functions and common libraries on Element App.
package element

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// Emoji represents the type of emoji in the Element app.
type Emoji string

// These are emoji options in the Element app.
const (
	Smile Emoji = "smile"
	Angry Emoji = "angry"
	Sad   Emoji = "sad"
)

const (
	// ElementPackage is the Android App package name for Element app.
	ElementPackage     = "im.vector.app"
	elementIDPrefix    = ElementPackage + ":id/"
	createChatButtonID = elementIDPrefix + "newLayoutCreateChatButton"
	messageFieldID     = elementIDPrefix + "composerEditText"
	actionTitleID      = elementIDPrefix + "actionTitle"
	textInputFieldID   = elementIDPrefix + "formTextInputTextInputEditText"
	searchFieldID      = elementIDPrefix + "search_src_text"
	roomNameID         = elementIDPrefix + "roomNameView"

	buttonClass      = "android.widget.Button"
	textClass        = "android.widget.TextView"
	imageButtonClass = "android.widget.ImageButton"

	retryTimes = 3
	// If the network is unstable, account synchronization may take a long time.
	syncTimeout      = 5 * time.Minute
	loadTimeout      = time.Minute
	longLoadTimeout  = 2 * time.Minute
	longUITimeout    = 45 * time.Second
	defaultUITimeout = 15 * time.Second
	shortUITimeout   = 5 * time.Second
	swipeDuration    = time.Second
)

// Element represents a type of Element instance.
type Element struct {
	tconn  *chrome.TestConn
	ui     *uiauto.Context
	kb     *input.KeyboardEventWriter
	a      *arc.ARC
	d      *ui.Device
	apkURL string
}

// New returns a new Element object.
func New(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *ui.Device, apkURL string) *Element {
	return &Element{
		tconn:  tconn,
		ui:     uiauto.New(tconn),
		kb:     kb,
		a:      a,
		d:      d,
		apkURL: apkURL,
	}
}

// Install installs or updates the Element app through Play Store.
// The app version will be logged after the installation.
func (e *Element) Install(ctx context.Context) error {
	if e.apkURL == "" {
		return util.InstallApp(ctx, e.tconn, e.a, e.d, ElementPackage)
	}
	return util.InstallAppFromAPKURL(ctx, e.a, e.d, ElementPackage, e.apkURL)
}

// Uninstall uninstalls the Element app if it is installed.
// This function does nothing if the app is initially uninstalled.
func (e *Element) Uninstall(ctx context.Context) error {
	return util.UninstallApp(ctx, e.a, ElementPackage)
}

// Launch launches the Element app.
func (e *Element) Launch(ctx context.Context, launchTimeout time.Duration) error {
	return util.LaunchApp(ctx, e.tconn, e.kb, apps.Element)
}

// Close closes the Element app.
func (e *Element) Close(ctx context.Context) error {
	return util.CloseApp(ctx, e.tconn, ElementPackage)
}

// Login creates a new account and logs in to the local server.
func (e *Element) Login(server string) uiauto.Action {
	notNowButton := e.d.Object(ui.Text("NOT NOW"), ui.ResourceID(elementIDPrefix+"later"))
	return uiauto.NamedCombine("create account and login",
		e.createAccount(server),
		// The |notNowButton| might take more time to appear on low-end devices.
		apputil.ClickIfExist(notNowButton, longUITimeout),
	)
}

// createAccount creates a new account on the local server.
func (e *Element) createAccount(server string) uiauto.Action {
	editButton := e.d.Object(ui.Text("EDIT"), ui.ResourceID(elementIDPrefix+"editServerButton"))
	serverSubmitButton := e.d.Object(ui.Text("NEXT"), ui.ResourceID(elementIDPrefix+"chooseServerSubmit"))
	editServer := uiauto.NamedCombine("edit server to "+server,
		apputil.FindAndClick(editButton, defaultUITimeout),
		e.typeText(server, ui.ClassName("android.widget.EditText"), ui.PackageName(ElementPackage)),
		apputil.FindAndClick(serverSubmitButton, defaultUITimeout),
	)

	// The database will not be reset between Tast retries.
	// Add a timestamp to the username to ensure it is unique.
	userName := fmt.Sprintf("powerTestUser_%d", time.Now().UnixNano())
	createAccountButton := e.d.Object(ui.Text("CREATE ACCOUNT"), ui.ResourceID(elementIDPrefix+"loginSplashSubmit"))
	skipButton := e.d.Object(ui.TextContains("Skip"), ui.ResourceID(elementIDPrefix+"useCaseSkip"))
	accountSubmitButton := e.d.Object(ui.Text("NEXT"), ui.ResourceID(elementIDPrefix+"createAccountSubmit"), ui.Enabled(true))
	// There are 2 possible texts of the no save button, "Not now" and "Never".
	// Use resource ID to correctly click the object.
	noSaveButton := e.d.Object(ui.ResourceID("android:id/autofill_save_no"))
	takeMeHomeButton := e.d.Object(ui.Text("TAKE ME HOME"), ui.ResourceID(elementIDPrefix+"accountCreatedTakeMeHome"))
	return uiauto.NamedCombine("create an account to Element app",
		apputil.FindAndClick(createAccountButton, defaultUITimeout),
		apputil.FindAndClick(skipButton, defaultUITimeout),
		editServer,
		e.typeText(userName, ui.ResourceID(elementIDPrefix+"createAccountEditText")),
		e.typeText("powerTestPassword", ui.ResourceID(elementIDPrefix+"createAccountPassword")),
		apputil.FindAndClick(accountSubmitButton, defaultUITimeout),
		apputil.ClickIfExist(noSaveButton, shortUITimeout),
		apputil.FindAndClick(takeMeHomeButton, defaultUITimeout),
	)
}

// SignOut signs out from Element app.
func (e *Element) SignOut() uiauto.Action {
	const (
		androidTitleID   = "android:id/title"
		generalTextTitle = "General"
	)
	profilePicture := e.d.Object(ui.DescriptionContains("Profile picture"), ui.ResourceID(elementIDPrefix+"avatar"))
	generalTitle := e.d.Object(ui.Text(generalTextTitle), ui.ResourceID(androidTitleID))
	openGeneralSettings := uiauto.NamedCombine("open general settings",
		e.navigateUpToObject(profilePicture),
		apputil.FindAndClick(profilePicture, defaultUITimeout),
		apputil.FindAndClick(generalTitle, defaultUITimeout),
	)

	generalText := e.d.Object(ui.Text(generalTextTitle), ui.ClassName(textClass))
	integrationsTitle := e.d.Object(ui.Text("Integrations"), ui.ResourceID(androidTitleID))
	advancedTitle := e.d.Object(ui.Text("Advanced"), ui.ResourceID(androidTitleID))
	// The "Sign out" text and the "Sing out" button share the same attributes.
	// Use ui.Instance(1) to click the "Sing out" button as the second object.
	signOutTitle := e.d.Object(ui.Text("Sign out"), ui.ResourceID(androidTitleID), ui.Instance(1))
	// The |generalText| is the heading of the page.
	// Swipe from bottom objects to the heading to show the |signOutTitle|.
	swipeToShowSignOut := uiauto.NamedCombine("swipe to show sign out title",
		e.swipeToShowObject(integrationsTitle, generalText, advancedTitle, swipeDuration),
		// The |signOutTitle| might not appear with single swipe on some DUTs.
		// Swipe again to ensure the |signOutTitle| is shown.
		e.swipeToShowObject(advancedTitle, generalText, signOutTitle, swipeDuration),
	)

	myEncryptedMessagesText := e.d.Object(ui.TextContains("my encrypted messages"), ui.ClassName(textClass))
	signOutButton := e.d.Object(ui.Text("SIGN OUT"), ui.ClassName(buttonClass))
	clickSignOutButton := uiauto.NamedCombine("click sign out button",
		apputil.FindAndClick(signOutTitle, defaultUITimeout),
		apputil.FindAndClick(myEncryptedMessagesText, defaultUITimeout),
		apputil.FindAndClick(signOutButton, defaultUITimeout),
	)
	return uiauto.NamedCombine("sign out from the Element app",
		openGeneralSettings,
		swipeToShowSignOut,
		clickSignOutButton,
	)
}

// CreatePublicRoom creates a new public room in the Element app.
func (e *Element) CreatePublicRoom(ctx context.Context, roomName, roomID string) error {
	return e.createRoom(ctx, roomName, roomID, true /* isPublic */)
}

// CreatePrivateRoom creates a new private room in the Element app.
func (e *Element) CreatePrivateRoom(ctx context.Context, roomName string) error {
	return e.createRoom(ctx, roomName, "" /* roomID */, false /* isPublic */)
}

func (e *Element) createRoom(ctx context.Context, roomName, roomID string, isPublic bool) error {
	const settingTextID = elementIDPrefix + "settings_section_title_text"
	createRoomButton := e.d.Object(ui.Description("Create a new conversation or room"), ui.ResourceID(createChatButtonID))
	createRoomText := e.d.Object(ui.Text("Create Room"), ui.ResourceID(elementIDPrefix+"create_room"))
	enterRoomCreationPage := uiauto.NamedCombine("enter room creation page",
		e.ui.WithTimeout(longUITimeout).RetryUntil(
			apputil.FindAndClick(createRoomButton, defaultUITimeout),
			apputil.WaitForExists(createRoomText, shortUITimeout),
		),
		apputil.FindAndClick(createRoomText, defaultUITimeout),
	)

	roomAccessText := e.d.Object(ui.Text("Room access"), ui.ResourceID(settingTextID))
	roomNameFieldWithText := e.d.Object(ui.Text(roomName), ui.ResourceID(textInputFieldID))
	createButton := e.d.Object(ui.Text("CREATE"), ui.ResourceID(elementIDPrefix+"form_submit_button"))
	if err := uiauto.NamedCombine("set room name as "+roomName,
		enterRoomCreationPage,
		e.typeText(roomName, ui.ResourceID(textInputFieldID)),
		e.swipeToShowObject(roomAccessText, roomNameFieldWithText, createButton, swipeDuration),
	)(ctx); err != nil {
		return err
	}

	if isPublic {
		// Private is the default access level.
		privateActionTitle := e.d.Object(ui.Text("Private"), ui.ResourceID(actionTitleID))
		publicActionTitle := e.d.Object(ui.Text("Public"), ui.ResourceID(actionTitleID))
		roomSettingText := e.d.Object(ui.Text("Room settings"), ui.ResourceID(settingTextID))
		if err := uiauto.NamedCombine("set room access as public",
			apputil.FindAndClick(privateActionTitle, defaultUITimeout),
			apputil.FindAndClick(publicActionTitle, defaultUITimeout),
			e.swipeToShowObject(roomSettingText, roomAccessText, createButton, swipeDuration),
			e.typeText(roomID, ui.TextContains("New published address"), ui.ResourceID(textInputFieldID)),
		)(ctx); err != nil {
			return err
		}
	}

	roomTitle := e.d.Object(ui.Text(roomName), ui.ClassName(textClass))
	return uiauto.NamedCombine("create room",
		apputil.FindAndClick(createButton, defaultUITimeout),
		uiauto.NamedAction("wait for room title "+roomName,
			apputil.WaitForExists(roomTitle, longLoadTimeout)),
	)(ctx)
}

// LeaveRoom leaves the current room.
// The room will be deleted after seven days after the last member leaves.
// This function assumed the user is inside the corresponding room.
func (e *Element) LeaveRoom(roomName string) uiauto.Action {
	roomNameText := e.d.Object(ui.ResourceID(roomNameID), ui.Text(roomName))
	profileAppBar := e.d.Object(ui.ResourceID(elementIDPrefix + "matrixProfileAppBarLayout"))
	actionBarRoot := e.d.Object(ui.ResourceID(elementIDPrefix + "action_bar_root"))
	leaveRoomButton := e.d.Object(ui.ResourceID(actionTitleID), ui.Text("Leave Room"))
	leaveButton := e.d.Object(ui.Text("LEAVE"), ui.ClassName(buttonClass))
	return uiauto.NamedCombine("leave room "+roomName,
		e.openSettingsPage(),
		e.swipeToShowObject(actionBarRoot, profileAppBar, leaveRoomButton, swipeDuration),
		apputil.FindAndClick(leaveRoomButton, defaultUITimeout),
		apputil.FindAndClick(leaveButton, defaultUITimeout),
		apputil.WaitUntilGone(leaveButton, defaultUITimeout),
		apputil.WaitUntilGone(roomNameText, defaultUITimeout),
	)
}

// SetUIDevice associates the given the UI device to the Element object.
func (e *Element) SetUIDevice(d *ui.Device) {
	e.d = d
}

// SendTextMessage sends a text message to the current room.
func (e *Element) SendTextMessage(message string) uiauto.Action {
	return uiauto.NamedCombine("send text message",
		e.typeText(message, ui.ResourceID(messageFieldID)),
		e.sendMessageAndWait(message),
	)
}

// SendEmojiMessage sends a message contains emojis to the current room.
func (e *Element) SendEmojiMessage(textMessage string, emojis ...Emoji) uiauto.Action {
	sendEmojiActions := []uiauto.Action{
		e.typeText(textMessage, ui.ResourceID(messageFieldID)),
	}
	for _, emoji := range emojis {
		sendEmojiActions = append(sendEmojiActions, e.addEmoji(emoji))
	}
	sendEmojiActions = append(sendEmojiActions, e.sendMessageAndWait(textMessage))
	return uiauto.Retry(retryTimes, uiauto.NamedCombine("send emoji message", sendEmojiActions...))
}

// addEmoji types emoji text and chooses the last emoji from the recommended list.
func (e *Element) addEmoji(emoji Emoji) uiauto.Action {
	emojiMessage := fmt.Sprintf(", :%s", emoji)
	// The recommended emoji list is not captured by ARC UI or uiautomator,
	// so it cannot be interacted with UI device or uiautomator.
	// The list would appear above the message field,
	// clicking at the location above the message field to add the emoji.
	// The emoji clicked might be different in different DUTs.
	clickEmoji := func(ctx context.Context) error {
		messageField := e.d.Object(ui.ResourceID(messageFieldID))
		messageFieldBound, err := messageField.GetBounds(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get message field bound")
		}
		x := messageFieldBound.CenterX()
		y := messageFieldBound.Top - messageFieldBound.Height/2
		return e.d.Click(ctx, x, y)
	}
	messageFieldWithText := e.d.Object(ui.TextContains(string(emoji)), ui.ResourceID(messageFieldID))
	return uiauto.NamedCombine(fmt.Sprintf("add emoji %s", emoji),
		e.kb.TypeAction(emojiMessage),
		apputil.WaitForExists(messageFieldWithText, defaultUITimeout),
		clickEmoji,
	)
}

// sendMessageAndWait clicks the send button and wait for the message sent image to appear.
func (e *Element) sendMessageAndWait(expectedMessage string) uiauto.Action {
	sendButton := e.d.Object(ui.Description("Send"), ui.ResourceID(elementIDPrefix+"sendButton"))
	expectedMessageText := e.d.Object(ui.TextContains(expectedMessage), ui.ResourceID(elementIDPrefix+"messageTextView"))
	messageSentImage := e.d.Object(ui.ResourceID(elementIDPrefix+"messageSendStateImageView"), ui.Description("Sent"))
	return uiauto.NamedCombine("send message and wait for expected message",
		apputil.FindAndClick(sendButton, defaultUITimeout),
		apputil.WaitForExists(expectedMessageText, defaultUITimeout),
		// On low-end machine, the message might take more time to be sent.
		apputil.WaitForExists(messageSentImage, longLoadTimeout),
	)
}

// RenameCurrentRoom renames the current room.
func (e *Element) RenameCurrentRoom(newRoomName string) uiauto.Action {
	moreOptionsButton := e.d.Object(ui.PackageName(ElementPackage), ui.Description("More options"), ui.Clickable(true))
	return uiauto.NamedCombine("rename current room as "+newRoomName,
		e.navigateUpToObject(moreOptionsButton),
		e.openSettingsPage(),
		// Sometimes the save button does not appear.
		// Retry to ensure the room is renamed.
		uiauto.Retry(retryTimes, e.setRoomNameAndSave(newRoomName)),
	)
}

func (e *Element) openSettingsPage() uiauto.Action {
	moreOptionsButton := e.d.Object(ui.PackageName(ElementPackage), ui.Description("More options"), ui.Clickable(true))
	optionTitle := e.d.Object(ui.Text("Settings"), ui.ResourceID(elementIDPrefix+"title"))
	roomSettingsTitle := e.d.Object(ui.Text("Room settings"), ui.ResourceID(actionTitleID))
	return uiauto.NamedCombine("open Settings page",
		apputil.FindAndClick(moreOptionsButton, defaultUITimeout),
		apputil.FindAndClick(optionTitle, defaultUITimeout),
		apputil.WaitForExists(roomSettingsTitle, defaultUITimeout),
	)
}

func (e *Element) setRoomNameAndSave(newRoomName string) uiauto.Action {
	return func(ctx context.Context) error {
		navigateUpButton := e.d.Object(ui.PackageName(ElementPackage), ui.Description("Navigate up"), ui.ClassName(imageButtonClass))
		discardChangeButton := e.d.Object(ui.Text("DISCARD CHANGES"), ui.ClassName(buttonClass))
		returnToSettingsPage := uiauto.Combine("return to Settings page",
			apputil.FindAndClick(navigateUpButton, defaultUITimeout),
			apputil.ClickIfExist(discardChangeButton, defaultUITimeout),
		)

		roomSettingsTitle := e.d.Object(ui.Text("Room settings"), ui.ResourceID(actionTitleID))
		toolbarTitle := e.d.Object(ui.ResourceID(elementIDPrefix + "roomSettingsToolbarTitleView"))
		if err := uiauto.NamedCombine("enter room settings",
			// Return to the Settings page when retrying.
			uiauto.IfFailThen(
				roomSettingsTitle.Exists,
				returnToSettingsPage,
			),
			apputil.FindAndClick(roomSettingsTitle, defaultUITimeout),
			apputil.WaitForExists(toolbarTitle, defaultUITimeout),
		)(ctx); err != nil {
			return err
		}

		// The room might be renamed in previous retries.
		// Check the current name before setting the new name.
		currentRoomName, err := toolbarTitle.GetText(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to get current room name")
		}
		if currentRoomName == newRoomName {
			testing.ContextLog(ctx, "Room name has been renamed to "+newRoomName)
			return nil
		}

		saveButton := e.d.Object(ui.Text("SAVE"), ui.ResourceID(elementIDPrefix+"roomSettingsSaveAction"))
		newToolbarTitle := e.d.Object(ui.Text(newRoomName), ui.ResourceID(elementIDPrefix+"roomSettingsToolbarTitleView"))
		return uiauto.NamedCombine("set room name and save",
			e.typeText(newRoomName, ui.ResourceID(textInputFieldID)),
			apputil.FindAndClick(saveButton, defaultUITimeout),
			apputil.WaitUntilGone(saveButton, longUITimeout),
			apputil.WaitForExists(newToolbarTitle, defaultUITimeout),
		)(ctx)
	}
}

// SearchPublicRoom searches the existing public room with the ID and the name.
func (e *Element) SearchPublicRoom(roomID, roomName string) uiauto.Action {
	roomID = "#" + roomID
	createRoomButton := e.d.Object(ui.Description("Create a new conversation or room"), ui.ResourceID(createChatButtonID))
	exploreRoomsText := e.d.Object(ui.Text("Explore Rooms"), ui.ResourceID(elementIDPrefix+"explore_rooms"))
	publicRoomText := fmt.Sprintf("(%s|%s)", roomID, roomName)
	publicRoom := e.d.Object(ui.TextMatches(publicRoomText), ui.ClassName(textClass))
	return uiauto.NamedCombine("explore public room with ID "+roomID,
		e.navigateUpToObject(createRoomButton),
		e.ui.WithTimeout(longUITimeout).RetryUntil(
			apputil.FindAndClick(createRoomButton, defaultUITimeout),
			apputil.WaitForExists(exploreRoomsText, shortUITimeout),
		),
		apputil.FindAndClick(exploreRoomsText, defaultUITimeout),
		e.typeText(roomID, ui.ResourceID(searchFieldID)),
		uiauto.NamedAction("wait for public room "+roomName,
			apputil.WaitForExists(publicRoom, longUITimeout)),
	)
}

// JoinRoom uses |roomFilter| to find the given room from the room list and joins the room.
func (e *Element) JoinRoom(roomName string) uiauto.Action {
	roomFilter := e.d.Object(ui.Description("Filter room names"), ui.ResourceID(elementIDPrefix+"menu_home_filter"))
	room := e.d.Object(ui.Text(roomName), ui.ResourceID(roomNameID))
	roomTitle := e.d.Object(ui.Text(roomName), ui.ResourceID(elementIDPrefix+"roomToolbarTitleView"))
	enterRoom := uiauto.NamedCombine("search room and join",
		e.typeText(roomName, ui.ResourceID(searchFieldID)),
		apputil.FindAndClick(room, defaultUITimeout),
		apputil.WaitUntilGone(room, defaultUITimeout),
		apputil.WaitForExists(roomTitle, defaultUITimeout),
	)
	return uiauto.NamedCombine(fmt.Sprintf("join %q room from home page", roomName),
		e.navigateUpToObject(roomFilter),
		apputil.FindAndClick(roomFilter, defaultUITimeout),
		uiauto.Retry(retryTimes, enterRoom),
	)
}

// CheckUserInRoom checks if the user is currently in the specified room.
func (e *Element) CheckUserInRoom(roomName string) uiauto.Action {
	roomTitle := e.d.Object(ui.Text(roomName), ui.ResourceID(elementIDPrefix+"roomToolbarTitleView"))
	return uiauto.NamedAction(fmt.Sprintf("check in %q room", roomName),
		apputil.WaitForExists(roomTitle, defaultUITimeout),
	)
}

// typeText types text in the text field with the given selectors.
func (e *Element) typeText(text string, selectors ...ui.SelectorOption) uiauto.Action {
	textField := e.d.Object(selectors...)
	textFieldFocused := e.d.Object(append(selectors, ui.Focused(true))...)
	textFieldWithText := e.d.Object(append(selectors, ui.TextContains(text))...)
	return uiauto.NamedCombine(fmt.Sprintf("type text %s", text),
		apputil.FindAndClick(textField, defaultUITimeout),
		apputil.WaitForExists(textFieldFocused, defaultUITimeout),
		uiauto.Retry(retryTimes, uiauto.Combine("input text",
			e.kb.AccelAction("Ctrl+A"),
			e.kb.TypeAction(text),
			apputil.WaitForExists(textFieldWithText, shortUITimeout),
		)),
	)
}

// navigateUpToObject keeps clicking the "Navigate up" button to go back
// to the previous page until the |expectedObject| appears.
func (e *Element) navigateUpToObject(expectedObject *ui.Object) uiauto.Action {
	navigateUpButton := e.d.Object(ui.PackageName(ElementPackage), ui.Description("Navigate up"), ui.ClassName(imageButtonClass))
	return uiauto.NamedCombine(fmt.Sprintf("navigate up to %v", expectedObject),
		uiauto.IfFailThen(
			expectedObject.Exists,
			e.ui.WithTimeout(longUITimeout).RetryUntil(
				apputil.FindAndClick(navigateUpButton, defaultUITimeout),
				apputil.WaitForExists(expectedObject, shortUITimeout),
			),
		),
	)
}

// swipeToShowObject swipes from |startObject| to |endObject| until |expectedObject| appears.
func (e *Element) swipeToShowObject(startObject, endObject, expectedObject *ui.Object, swipeDuration time.Duration) uiauto.Action {
	return func(ctx context.Context) error {
		startObjectBound, err := startObject.GetBounds(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to get bounds of %v", startObject)
		}
		startPoint := startObjectBound.CenterPoint()

		endObjectBound, err := endObject.GetBounds(ctx)
		if err != nil {
			return errors.Wrapf(err, "failed to get bounds of %v", endObject)
		}
		endPoint := endObjectBound.CenterPoint()

		// Use DragAndDrop to simulate the swipe action.
		return e.ui.WithTimeout(longUITimeout).RetryUntil(
			apputil.DragAndDrop(e.a, startPoint, endPoint, swipeDuration),
			apputil.WaitForExists(expectedObject, shortUITimeout),
		)(ctx)
	}
}

// ReverseTCPForLocalServer reverses the TCP port to connect to the local server.
func (e *Element) ReverseTCPForLocalServer(ctx context.Context) (string, func(context.Context) error, error) {
	port, err := e.a.ReverseTCP(ctx, power.TuwunelServerDefaultPort)
	if err != nil {
		return "", nil, errors.Wrap(err, "failed to get reverse TCP port")
	}
	server := fmt.Sprintf("http://localhost:%d", port)
	return server, func(ctx context.Context) error { return e.a.RemoveReverseTCP(ctx, port) }, nil
}
