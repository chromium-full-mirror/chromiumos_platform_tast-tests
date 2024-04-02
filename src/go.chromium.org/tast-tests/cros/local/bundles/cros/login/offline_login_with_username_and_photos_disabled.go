// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/bundles/cros/login/signinutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/shill"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OfflineLoginWithUsernameAndPhotosDisabled,
		Desc:         "Checks that a user can login again if they have already signed in even though the network is offline and the device owner disabled the username and photos",
		LacrosStatus: testing.LacrosVariantUnneeded,
		Contacts: []string{
			"cros-lurs@google.com",
			"bchikhaoui@google.com",
			"cros-oac@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 3*chrome.LoginTimeout + 45*time.Second,
		SearchFlags: []*testing.StringPair{{
			Key: "feature_id",
			// Offline Authentication with network offline and usernames
			// and photos disabled by policy.
			Value: "screenplay-ed77a405-6f9c-45f1-b1dc-5787f741e82a",
		}},
	})
}

func OfflineLoginWithUsernameAndPhotosDisabled(ctx context.Context, s *testing.State) {

	creds := []chrome.Creds{
		{User: "deviceOwner@gmail.com", Pass: "test pass 1"},
		{User: "additionalUser1@gmail.com", Pass: "test pass 2"},
	}

	cleanUpCtx := ctx
	// 10 seconds should be enough for all clean up operations.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	setupOwnerAndUsersAndPresetting(ctx, cleanUpCtx, s, creds)

	loginOffline(ctx, s, creds)

}

func loginOffline(ctx context.Context, s *testing.State, creds []chrome.Creds) {
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx,
		chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))

	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}
	defer cr.Close(cleanUpCtx)

	oobeConn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to create OOBE connection: ", err)
	}
	defer oobeConn.Close()

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(cleanUpCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	manager, err := shill.NewManager(ctx)
	if err != nil {
		s.Fatal("Failed to create Manager object: ", err)
	}
	h := helper{Manager: manager}
	defer h.restoreAllNetworkInterfaces(cleanUpCtx)
	if err := h.disableAllNetworkInterfaces(ctx); err != nil {
		s.Fatal("Failed to disable non cellular interface: ", err)
	}

	// GetEnabledTechnologies returns a list of all enabled shill networking technologies.
	enabledTechnologies, err := manager.GetEnabledTechnologies(ctx)
	if err != nil {
		s.Fatal("Failed to retrieve enabled shill networking technologies: ", err)
	}

	s.Log("List of all enabled shill networking technologies ", enabledTechnologies)
	if err := clickSignInAsExistingUserLink(ctx, oobeConn); err != nil {
		s.Fatal("Failed to click signIn as existing user: ", err)
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.OfflineLoginScreen.isReadyForTesting()"); err != nil {
		s.Fatal("Failed to wait for the offline login screen to be visible: ", err)
	}

	var emailFieldName string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.OfflineLoginScreen.getEmailFieldName()", &emailFieldName); err != nil {
		s.Fatal("Failed to retrieve the email field name: ", err)
	}

	var passwordFieldName string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.OfflineLoginScreen.getPasswordFieldName()", &passwordFieldName); err != nil {
		s.Fatal("Failed to retrieve the password field name: ", err)
	}

	kb, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn)
	fillTextField(ctx, s, ui, kb, emailFieldName, creds[1].User)
	clickNextButton(ctx, s, ui, oobeConn)
	fillTextField(ctx, s, ui, kb, passwordFieldName, creds[1].Pass)
	clickNextButton(ctx, s, ui, oobeConn)

	if err := lockscreen.WaitForLoggedIn(ctx, tconn, chrome.LoginTimeout); err != nil {
		s.Fatal("Failed to login: ", err)
	}
}

func clickSignInAsExistingUserLink(ctx context.Context, oobeConn *chrome.Conn) error {
	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ErrorScreen.isReadyForTesting()"); err != nil {
		return errors.Wrap(err, "failed to wait for the error screen to be visible")
	}

	if err := oobeConn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.ErrorScreen.isOfflineLinkVisible()"); err != nil {
		return errors.Wrap(err, "failed to wait for the offline link to be visible")
	}

	if err := oobeConn.Eval(ctx, "OobeAPI.screens.ErrorScreen.clickSignInAsExistingUserLink()", nil); err != nil {
		return errors.Wrap(err, "failed to click on sign in as existing user link")
	}

	return nil

}

type helper struct {
	Manager            *shill.Manager
	enableEthernetFunc func(ctx context.Context)
	enableWifiFunc     func(ctx context.Context)
	enableCellularFunc func(ctx context.Context)
}

func (h *helper) disableAllNetworkInterfaces(ctx context.Context) error {
	ctx, cancel := ctxutil.Shorten(ctx, shill.EnableWaitTime*2)
	defer cancel()

	// Disable Ethernet if present and maybe re-enabling.
	ethernetFunc, err := h.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyEthernet)
	if err != nil {
		return errors.Wrap(err, "unable to disable Ethernet")
	}

	// Disable  Cellular if present and maybe re-enabling.
	cellularFunc, err := h.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyCellular)
	if err != nil {
		return errors.Wrap(err, "unable to disable Cellular")
	}

	// Disable Wifi if present and maybe re-enabling.
	wifiFunc, err := h.Manager.DisableTechnologyForTesting(ctx, shill.TechnologyWifi)
	if err != nil {
		return errors.Wrap(err, "unable to disable Wifi")
	}

	h.enableEthernetFunc = ethernetFunc
	h.enableWifiFunc = wifiFunc
	h.enableCellularFunc = cellularFunc

	return nil
}

// restoreAllNetworkInterfaces enable previously disabled interfaces.
func (h *helper) restoreAllNetworkInterfaces(ctx context.Context) {
	if h.enableEthernetFunc != nil {
		h.enableEthernetFunc(ctx)
	}

	if h.enableWifiFunc != nil {
		h.enableWifiFunc(ctx)
	}

	if h.enableCellularFunc != nil {
		h.enableWifiFunc(ctx)
	}

	h.enableEthernetFunc = nil
	h.enableWifiFunc = nil
	h.enableCellularFunc = nil
}

// fillTextField assumes that the text field will be autofocused.
// This may not work correctly for pages with two fields.
func fillTextField(ctx context.Context, s *testing.State, ui *uiauto.Context, kb *input.KeyboardEventWriter, nodeName, nodeValue string) {

	textfield := nodewith.Name(nodeName).Role(role.TextField)

	if err := uiauto.Combine("Fill the text field",
		ui.WaitUntilExists(textfield),
		ui.DoDefaultUntil(textfield, ui.Exists(textfield.Focused())),
	)(ctx); err != nil {
		s.Fatal("Failed to select the text field : ", err)
	}

	if err := kb.Type(ctx, nodeValue); err != nil {
		s.Fatal("Failed to type the text field : ", err)
	}

}

func clickNextButton(ctx context.Context, s *testing.State, ui *uiauto.Context, oobeConn *chrome.Conn) {

	var nextbuttonName string
	if err := oobeConn.Eval(ctx, "OobeAPI.screens.OfflineLoginScreen.getNextButtonName()", &nextbuttonName); err != nil {
		s.Fatal("Failed to retrieve the next button name: ", err)
	}

	if err := uiauto.Combine("Click on next button",
		ui.WaitUntilExists(nodewith.Name(nextbuttonName).Role(role.Button)),
		ui.LeftClick(nodewith.Name(nextbuttonName).Role(role.Button)),
	)(ctx); err != nil {
		s.Fatal("Failed to click on next button: ", err)
	}
}

func setupOwnerAndUsersAndPresetting(ctx, cleanUpCtxs context.Context, s *testing.State, creds []chrome.Creds) {

	chrome.New(ctx,
		chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))

	// Login with the Device Owner.
	cr, err := userutil.Login(ctx, creds[0].User, creds[0].Pass)
	if err != nil {
		s.Fatal("Failed to log in as device owner: ", err)
	}

	// For the device owner we wait until their ownership has been established.
	err = userutil.WaitForOwnership(ctx, cr)

	if err != nil {
		s.Fatal("User setup to owner failed: ", err)
	}

	// Setup the Setting Options.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanUpCtxs, s.OutDir(), s.HasError, tconn)

	settings, err := signinutil.OpenManageOtherPeople(ctx, cr, tconn)
	if err != nil {
		s.Fatal("Failed to open Manage other people: ", err)
	}
	defer cr.Close(cleanUpCtxs)
	if settings != nil {
		defer settings.Close(cleanUpCtxs)
	}
	ui := uiauto.New(tconn)

	err = ui.LeftClick(nodewith.Name("Show usernames and photos on the sign-in screen").Role(role.ToggleButton))(ctx)
	if err != nil {
		s.Fatal("Failed to click on the show usernames and photos toggle: ", err)
	}

	// Waiting for PropertyChangeComplete signal for the ShowUserNames setting
	// to confirm it has been written to disk.
	sessionManager, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to create session_manager binding: ", err)
	}
	settingsWatcher, err := sessionManager.WatchPropertyChangeComplete(ctx)
	if err != nil {
		s.Fatal("Failed to start watching PropertyChangeComplete signal: ", err)
	}
	defer settingsWatcher.Close(ctx)

	select {
	case <-settingsWatcher.Signals:
	case <-ctx.Done():
		s.Fatal("Timed out waiting for PropertyChangeComplete signal: ", ctx.Err())
	}

	setting, err := session.RetrieveSettings(ctx, sessionManager)

	if err != nil {
		s.Fatal("Failed to retrieve settings: ", err)
	}

	if setting.ShowUserNames.GetShowUserNames() {
		s.Fatal("Failed to toggle ShowUserNames value")
	}

	// Create another user.
	if err := userutil.CreateUser(ctx, creds[1].User, creds[1].Pass, chrome.KeepState()); err != nil {
		s.Fatal("Failed to create new user: ", err)
	}

}
