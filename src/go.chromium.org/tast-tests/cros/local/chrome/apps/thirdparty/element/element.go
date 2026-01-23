// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package element contains UI functions and common libraries on Element App.
package element

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/apputil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/webutil"
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
	roomNameFieldID    = elementIDPrefix + "formTextInputTextInputEditText"
	searchFieldID      = elementIDPrefix + "search_src_text"
	roomNameID         = elementIDPrefix + "roomNameView"

	buttonClass      = "android.widget.Button"
	textClass        = "android.widget.TextView"
	imageButtonClass = "android.widget.ImageButton"

	retryTimes = 3
	// If the network is unstable, account synchronization may take a long time.
	syncTimeout      = 5 * time.Minute
	loadTimeout      = time.Minute
	longLoadTimeout  = 90 * time.Second
	longUITimeout    = 30 * time.Second
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
	cr     *chrome.Chrome
	apkURL string
}

// New returns a new Element object.
func New(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, a *arc.ARC, d *ui.Device, cr *chrome.Chrome, apkURL string) *Element {
	return &Element{
		tconn:  tconn,
		ui:     uiauto.New(tconn),
		kb:     kb,
		a:      a,
		d:      d,
		cr:     cr,
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

// Login logs in to Element app with Google account.
// It will create an account if the account has not been created.
func (e *Element) Login(ctx context.Context, creds credconfig.Creds) error {
	haveAccountButton := e.d.Object(ui.Text("SIGN IN"), ui.ResourceID(elementIDPrefix+"loginSplashAlreadyHaveAccount"))
	continueButton := e.d.Object(ui.Text("Continue"), ui.ClassName(buttonClass))
	if err := uiauto.NamedCombine("login to Element app",
		apputil.FindAndClick(haveAccountButton, defaultUITimeout),
		// It might take long time to show the login screen.
		// Use loadTimeout to click the continueButton.
		apputil.FindAndClick(continueButton, loadTimeout),
	)(ctx); err != nil {
		return err
	}

	if err := e.loginWithGaiaAccount(creds)(ctx); err != nil {
		return errors.Wrap(err, "failed to login with Gaia account")
	}

	const waitingStatusTextID = elementIDPrefix + "waitingStatusText"
	notNowButton := e.d.Object(ui.Text("NOT NOW"), ui.ResourceID(elementIDPrefix+"later"))
	waitingStatusText := e.d.Object(ui.ResourceID(waitingStatusTextID), ui.ClassName(textClass))
	return uiauto.NamedCombine("skip splash",
		// The |notNowButton| might take more time to appear on low-end devices.
		apputil.ClickIfExist(notNowButton, longUITimeout),
		// Wait for the app finishes syncing the account data with the server.
		// 1. If the account only joins a few rooms, the text would immediately disappear
		// after being shown, and the UI might fail to capture it.
		// 2. If the account joins many rooms, the data sync might take a long time to finish.
		uiauto.IfSuccessThen(
			apputil.WaitForExists(waitingStatusText, defaultUITimeout),
			apputil.WaitUntilGone(waitingStatusText, syncTimeout),
		),
		e.dismissEncryptionAlertIfExists(),
	)(ctx)
}

// loginWithGaiaAccount completes the login flow with Gaia account.
func (e *Element) loginWithGaiaAccount(creds credconfig.Creds) uiauto.Action {
	email := creds.User
	usernameOrEmailField := nodewith.Name("Username or Email").Role(role.TextField)
	emailText := nodewith.Name(email).Role(role.StaticText).Ancestor(usernameOrEmailField)
	setEmail := uiauto.NamedCombine("set email as "+email,
		e.ui.DoDefaultUntil(usernameOrEmailField,
			e.ui.WithTimeout(shortUITimeout).WaitUntilExists(usernameOrEmailField.Focused()),
		),
		e.kb.TypeAction(email),
		e.ui.WaitUntilExists(emailText),
	)

	password := creds.Pass
	passwordField := nodewith.Name("Password").Role(role.TextField)
	hiddenPassword := strings.Repeat("•", len(password))
	passwordText := nodewith.Name(hiddenPassword).Role(role.StaticText).Ancestor(passwordField)
	setPassword := uiauto.NamedCombine("set password",
		e.ui.DoDefaultUntil(passwordField,
			e.ui.WithTimeout(shortUITimeout).WaitUntilExists(passwordField.Focused()),
		),
		e.kb.TypeAction(password),
		e.ui.WaitUntilExists(passwordText),
	)

	continueButton := nodewith.Name("Continue").Role(role.Button)
	continueHeading := nodewith.Name("Continue to element://connect?").Role(role.Heading)
	loginWithAccount := uiauto.NamedCombine("login with Gaia account",
		setEmail,
		setPassword,
		e.ui.DoDefault(continueButton),
		e.ui.WaitUntilExists(continueHeading),
	)

	return uiauto.NamedCombine("sign in with Gaia account",
		e.waitForLoginWindowMaximized,
		// TODO(b:478044130): Remove this workaround once the app is updated.
		e.navigateToNewLoginWebsite,
		// The account information may not be cleaned during the test sequence.
		e.ui.WaitUntilAnyExists(usernameOrEmailField, continueHeading),
		uiauto.IfSuccessThen(e.ui.Exists(usernameOrEmailField), loginWithAccount),
		e.ui.DoDefaultUntil(continueButton,
			e.ui.WaitUntilGone(continueHeading),
		),
	)
}

// navigateToNewLoginWebsite navigates to the correct login website.
func (e *Element) navigateToNewLoginWebsite(ctx context.Context) error {
	loginURLPrefix := "https://matrix-client.matrix.org"
	conn, err := e.cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix(loginURLPrefix))
	if err != nil {
		return errors.Wrap(err, "failed getting connection to new target")
	}
	defer conn.Close()

	var url string
	if err := conn.Eval(ctx, "window.location.href", &url); err != nil {
		return errors.Wrap(err, "failed to get URL")
	}

	if strings.Contains(url, "LOGIN") {
		newURL := strings.Replace(url, "LOGIN", "login", 1)
		if err := conn.Navigate(ctx, newURL); err != nil {
			return errors.Wrap(err, "failed to navigate")
		}
		if err := webutil.WaitForQuiescence(ctx, conn, 10*time.Second); err != nil {
			return errors.Wrap(err, "failed to wait for the page loaded")
		}
	}
	return nil
}

// waitForLoginWindowMaximized activates and maximizes the login window.
func (e *Element) waitForLoginWindowMaximized(ctx context.Context) error {
	const windowTitle = "Chrome"
	// The window title might be displayed in different languages,
	// and "Chrome" is the only common text in the title.
	// Wait for any window with "Chrome" in the title to find the login window.
	loginWindow, err := ash.WaitForAnyWindowWithTitle(ctx, e.tconn, windowTitle)
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
	usernameField := nodewith.Name("Username").Role(role.TextField)
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

	agreeTermsCheckBox := nodewith.Name("I agree to the Terms and Conditions").Role(role.CheckBox)
	agreeTerms := uiauto.NamedAction("agree terms",
		e.ui.DoDefaultUntil(agreeTermsCheckBox,
			e.ui.WithTimeout(shortUITimeout).WaitUntilCheckedState(agreeTermsCheckBox, true),
		),
	)
	createAccountButton := nodewith.Name("Create Account").Role(role.Button)
	continueButton := nodewith.Name("Continue").Role(role.Button)
	return uiauto.NamedCombine("create account",
		setUsername,
		agreeTerms,
		e.ui.DoDefaultUntil(createAccountButton,
			e.ui.WithTimeout(shortUITimeout).WaitUntilExists(continueButton),
		),
		e.ui.DoDefault(continueButton),
	)
}

// dismissEncryptionAlertIfExists dismisses the encryption alert if it exists.
func (e *Element) dismissEncryptionAlertIfExists() uiauto.Action {
	alert := e.d.Object(ui.ResourceID(elementIDPrefix + "llAlertBackground"))
	elementWindow := nodewith.Name("Element").Role(role.Window).HasClass("Widget")
	backButton := nodewith.Name("Back button").Role(role.Button).Ancestor(elementWindow)
	skipButton := e.d.Object(ui.TextMatches("(?i)SKIP"), ui.PackageName(ElementPackage))
	dismissAlert := uiauto.NamedCombine("dismiss encryption alert",
		apputil.FindAndClick(alert, defaultUITimeout),
		apputil.WaitUntilGone(alert, defaultUITimeout),
		e.ui.LeftClick(backButton),
		uiauto.Retry(3, apputil.ClickIfExist(skipButton, defaultUITimeout)),
	)
	return uiauto.IfSuccessThen(
		alert.Exists,
		dismissAlert,
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

// CreateRoom creates a new room in the Element app.
func (e *Element) CreateRoom(roomName string) uiauto.Action {
	createRoomButton := e.d.Object(ui.Description("Create a new conversation or room"), ui.ResourceID(createChatButtonID))
	createRoomText := e.d.Object(ui.Text("Create Room"), ui.ResourceID(elementIDPrefix+"create_room"))
	enterRoomCreationPage := uiauto.NamedCombine("enter room creation page",
		apputil.FindAndClick(createRoomButton, defaultUITimeout),
		apputil.FindAndClick(createRoomText, defaultUITimeout),
	)

	roomAccessText := e.d.Object(ui.Text("Room access"), ui.ResourceID(elementIDPrefix+"settings_section_title_text"))
	roomNameFieldWithText := e.d.Object(ui.Text(roomName), ui.ResourceID(roomNameFieldID))
	createButton := e.d.Object(ui.Text("CREATE"), ui.ResourceID(elementIDPrefix+"form_submit_button"))
	roomTitle := e.d.Object(ui.Text(roomName), ui.ClassName(textClass))
	return uiauto.NamedCombine("create room",
		e.dismissEncryptionAlertIfExists(),
		enterRoomCreationPage,
		e.typeText(roomNameFieldID, roomName),
		e.swipeToShowObject(roomAccessText, roomNameFieldWithText, createButton, swipeDuration),
		apputil.FindAndClick(createButton, defaultUITimeout),
		uiauto.NamedAction("wait for room title "+roomName,
			apputil.WaitForExists(roomTitle, loadTimeout)),
	)
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
		e.dismissEncryptionAlertIfExists(),
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
			e.typeText(roomNameFieldID, newRoomName),
			apputil.FindAndClick(saveButton, defaultUITimeout),
			apputil.WaitUntilGone(saveButton, longUITimeout),
			apputil.WaitForExists(newToolbarTitle, defaultUITimeout),
		)(ctx)
	}
}

// SearchPublicRoom searches the existing public room with the ID and the name.
func (e *Element) SearchPublicRoom(roomID, roomName string) uiauto.Action {
	createRoomButton := e.d.Object(ui.Description("Create a new conversation or room"), ui.ResourceID(createChatButtonID))
	exploreRoomsText := e.d.Object(ui.Text("Explore Rooms"), ui.ResourceID(elementIDPrefix+"explore_rooms"))
	publicRoomText := fmt.Sprintf("(%s|%s)", roomID, roomName)
	publicRoom := e.d.Object(ui.TextMatches(publicRoomText), ui.ClassName(textClass))
	return uiauto.NamedCombine("explore public room with ID "+roomID,
		e.navigateUpToObject(createRoomButton),
		e.dismissEncryptionAlertIfExists(),
		apputil.FindAndClick(createRoomButton, defaultUITimeout),
		apputil.FindAndClick(exploreRoomsText, defaultUITimeout),
		e.typeText(searchFieldID, roomID),
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
		e.typeText(searchFieldID, roomName),
		apputil.FindAndClick(room, defaultUITimeout),
		apputil.WaitUntilGone(room, defaultUITimeout),
		apputil.WaitForExists(roomTitle, defaultUITimeout),
	)
	return uiauto.NamedCombine(fmt.Sprintf("join %q room from home page", roomName),
		e.navigateUpToObject(roomFilter),
		e.dismissEncryptionAlertIfExists(),
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

// typeText types the text in the given field.
func (e *Element) typeText(fieldID, text string) uiauto.Action {
	textField := e.d.Object(ui.ResourceID(fieldID))
	textFieldFocused := e.d.Object(ui.ResourceID(fieldID), ui.Focused(true))
	textFieldWithText := e.d.Object(ui.TextContains(text), ui.ResourceID(fieldID))
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
		e.dismissEncryptionAlertIfExists(),
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
