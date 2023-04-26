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
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/power/util"
	"go.chromium.org/tast/core/errors"
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
	elementPackage     = "im.vector.app"
	elementIDPrefix    = elementPackage + ":id/"
	createChatButtonID = elementIDPrefix + "newLayoutCreateChatButton"
	messageFieldID     = elementIDPrefix + "composerEditText"
	actionTitleID      = elementIDPrefix + "actionTitle"
	roomNameFieldID    = elementIDPrefix + "formTextInputTextInputEditText"
	searchFieldID      = elementIDPrefix + "search_src_text"

	buttonClass = "android.widget.Button"
	textClass   = "android.widget.TextView"

	retryTimes       = 3
	longUITimeout    = 30 * time.Second
	defaultUITimeout = 15 * time.Second
	shortUITimeout   = 5 * time.Second
	swipeDuration    = time.Second
)

var (
	elementID      = apps.Element.ID
	elementAppName = apps.Element.Name
)

// Element represents a type of Element instance.
type Element struct {
	tconn *chrome.TestConn
	ui    *uiauto.Context
	kb    *input.KeyboardEventWriter
	a     *arc.ARC
	d     *ui.Device
}

// New returns a new Element object.
func New(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *ui.Device) *Element {
	return &Element{
		tconn: tconn,
		ui:    uiauto.New(tconn),
		kb:    kb,
		a:     a,
		d:     d,
	}
}

// Install installs or updates the Element app through Play Store.
// The app version will be logged after the installation.
func (e *Element) Install(ctx context.Context) error {
	return util.InstallApp(ctx, e.tconn, e.a, e.d, elementPackage)
}

// Uninstall uninstalls the Element app if it is installed.
// This function does nothing if the app is initially uninstalled.
func (e *Element) Uninstall(ctx context.Context) error {
	return util.UninstallApp(ctx, e.a, elementPackage)
}

// Launch launches the Element app.
func (e *Element) Launch(ctx context.Context, launchTimeout time.Duration) error {
	return util.LaunchApp(ctx, e.tconn, e.kb, apps.Element)
}

// Close closes the Element app.
func (e *Element) Close(ctx context.Context) error {
	return util.CloseApp(ctx, e.tconn, elementPackage)
}

// Login logs in to Element app with Google account.
// It will create an account if the account has not been created.
func (e *Element) Login(ctx context.Context, username string) error {
	haveAccountButton := e.d.Object(ui.Text("I ALREADY HAVE AN ACCOUNT"), ui.ResourceID(elementIDPrefix+"loginSplashAlreadyHaveAccount"))
	googleContinueButton := e.d.Object(ui.Text("Continue with Google"), ui.ClassName(buttonClass))
	if err := uiauto.NamedCombine("login to Element app",
		apputil.FindAndClick(haveAccountButton, defaultUITimeout),
		// It might take long time to show the login screen.
		// Use longUITimeout to click the googleContinueButton.
		apputil.FindAndClick(googleContinueButton, longUITimeout),
	)(ctx); err != nil {
		return err
	}

	userLink := nodewith.NameContaining(username).Role(role.Link)
	createAccountRootWebArea := nodewith.Name("Create your account").Role(role.RootWebArea)
	continueLink := nodewith.Name("Continue").Role(role.Link)
	if err := uiauto.NamedCombine("login with Google",
		e.waitForLoginWindowMaximized,
		e.ui.DoDefaultUntil(userLink,
			e.ui.WithTimeout(shortUITimeout).WaitUntilGone(userLink),
		),
		e.ui.WaitUntilAnyExists(createAccountRootWebArea, continueLink),
		uiauto.IfSuccessThen(
			e.ui.Exists(createAccountRootWebArea),
			e.createAccount(username),
		),
		e.ui.DoDefault(continueLink),
	)(ctx); err != nil {
		return err
	}

	const waitingStatusTextID = elementIDPrefix + "waitingStatusText"
	notNowButton := e.d.Object(ui.Text("NOT NOW"), ui.ResourceID(elementIDPrefix+"later"))
	createRoomButton := e.d.Object(ui.Description("Create a new conversation or room"), ui.ResourceID(createChatButtonID))
	waitingStatusText := e.d.Object(ui.ResourceID(waitingStatusTextID), ui.ClassName(textClass))
	return uiauto.NamedCombine("skip splash",
		apputil.ClickIfExist(notNowButton, defaultUITimeout),
		apputil.WaitForExists(createRoomButton, defaultUITimeout),
		// Wait for the app finishes syncing the account data with the server.
		// 1. If the account only joins a few rooms, the text would immediately disappear
		// after being shown, and the UI might fail to capture it.
		// 2. If the account joins many rooms, the data sync might take a long time to finish.
		uiauto.IfSuccessThen(
			apputil.WaitForExists(waitingStatusText, defaultUITimeout),
			apputil.WaitUntilGone(waitingStatusText, longUITimeout),
		),
		e.dismissEncryptionAlert,
	)(ctx)
}

// waitForLoginWindowMaximized activates and maximizes the login window.
func (e *Element) waitForLoginWindowMaximized(ctx context.Context) error {
	// The window title might be displayed in different languages,
	// and "Google" is the only common text in the title.
	// Wait for any window with "Google" in the title to find the login window.
	loginWindow, err := ash.WaitForAnyWindowWithTitle(ctx, e.tconn, "Google")
	if err != nil {
		return errors.Wrap(err, "failed to find the login window")
	}

	if err := loginWindow.ActivateWindow(ctx, e.tconn); err != nil {
		return errors.Wrap(err, "failed to activate the login window")
	}
	if err := ash.SetWindowStateAndWait(ctx, e.tconn, loginWindow.ID, ash.WindowStateMaximized); err != nil {
		return errors.Wrap(err, "failed to maximized the login window")
	}
	return nil
}

// createAccount creates a new Element account with Google account.
// It assumes the create account page is already opened.
func (e *Element) createAccount(username string) uiauto.Action {
	usernameField := nodewith.Name("Username (required)").Role(role.TextField)
	usernameText := nodewith.Name(username).Role(role.StaticText).Ancestor(usernameField)
	checkingText := nodewith.NameContaining("Checking").Role(role.StaticText)
	setUsername := uiauto.NamedCombine("set username as "+username,
		e.ui.DoDefaultUntil(usernameField,
			e.ui.WithTimeout(shortUITimeout).WaitUntilExists(usernameField.Focused()),
		),
		e.kb.TypeAction(username),
		e.ui.WaitUntilExists(usernameText),
		// The website would show "Checking if username is available ..."
		// when validating the username.
		// Wait for the website to finish the validation.
		e.ui.WaitUntilGone(checkingText),
	)

	agreeTermsCheckBox := nodewith.Name("I have read and agree to the terms and conditions.").Role(role.CheckBox)
	agreeTerms := uiauto.NamedAction("agree terms",
		e.ui.DoDefaultUntil(agreeTermsCheckBox,
			e.ui.WithTimeout(shortUITimeout).WaitUntilCheckedState(agreeTermsCheckBox, true),
		),
	)
	continueButton := nodewith.Name("Continue").Role(role.Button)
	continueLink := nodewith.Name("Continue").Role(role.Link)
	return uiauto.NamedCombine("create account",
		setUsername,
		e.ui.DoDefaultUntil(continueButton,
			e.ui.WithTimeout(shortUITimeout).WaitUntilExists(agreeTermsCheckBox),
		),
		agreeTerms,
		e.ui.DoDefaultUntil(continueButton,
			e.ui.WithTimeout(shortUITimeout).WaitUntilExists(continueLink),
		),
	)
}

// dismissEncryptionAlert dismisses the encrypted alert after logging in to the Element app.
func (e *Element) dismissEncryptionAlert(ctx context.Context) error {
	topRow, err := input.KeyboardTopRowLayout(ctx, e.kb)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard top row layout")
	}

	alert := e.d.Object(ui.ResourceID(elementIDPrefix + "llAlertBackground"))
	skipButton := e.d.Object(ui.Text("SKIP"), ui.ResourceID("android:id/button2"))
	dismissAlert := uiauto.NamedCombine("dismiss encryption alert",
		apputil.FindAndClick(alert, defaultUITimeout),
		apputil.WaitUntilGone(alert, defaultUITimeout),
		// Use keyboard to trigger back action on both clamshell and tablet devices.
		e.kb.AccelAction(topRow.BrowserBack),
		apputil.FindAndClick(skipButton, defaultUITimeout),
	)
	return uiauto.IfSuccessThen(
		apputil.WaitForExists(alert, defaultUITimeout),
		dismissAlert,
	)(ctx)
}

// SignOut signs out from Element app.
func (e *Element) SignOut() uiauto.Action {
	const (
		androidTitleID   = "android:id/title"
		generalTextTitle = "General"
	)
	openSettingsButton := e.d.Object(ui.Description("Open settings"), ui.ResourceID(elementIDPrefix+"avatar"))
	generalTitle := e.d.Object(ui.Text(generalTextTitle), ui.ResourceID(androidTitleID))
	openGeneralSettings := uiauto.NamedCombine("open general settings",
		e.navigateUpToObject(openSettingsButton),
		apputil.FindAndClick(openSettingsButton, defaultUITimeout),
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
		e.swipeFromObjectToObject(integrationsTitle, generalText, swipeDuration),
		// The |signOutTitle| might not appear with single swipe on some DUTs.
		// Swipe again to ensure the |signOutTitle| is shown.
		e.swipeFromObjectToObject(advancedTitle, generalText, swipeDuration),
		apputil.WaitForExists(signOutTitle, defaultUITimeout),
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

// CreateRoom creates a new room in the Element app.
func (e *Element) CreateRoom(roomName string) uiauto.Action {
	createRoomButton := e.d.Object(ui.Description("Create a new conversation or room"), ui.ResourceID(createChatButtonID))
	createRoomText := e.d.Object(ui.Text("Create Room"), ui.ResourceID(elementIDPrefix+"create_room"))
	enterRoomCreationPage := uiauto.NamedCombine("enter room creation page",
		apputil.FindAndClick(createRoomButton, defaultUITimeout),
		apputil.FindAndClick(createRoomText, defaultUITimeout),
	)

	roomSettingsText := e.d.Object(ui.Text("Room settings"), ui.ResourceID(elementIDPrefix+"settings_section_title_text"))
	roomNameFieldWithText := e.d.Object(ui.Text(roomName), ui.ResourceID(roomNameFieldID))
	createButton := e.d.Object(ui.Text("CREATE"), ui.ResourceID(elementIDPrefix+"form_submit_button"))
	roomTitle := e.d.Object(ui.Text(roomName), ui.ClassName(textClass))
	return uiauto.NamedCombine("create room",
		enterRoomCreationPage,
		e.typeText(roomNameFieldID, roomName),
		e.swipeFromObjectToObject(roomSettingsText, roomNameFieldWithText, swipeDuration),
		apputil.FindAndClick(createButton, defaultUITimeout),
		apputil.WaitForExists(roomTitle, defaultUITimeout),
	)
}

// LeaveCurrentRoom leaves the current room.
// The room will be deleted after seven days after the last member leaves.
func (e *Element) LeaveCurrentRoom() uiauto.Action {
	toolBar := e.d.Object(ui.ResourceID(elementIDPrefix + "includeRoomToolbar"))
	navigateUpButton := e.d.Object(ui.Description("Navigate up"), ui.ClassName("android.widget.ImageButton"))
	moreText := e.d.Object(ui.Text("More"), ui.ResourceID(elementIDPrefix+"itemProfileSectionView"))
	leaveRoomButton := e.d.Object(ui.Text("Leave Room"), ui.ResourceID(actionTitleID))
	leaveButton := e.d.Object(ui.Text("LEAVE"), ui.ClassName(buttonClass))
	return uiauto.NamedCombine("leave room",
		e.navigateUpToObject(toolBar),
		apputil.FindAndClick(toolBar, defaultUITimeout),
		e.swipeFromObjectToObject(moreText, navigateUpButton, swipeDuration),
		apputil.FindAndClick(leaveRoomButton, shortUITimeout),
		apputil.FindAndClick(leaveButton, defaultUITimeout),
		apputil.WaitUntilGone(toolBar, defaultUITimeout),
	)
}

// SendTextMessage sends a text message to the current room.
func (e *Element) SendTextMessage(message string) uiauto.Action {
	return uiauto.NamedCombine("send text message",
		e.typeText(messageFieldID, message),
		e.sendMessageAndWait(message),
	)
}

// SendEmojiMessage sends a message contains emojis to the current room.
func (e *Element) SendEmojiMessage(textMessage string, emojis ...Emoji) uiauto.Action {
	sendEmojiActions := []uiauto.Action{
		e.typeText(messageFieldID, textMessage),
	}
	for _, emoji := range emojis {
		sendEmojiActions = append(sendEmojiActions, e.addEmoji(emoji))
	}
	sendEmojiActions = append(sendEmojiActions, e.sendMessageAndWait(textMessage))
	return uiauto.NamedCombine("send emoji message", sendEmojiActions...)
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

// sendMessageAndWait clicks the send button and wait for the expected message to appear.
func (e *Element) sendMessageAndWait(expectedMessage string) uiauto.Action {
	sendButton := e.d.Object(ui.Description("Send"), ui.ResourceID(elementIDPrefix+"sendButton"))
	expectedMessageText := e.d.Object(ui.TextContains(expectedMessage), ui.ResourceID(elementIDPrefix+"messageTextView"))
	return uiauto.NamedCombine("send message and wait for expected message",
		apputil.FindAndClick(sendButton, defaultUITimeout),
		apputil.WaitForExists(expectedMessageText, defaultUITimeout),
	)
}

// RenameCurrentRoom renames the current room.
func (e *Element) RenameCurrentRoom(newRoomName string) uiauto.Action {
	moreOptionsButton := e.d.Object(ui.Description("More options"), ui.Clickable(true))
	optionTitle := e.d.Object(ui.Text("Settings"), ui.ResourceID(elementIDPrefix+"title"))
	roomSettingsTitle := e.d.Object(ui.Text("Room settings"), ui.ResourceID(actionTitleID))
	openRoomSettings := uiauto.NamedCombine("open room settings",
		apputil.FindAndClick(moreOptionsButton, defaultUITimeout),
		apputil.FindAndClick(optionTitle, defaultUITimeout),
		apputil.WaitForExists(roomSettingsTitle, defaultUITimeout),
	)

	saveButton := e.d.Object(ui.Text("SAVE"), ui.ResourceID(elementIDPrefix+"roomSettingsSaveAction"))
	newToolbarTitle := e.d.Object(ui.Text(newRoomName), ui.ResourceID(elementIDPrefix+"roomSettingsToolbarTitleView"))
	setRoomNameAndSave := uiauto.Combine("set room name and save",
		// Return to the room settings page when retrying.
		uiauto.IfFailThen(
			roomSettingsTitle.Exists,
			e.navigateUpToObject(roomSettingsTitle),
		),
		apputil.FindAndClick(roomSettingsTitle, defaultUITimeout),
		e.typeText(roomNameFieldID, newRoomName),
		apputil.FindAndClick(saveButton, defaultUITimeout),
		apputil.WaitForExists(newToolbarTitle, defaultUITimeout),
	)
	return uiauto.NamedCombine("rename current room as "+newRoomName,
		e.navigateUpToObject(moreOptionsButton),
		openRoomSettings,
		// Sometimes the save button does not appear.
		// Retry to ensure the room is renamed.
		uiauto.Retry(retryTimes, setRoomNameAndSave),
	)
}

// SearchPublicRoom searches the existing public room with the ID and the name.
func (e *Element) SearchPublicRoom(roomID, roomName string) uiauto.Action {
	createRoomButton := e.d.Object(ui.Description("Create a new conversation or room"), ui.ResourceID(createChatButtonID))
	exploreRoomsText := e.d.Object(ui.Text("Explore Rooms"), ui.ResourceID(elementIDPrefix+"explore_rooms"))
	publicRoom := e.d.Object(ui.Text(roomName), ui.ClassName(textClass))
	return uiauto.NamedCombine("explore public room with ID "+roomID,
		e.navigateUpToObject(createRoomButton),
		apputil.FindAndClick(createRoomButton, defaultUITimeout),
		apputil.FindAndClick(exploreRoomsText, defaultUITimeout),
		e.typeText(searchFieldID, roomID),
		apputil.WaitForExists(publicRoom, defaultUITimeout),
	)
}

// JoinRoom uses |roomFilter| to find the given room from the room list and joins the room.
func (e *Element) JoinRoom(roomName string) uiauto.Action {
	roomFilter := e.d.Object(ui.Description("Filter room names"), ui.ResourceID(elementIDPrefix+"menu_home_filter"))
	room := e.d.Object(ui.Text(roomName), ui.ResourceID(elementIDPrefix+"roomNameView"))
	roomTitle := e.d.Object(ui.Text(roomName), ui.ClassName(textClass))
	return uiauto.NamedCombine(fmt.Sprintf("join %q room from home page", roomName),
		e.navigateUpToObject(roomFilter),
		apputil.FindAndClick(roomFilter, defaultUITimeout),
		e.typeText(searchFieldID, roomName),
		apputil.FindAndClick(room, defaultUITimeout),
		apputil.WaitForExists(roomTitle, defaultUITimeout),
	)
}

// typeText types the text in the given field.
func (e *Element) typeText(fieldID, text string) uiauto.Action {
	textField := e.d.Object(ui.ResourceID(fieldID))
	textFieldFocused := e.d.Object(ui.ResourceID(fieldID), ui.Focused(true))
	textFieldWithText := e.d.Object(ui.TextContains(text), ui.ResourceID(fieldID))
	return uiauto.NamedCombine(fmt.Sprintf("type text %s", text),
		apputil.FindAndClick(textField, defaultUITimeout),
		apputil.WaitForExists(textFieldFocused, defaultUITimeout),
		e.kb.AccelAction("Ctrl+A"),
		e.kb.TypeAction(text),
		apputil.WaitForExists(textFieldWithText, defaultUITimeout),
	)
}

// navigateUpToObject keeps clicking the "Navigate up" button to go back
// to the previous page until the |expectedObject| appears.
func (e *Element) navigateUpToObject(expectedObject *ui.Object) uiauto.Action {
	navigateUpButton := e.d.Object(ui.Description("Navigate up"), ui.ClassName("android.widget.ImageButton"))
	return uiauto.IfFailThen(
		expectedObject.Exists,
		e.ui.WithTimeout(longUITimeout).RetryUntil(
			apputil.FindAndClick(navigateUpButton, defaultUITimeout),
			apputil.WaitForExists(expectedObject, shortUITimeout),
		),
	)
}

// swipeFromObjectToObject swipes from |startObject| to |endObject|.
func (e *Element) swipeFromObjectToObject(startObject, endObject *ui.Object, swipeDuration time.Duration) uiauto.Action {
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
		return apputil.DragAndDrop(e.a, startPoint, endPoint, swipeDuration)(ctx)
	}
}
