// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package citrix

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/uidetection"
	"go.chromium.org/tast-tests/cros/local/vdi/apps"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const uiDetectionTimeout = time.Minute

// Connector structure used for performing operation on Citrix app.
type Connector struct {
	dataPath func(string) string
	detector *uidetection.Context
	tconn    *chrome.TestConn
	keyboard *input.KeyboardEventWriter
}

// Init initializes state of the connector.
func (c *Connector) Init(dataPath func(string) string, tconn *chrome.TestConn, d *uidetection.Context, k *input.KeyboardEventWriter) {
	c.dataPath = dataPath
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
		ui.WithTimeout(2*time.Minute).WaitUntilExists(textField),
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
	ui := uiauto.New(c.tconn)

	usernameTextField := nodewith.NameContaining("user@domain.com").Role(role.TextField)
	if err := uiauto.Combine("enter username and password and connect login",
		ui.WithTimeout(2*time.Minute).WaitUntilExists(usernameTextField),
		ui.DoDefault(usernameTextField),
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

	if cfg.Server != "" {
		if err := c.EnterServerURL(ctx, cfg); err != nil {
			return errors.Wrap(err, "failed to enter server url")
		}
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
		c.detector.LeftClick(uidetection.TextBlock([]string{"Citrix", "Workspace"})),
		c.keyboard.AccelAction("Tab"),
		c.keyboard.AccelAction("Tab"),
		c.keyboard.AccelAction("Enter"),
		c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(uidetection.TextBlock([]string{"Log", "Out"})), // Click on the user icon.
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

// WaitForMainScreenVisible ensures that all apps are visible on the screen
// indicates it is the main Citrix screen.
func (c *Connector) WaitForMainScreenVisible(ctx context.Context) error {
	if err := c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(uidetection.TextBlock([]string{"Citrix", "Workspace"}).First())(ctx); err != nil {
		return errors.Wrap(err, "didn't see expected text block in the main Citrix screen")
	}
	if err := c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(uidetection.TextBlock([]string{"Google", "Chrome"}).First())(ctx); err != nil {
		return errors.Wrap(err, "didn't see expected Google Chrome app in the main Citrix screen")
	}
	if err := c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(uidetection.Word("Notepad").First())(ctx); err != nil {
		return errors.Wrap(err, "didn't see expected Notepad app in the main Citrix screen")
	}
	if err := c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(uidetection.TextBlock([]string{"COMMERCIAL", "VDI"}).First())(ctx); err != nil {
		return errors.Wrap(err, "didn't see expected Desktop in the main Citrix screen")
	}

	return nil
}

// OpenApplication opens given application in Citrix, runs
// checkIfOpened function to ensure app opened. Before calling
// make sure main Citrix screen is visible by calling
// WaitForMainScreenVisible().
func (c *Connector) OpenApplication(ctx context.Context, appName string, checkIfOpened func(context.Context) error) uiauto.Action {
	//TODO: b/268335458 Relpace uidetect with uiauto when applicable.
	ui := uiauto.New(c.tconn)
	return func(ctx context.Context) error {
		testing.ContextLogf(ctx, "Citrix: opening app %s", appName)
		appIcon := uidetection.TextBlock(strings.Fields(appName)).First()
		return uiauto.Combine("open application "+appName+" in Citrix",
			c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(appIcon),
			ui.WithTimeout(100*time.Second).RetryUntil(c.detector.LeftClick(appIcon), checkIfOpened),
		)(ctx)
	}
}

// closeAllAppsWithX closes all apps in Citrix. It's useful for closing apps launched in Kiosk session, since switching windows isn't possible in kiosk mode.
func (c *Connector) closeAllAppsWithX(ctx context.Context) error {
	testing.ContextLog(ctx, "Citrix: closing all apps with X button")
	ui := uiauto.New(c.tconn)
	appWindowControls := uidetection.CustomIcon(c.dataPath("citrix/app_window_controls.png"), uidetection.MinConfidence(0.65)).First()
	doYouWantToSaveDialog := uidetection.TextBlock([]string{"Do", "you", "want", "to", "save", "changes"})
	// Use polling to repeatedly try to close app windows until none are found.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := c.detector.WithTimeout(uiDetectionTimeout).Exists(appWindowControls)(ctx); err != nil {
			// No app windows found, so we're done.
			return nil // Poll successfully completed
		}

		// Close an app window.
		closeAppWithX := func(ctx context.Context) error {
			windowControlsLoc, err := c.detector.WithTimeout(uiDetectionTimeout).Location(ctx, appWindowControls)
			if err != nil {
				return errors.Wrap(err, "failed to find the location of the window controls")
			}
			if err := ui.MouseClickAtLocation( /*left click*/ 0, coords.Point{X: windowControlsLoc.RightCenter().X, Y: windowControlsLoc.RightCenter().Y})(ctx); err != nil {
				return errors.Wrap(err, "failed to close the app with the close button")
			}
			if err := c.detector.WaitUntilExists(doYouWantToSaveDialog); err == nil {
				if err := uiauto.Combine("don't save",
					c.keyboard.AccelAction("Tab"), // Move to "Don't Save" button
					c.keyboard.AccelAction("Enter"),
				)(ctx); err != nil {
					return errors.Wrap(err, "couldn't click on don't save")
				}
			}
			return nil
		}

		if err := closeAppWithX(ctx); err != nil {
			// Log the error, but don't stop polling.  The window might still disappear.
			testing.ContextLog(ctx, "Error closing app window (will retry): ", err)
		}

		return errors.New("app windows still exist") // Indicate that polling should continue.
	}, &testing.PollOptions{Timeout: 2 * time.Minute}); err != nil {
		// If polling times out, that means there are still app windows open, and we failed to close them.
		return errors.Wrap(err, "failed to close all app windows")
	}
	return nil
}

// closeAllAppsWithLogoff closes all apps in Citrix using the Logoff script. It will force close all the Citrix apps for User and MGS.
func (c *Connector) closeAllAppsWithLogoff(ctx context.Context) error {
	testing.ContextLog(ctx, "Citrix: closing all apps using the Logoff script")
	if err := c.focusOnMainScreenWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to focus on Citrix Workspace main screen")
	}
	logoff := uidetection.Word("Logoff")
	return uiauto.Combine("run Logoff script",
		c.detector.WithTimeout(uiDetectionTimeout).WaitUntilExists(logoff),
		c.detector.LeftClick(logoff),
	)(ctx)
}

// CloseDesktop closes all apps and logs off the windows desktop.
func (c *Connector) CloseDesktop(ctx context.Context) error {
	testing.ContextLog(ctx, "Citrix: closing desktop")
	recycleBin := uidetection.TextBlock([]string{"Recycle", "Bin"})
	// Recycle Bin exists, so the desktop is likely open.
	if err := c.detector.WithTimeout(uiDetectionTimeout).WithScreenshotResizing().Exists(recycleBin)(ctx); err == nil {
		if err := uiauto.Combine("Run Logout script",
			// Move focus on Windows desktop.
			c.detector.LeftClick(recycleBin),
			c.keyboard.AccelAction("Ctrl+Alt+S"),
		)(ctx); err != nil {
			return errors.Wrap(err, "failed to run the Signout script using the combo keys Ctrl+Alt+S")
		}
	}
	return nil
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

func (c *Connector) focusOnMainScreenWindow(ctx context.Context) error {
	windowTitle := "Citrix Workspace"
	testing.ContextLogf(ctx, "Focusing on window: %s", windowTitle)

	matchTitle := func(w *ash.Window) bool {
		return strings.Contains(w.Title, windowTitle)
	}

	// Get the first window with title Citrix Workspace.
	w, err := ash.FindWindow(ctx, c.tconn, matchTitle)
	if err != nil {
		return errors.Wrap(err, "could not find window with matching title")
	}
	if err := w.ActivateWindow(ctx, c.tconn); err != nil {
		return errors.Wrap(err, "failed to activate window")
	}
	return nil
}

// OpenDesktop opens the windows desktop and makes sure that the desktop is visible.
func (c *Connector) OpenDesktop(ctx context.Context) error {
	const desktopName string = "COMMERCIAL VDI"
	isOpened := func(ctx context.Context) error {
		recycleBin := uidetection.TextBlock([]string{"Recycle", "Bin"})
		// Find the Recycle Bin first.
		if err := c.detector.WithTimeout(60 * time.Second).WaitUntilExists(recycleBin)(ctx); err != nil {
			return errors.Wrap(err, "failed waiting for the recycle bin to appear")
		}
		return nil
	}
	if err := c.OpenApplication(ctx, desktopName, isOpened)(ctx); err != nil {
		return errors.Wrap(err, "failed to close desktop")
	}
	return nil
}

// cleanUpDesktopKiosk cleans up the kiosk session by closing all apps using X button and closing the desktop if necessary.
// It is being executed in fixtures vdi_kiosk only PostTest() function.
func (c *Connector) cleanUpDesktopKiosk(ctx context.Context) error {
	testing.ContextLog(ctx, "Citrix: Cleaning up desktop for kiosk session")
	// Close apps if needed.
	if err := c.closeAllAppsWithX(ctx); err != nil {
		return errors.Wrap(err, "failed to close all the apps using the X button")
	}
	if err := c.CloseDesktop(ctx); err != nil {
		return errors.Wrap(err, "failed to close desktop")
	}
	return c.WaitForMainScreenVisible(ctx)
}

// cleanUpDesktopUser cleans up the desktop session by the Signout script.
// This should be used by User and MGS only. Use CleanUpKioskDesktopSession for Kiosk.
func (c *Connector) cleanUpDesktopUser(ctx context.Context) error {
	testing.ContextLog(ctx, "Citrix: Cleaning up desktop for User/MGS session")

	if err := c.focusOnMainScreenWindow(ctx); err != nil {
		return errors.Wrap(err, "failed to focus on Citrix Workspace main screen")
	}
	if err := c.OpenDesktop(ctx); err != nil {
		return errors.Wrap(err, "failed to open desktop")
	}
	if err := c.CloseDesktop(ctx); err != nil {
		return errors.Wrap(err, "failed to close desktop")
	}
	return c.WaitForMainScreenVisible(ctx)
}

// CleanupAllApps closes all apps in Citrix given the current mode.
// If it is kiosk mode, then switching windows isn't possible and using the X button is required.
// If it is User or MGS, then switching windows is possible and using the Logoff script is more stable.
func (c *Connector) CleanupAllApps(ctx context.Context, isKioskMode bool) error {
	if isKioskMode {
		return c.closeAllAppsWithX(ctx)
	}
	return c.closeAllAppsWithLogoff(ctx)
}

// CleanUpDesktop ensures the Desktop is cleaned up using the Signout script.
// If it is kiosk mode, then switching windows isn't possible and using the X button is required to close all apps first, then close the desktop.
// If it is User or MGS, then switching windows is possible and opening desktop and using the Signout script is more stable.
func (c *Connector) CleanUpDesktop(ctx context.Context, isKioskMode bool) error {
	if isKioskMode {
		return c.cleanUpDesktopKiosk(ctx)
	}
	return c.cleanUpDesktopUser(ctx)
}
