// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"context"
	"time"

	crApps "go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vdi/apps"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const uiDetectionTimeout = 45 * time.Second

// Connector structure used for performing operation on Citrix app.
type Connector struct {
	dataPath func(string) string
	detector *uidetection.Context
	tconn    *chrome.TestConn
	keyboard *input.KeyboardEventWriter
}

// Init initializes state of the connector.
func (c *Connector) Init(s *testing.FixtState, tconn *chrome.TestConn, d *uidetection.Context, k *input.KeyboardEventWriter) {
	c.dataPath = s.DataPath
	c.detector = d
	c.tconn = tconn
	c.keyboard = k
}

// EnterServerURL enters url to the Citrix setup and hits connect.
func (c *Connector) EnterServerURL(ctx context.Context, cfg *apps.VDILoginConfig) error {
	testing.ContextLog(ctx, "Citrix: entering server url")
	ui := uiauto.New(c.tconn)

	textField := nodewith.Name("Store URL or Email address").Role(role.TextField)
	testing.ContextLog(ctx, "Citrix: entering server url")
	if err := uiauto.Combine("enter citrix server url, connect and wait for next screen",
		ui.WaitUntilExists(textField),
		ui.DoDefault(textField),
		c.keyboard.TypeAction(cfg.Server),
		c.keyboard.AccelAction("Enter"), // Connect to the server.
	)(ctx); err != nil {
		return errors.Wrap(err, "failed entering server url")
	}

	return nil
}

// EnterCredentialsAndLogin waits for the screen and enters credentials.
func (c *Connector) EnterCredentialsAndLogin(ctx context.Context, cfg *apps.VDILoginConfig) error {
	testing.ContextLog(ctx, "Citrix: entering username and password and logging in")
	//TODO: b/268335458 Relpace uidetect with uiauto when applicable.
	if err := uiauto.Combine("enter username and password and connect login",
		c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(uidetection.TextBlock([]string{"User", "name"})),
		c.keyboard.TypeAction(cfg.Username),
		c.keyboard.AccelAction("Tab"),
		c.keyboard.TypeAction(cfg.Password),
		c.keyboard.AccelAction("Tab"),
		c.keyboard.AccelAction("Enter"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed entering username or password")
	}

	return nil
}

// Login connects to the server and logs in using information provided in config.
func (c *Connector) Login(ctx context.Context, cfg *apps.VDILoginConfig) error {
	testing.ContextLog(ctx, "Citrix: logging in")

	if err := c.EnterServerURL(ctx, cfg); err != nil {
		return errors.Wrap(err, "failed to enter server url")
	}

	if err := c.EnterCredentialsAndLogin(ctx, cfg); err != nil {
		return errors.Wrap(err, "failed to enter credentials and log in")
	}

	return nil
}

// Logout logs out user. Can be called when Citrix logged-in main screen is
// visible.
func (c *Connector) Logout(ctx context.Context) error {
	testing.ContextLog(ctx, "Citrix: logging out")
	//TODO: b/268335458 Relpace uidetect with uiauto when applicable.
	if err := uiauto.Combine("log out from the Citrix",
		c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(uidetection.TextBlock([]string{"Citrix", "Workspace"})),
		c.detector.LeftClick(uidetection.TextBlock([]string{"Citrix", "Workspace"})), // By clicking, focus on the first UI element.
		c.detector.LeftClick(uidetection.Word("C").ExactMatch()),                     // Click on the user icon.
		c.detector.WithScreenshotResizing().LeftClick(uidetection.TextBlock([]string{"Log", "Out"})),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to log out")
	}

	return nil
}

// LoginAfterRestart handles special case when application does not cache the
// session and requires login after app restarts.
func (c *Connector) LoginAfterRestart(ctx context.Context) error {
	// Do nothing since Citrix session is cached by the app and login is not
	// needed.
	testing.ContextLog(ctx, "Citrix: already logged in")
	return nil
}

// WaitForMainScreenVisible ensures that element visible on the screen
// indicates it is the main Citrix screen.
func (c *Connector) WaitForMainScreenVisible(ctx context.Context) error {

	if err := c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(uidetection.TextBlock([]string{"Search", "Workspace"}))(ctx); err != nil {
		return errors.Wrap(err, "didn't see expected text block after logging into Citrix")
	}

	return nil
}

// SearchAndOpenApplication opens given application using search provided in
// Citrix, runs checkIfOpened function to ensure app opened. Before calling
// make sure main Citrix screen is visible by calling
// WaitForMainScreenVisible(). Call ResetSearch() to clean the search state.
func (c *Connector) SearchAndOpenApplication(ctx context.Context, appName string, checkIfOpened func(context.Context) error) uiauto.Action {
	//TODO: b/268335458 Relpace uidetect with uiauto when applicable.
	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Citrix: opening app %s", appName)
		return uiauto.Combine("open application "+appName+" in Citrix",
			// kamilszarek@: I tried using uidetector clicking on test block
			// "Search Workspace" and it wound click it but focus does not
			// go on the search field.
			c.keyboard.AccelAction("Tab"),
			c.keyboard.AccelAction("Tab"),
			c.keyboard.AccelAction("Tab"),   // Go to the search field.
			c.keyboard.AccelAction("Enter"), // Move focus to the search field.
			c.keyboard.TypeAction(appName),
			c.keyboard.AccelAction("Down"),  // Go to the first result.
			c.keyboard.AccelAction("Enter"), // Open the app.
			checkIfOpened,
		)(ctx)
	}
}

// ResetSearch cleans search field. Call only when search was triggered by
// SearchAndOpenApplication().
func (c *Connector) ResetSearch(ctx context.Context) error {
	testing.ContextLog(ctx, "Citrix: cleaning search")
	// Check that result was actually triggered.
	if err := c.detector.WithTimeout(uiDetectionTimeout).WithScreenshotResizing().WaitUntilExists(uidetection.TextBlock([]string{"See", "more", "results"}))(ctx); err != nil {
		return errors.Wrap(err, "search results view is not visible")
	}
	// Citrix, after executed search and the app was opened and then closed
	// keeps the search result overlay on with focus on it.
	if err := uiauto.Combine("clearing search results",
		c.keyboard.AccelAction("Esc"), // Return to the Citrix view.
	)(ctx); err != nil {
		return errors.Wrap(err, "couldn't clear the search results")
	}
	return nil
}

// ReplaceDetector replaces detector instance.
func (c *Connector) ReplaceDetector(d *uidetection.Context) {
	c.detector = d
}

// CleanUpSession cleans up the session by logging off from existing
// connections. It is being executed in fixtures (mgs, user session) PostTest()
// function.
// If not performed then user upon consecutive logins will have several apps
// opened.
func (c *Connector) CleanUpSession(ctx context.Context) error {
	testing.ContextLog(ctx, "Citrix: open Connection Center and focus on it")
	ui := uiauto.New(c.tconn)
	vdiAppShelfButton := nodewith.Name(crApps.Citrix.Name).HasClass("ash/ShelfAppButton")
	connectorCenterContextMenuItem := nodewith.Name("Connection Center").HasClass("MenuItemView")
	if err := uiauto.Combine("open Connector Center on VDI app",
		ui.RightClick(vdiAppShelfButton),
		ui.WaitUntilExists(connectorCenterContextMenuItem),
		ui.LeftClick(connectorCenterContextMenuItem),
		ui.LeftClick(vdiAppShelfButton),
		ui.WaitUntilExists(connectorCenterContextMenuItem),
		ui.LeftClick(connectorCenterContextMenuItem),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open Connector Center and bring it to the foreground")
	}

	session := nodewith.HasClass("sessionElement")
	// Only if there are active sessions log off from them.
	if err := ui.Exists(session)(ctx); err == nil {
		testing.ContextLog(ctx, "Citrix: select session and log off")
		logoffBtn := nodewith.Name("Logoff").HasClass("sessionButton").Role(role.Button)
		if err := uiauto.Combine("select session and log off",
			ui.LeftClick(session),
			ui.LeftClick(logoffBtn),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to select session and log off")
		}
	} else {
		testing.ContextLog(ctx, "Citrix: no active sessions found")
	}

	// Close the Connection Center window.
	closeButton := nodewith.HasClass("close_button")
	if err := ui.LeftClick(closeButton)(ctx); err != nil {
		return errors.Wrap(err, "failed to close the connection center")
	}

	return nil
}
