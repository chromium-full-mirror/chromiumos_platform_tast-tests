// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package migrate contains functionality shared by tests that tests profile
// migration from Ash to Lacros.
package migrate

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/localstate"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// LacrosFirstRunPath is the path to `First Run` sentinel file in Lacros profile dir.
const LacrosFirstRunPath = "/home/chronos/user/lacros/First Run"

// Run migrates user profile from Ash to Lacros and wait until migration is marked as completed by Ash.
// Once the migration is completed, it will relaunch Ash Chrome and returns the new `chrome.Chrome` instance.
// Callers are expected to add the Chrome options and set up the environment to trigger forward migration.
func Run(ctx context.Context, chromeOpts []chrome.Option, opts []lacrosfixt.Option) (*chrome.Chrome, error) {
	// TODO(chromium:1290297): This is a hack.
	// chrome.New doesn't really support profile migration because it
	// doesn't anticipate the additional Chrome restart that profile
	// migration effects. As a result, the *Chrome return value is already
	// invalid and we must not use it. Moreover, we must disable the
	// RemoveNotification option because otherwise chrome.New will try to
	// interact with Chrome at a time when that is no longer safe.
	// In order to obtain a valid *Chrome value for the test to continue
	// with, we restart Chrome once more after profile migration.
	testing.ContextLog(ctx, "Restarting for profile migration")
	chromeOpts = append(chromeOpts,
		chrome.KeepState(),
		chrome.RemoveNotification(false),
	)
	opts = append(opts, lacrosfixt.ChromeOptions(chromeOpts...))
	chromeOpts, err := lacrosfixt.NewConfig(opts...).Opts()
	if err != nil {
		return nil, errors.Wrap(err, "failed to compute Chrome options")
	}

	crDoNotUse, err := chrome.New(ctx, chromeOpts...)
	if err != nil {
		return nil, err
	}
	defer func() {
		if crDoNotUse != nil {
			crDoNotUse.Close(ctx)
		}
	}()

	testing.ContextLog(ctx, "Waiting for profile migration to complete")
	userHash, err := cryptohome.UserHash(ctx, crDoNotUse.Creds().User)
	if err != nil {
		return nil, err
	}
	pref := "lacros.profile_migration_completed_for_user." + userHash
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		completedVal, err := localstate.UnmarshalPref(browser.TypeAsh, pref)
		if err != nil {
			return err
		}
		completed, ok := completedVal.(bool)
		if !ok || !completed {
			return errors.New("profile migration incomplete")
		}
		return nil
	}, nil); err != nil {
		return nil, err
	}

	crDoNotUse.Close(ctx)
	crDoNotUse = nil
	testing.ContextLog(ctx, "Restarting after profile migration")
	return chrome.New(ctx, chromeOpts...)
}

// StartChromeToClearMigrationState resets profile migration by starting Chrome with Lacros disabled.
func StartChromeToClearMigrationState(ctx context.Context) (*chrome.Chrome, error) {
	cr, err := chrome.New(ctx, chrome.DisableFeatures("LacrosOnly"))
	if err != nil {
		return nil, errors.Wrap(err, "failed to start chrome")
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(LacrosFirstRunPath); !os.IsNotExist(err) {
			return errors.Wrap(err, "'First Run' file exists or cannot be read")
		}
		return nil
	}, nil); err != nil {
		cr.Close(ctx)
		return nil, errors.Wrap(err, "'First Run' file exists or cannot be read")
	}

	return cr, nil
}

// VerifyLacrosLaunch checks if Lacros is launchable after profile migration.
func VerifyLacrosLaunch(ctx context.Context, s *testing.State, cr *chrome.Chrome) error {
	if _, err := os.Stat(LacrosFirstRunPath); err != nil {
		return errors.Wrap(err, "error reading 'First Run' file")
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}

	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "failed to launch lacros")
	}

	l.Close(ctx)

	return nil
}

const (
	// Arbitrary page title.
	abcPageTitle = "Alphabet Investor Relations"
	// Arbitrary page URL.
	abcPageURL = "https://abc.xyz"
)

// SetupProfileData creates a bookmark, a shortcut, installs an extension,
// downloads a file, and simulates page activity
// by saving a cookie, an IndexedDB entry, a LocalStorage value
// and creating browsing history.
// Clients are expected to launch a browser and have one new tab page
// (to absorb the difference between Ash and Lacros since Lacros
// requires at least one tab to be present for the process to keep running)
// before calling the function.
// Clients are not expected to close tabs after the method call,
// if the data is to be verified by VerifyProfileData.
func SetupProfileData(ctx context.Context, cr *chrome.Chrome, s *testing.State, br *browser.Browser) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}
	ui := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)
	if err := policyutil.EnsureGoogleCookiesAccepted(ctx, br); err != nil {
		return errors.Wrap(err, "failed to accept cookies")
	}
	if err := setupBookmark(ctx, ui, br, kb); err != nil {
		return err
	}
	if err := setupExtension(ctx, ui, br); err != nil {
		return err
	}
	if err := setupShortcut(ctx, ui, br, kb); err != nil {
		return err
	}
	if err := setupDownloads(ctx, ui, br, kb); err != nil {
		return err
	}
	if err := setupHistoryEntry(ctx, ui, br, abcPageURL, abcPageTitle); err != nil {
		return err
	}
	if err := setupExternalPageActivity(ctx, ui, br, s, abcPageURL); err != nil {
		return err
	}
	if err := setupTabPageHistory(ctx, ui, br, abcPageURL); err != nil {
		return err
	}
	if err := br.CloseWithURL(ctx, chrome.NewTabURL); err != nil {
		return errors.Wrap(err, "failed to close the empty tab")
	}
	return nil
}

// VerifyProfileData verifies data previously set up by SetupProfileData.
// Clients are expected to launch a browser before calling the function.
func VerifyProfileData(ctx context.Context, cr *chrome.Chrome, s *testing.State, br *browser.Browser) error {
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to create Test API connection")
	}
	ui := uiauto.New(tconn)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)
	if err := policyutil.EnsureGoogleCookiesAccepted(ctx, br); err != nil {
		return errors.Wrap(err, "failed to accept cookies")
	}
	// Unlike other verify* functions, `verifyTabPageHistory` relies on what tab
	// is opened when launching the browser so verify it first.
	if err := verifyTabPageHistory(ctx, ui, br, kb, abcPageTitle); err != nil {
		return err
	}
	if err := verifyBookmark(ctx, ui, br); err != nil {
		return err
	}
	if err := verifyExtension(ctx, ui, br); err != nil {
		return err
	}
	if err := verifyShortcut(ctx, ui, br); err != nil {
		return err
	}
	if err := verifyDownloads(ctx, ui, br); err != nil {
		return err
	}
	if err := verifyHistoryEntry(ctx, ui, br, abcPageTitle); err != nil {
		return err
	}
	if err := verifyExternalPageActivity(ctx, ui, br, kb, s, abcPageURL, abcPageTitle); err != nil {
		return err
	}
	return nil
}

const (
	// chrome://newtab page URL.
	newTabURL = "chrome://newtab"
	// History page URL.
	historyURL = "chrome://history"
	// Downloads page URL.
	downloadsURL = "chrome://downloads"
	// Arbitrary bookmark name.
	bookmarkName = "MyBookmark12345"
)

// setupBookmark creates an arbitrary bookmark.
func setupBookmark(ctx context.Context, ui *uiauto.Context, br *browser.Browser, kb *input.KeyboardEventWriter) error {
	// Bookmark the downloads page.
	conn, err := br.NewConn(ctx, downloadsURL)
	if err != nil {
		return errors.Wrap(err, "failed to open downloads page")
	}
	defer conn.Close()
	if err := kb.Accel(ctx, "Ctrl+d"); err != nil {
		return errors.Wrap(err, "failed to open bookmark creation popup")
	}
	if err := kb.Type(ctx, bookmarkName); err != nil {
		return errors.Wrap(err, "failed to type bookmark name")
	}
	doneButton := nodewith.Name("Done").Role(role.Button)
	if err := uiauto.Combine("Save bookmark",
		ui.LeftClick(doneButton),
		ui.WaitUntilGone(doneButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to save bookmark")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close downloads page")
	}
	return nil
}

// verifyBookmark verifies the existence of a bookmark set up by setupBookmark.
func verifyBookmark(ctx context.Context, ui *uiauto.Context, br *browser.Browser) error {
	// Check that the bookmark is present.
	conn, err := br.NewConn(ctx, newTabURL)
	defer conn.Close()
	if err != nil {
		return errors.Wrap(err, "failed to open new tab")
	}
	bookmark := nodewith.NameStartingWith(bookmarkName).First()
	if err = ui.WaitUntilExists(bookmark)(ctx); err != nil {
		return errors.Wrap(err, "failed to find bookmark")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close new tab")
	}
	return nil
}

const (
	// Arbitrary extension from Chrome Store.
	extensionName = "User-Agent Switcher for Chrome"
	// ID of the above extension.
	extensionID = "djflhoibgkdhkhhcedjiklpkjnoahfmg"
	// Chrome Store URL of the above extension
	extensionWebStoreURL = "https://chrome.google.com/webstore/detail/" + extensionID + "?hl=en"
	// Arbitrary extension URL.
	extensionURL = "chrome://extensions/?id=" + extensionID
)

// setupExtension installs an arbitrary extension.
func setupExtension(ctx context.Context, ui *uiauto.Context, br *browser.Browser) error {
	// Navigate to the extension web store page.
	conn, err := br.NewConn(ctx, extensionWebStoreURL)
	if err != nil {
		return errors.Wrap(err, "failed to navigate to extension web store page")
	}
	defer conn.Close()
	// Look for the "Add to Chrome" button on the page, which shows
	// a popup for the final installation of the extension.
	// On the popup find the "Add extension" button to install the extension.
	// After the extension is installed, the "Add to Chrome" button on
	// the extension page should change to the "Remove from Chrome" button.
	addToChromeButton := nodewith.Name("Add to Chrome").Role(role.Button).First()
	addExtensionButton := nodewith.Name("Add extension").Role(role.Button)
	removeButton := nodewith.Name("Remove from Chrome").Role(role.Button).First()
	if err := uiauto.Combine("Install extension",
		ui.LeftClick(addToChromeButton),
		// The "Add extension" button may not immediately be clickable.
		ui.LeftClickUntil(addExtensionButton, ui.Gone(addExtensionButton)),
		// TODO(crbug.com/1326398): Remove tab reload when this bug is fixed.
		ui.RetryUntil(br.ReloadActiveTab, ui.WithTimeout(7*time.Second).WaitUntilExists(removeButton)),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to install")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close extension web store page")
	}
	return nil
}

// verifyExtension checks that the extension is installed and enabled.
func verifyExtension(ctx context.Context, ui *uiauto.Context, br *browser.Browser) error {
	conn, err := br.NewConn(ctx, extensionURL)
	if err != nil {
		return errors.Wrap(err, "failed to open extension page")
	}
	defer conn.Close()
	extensionText := nodewith.Name(extensionName).Role(role.StaticText)
	onText := nodewith.Name("On").Role(role.StaticText)
	if err := uiauto.Combine("Verify the extension is installed and enabled",
		ui.WaitUntilExists(extensionText),
		ui.Exists(onText),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to verify extension status")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close extension page")
	}
	return nil
}

const (
	// Arbitrary shortcut name.
	shortcutName = "MyShortcut12345"
	// Arbitrary shortcut URL.
	shortcutURL = "foobar"
)

// setupShortcut creates an arbitrary shortcut.
func setupShortcut(ctx context.Context, ui *uiauto.Context, br *browser.Browser, kb *input.KeyboardEventWriter) error {
	conn, err := br.NewConn(ctx, newTabURL)
	if err != nil {
		return errors.Wrap(err, "failed to open new tab")
	}
	defer conn.Close()
	addShortcutButton := nodewith.Name("Add shortcut").Role(role.Button)
	if err := uiauto.Combine("Click 'Add shortcut' button",
		ui.LeftClick(addShortcutButton),
		ui.WaitUntilGone(addShortcutButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open 'Add shortcut' popup")
	}
	if err := kb.Type(ctx, shortcutName+"\t"+shortcutURL); err != nil {
		return errors.Wrap(err, "failed to type shortcut data")
	}
	doneButton := nodewith.Name("Done").Role(role.Button)
	if err := uiauto.Combine("Click 'Done' button",
		ui.LeftClick(doneButton),
		ui.WaitUntilGone(doneButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to create shortcut")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close new tab")
	}
	return nil
}

// verifyShortcut verifies the existence of a shortcut created
// by setupShortcut.
func verifyShortcut(ctx context.Context, ui *uiauto.Context, br *browser.Browser) error {
	conn, err := br.NewConn(ctx, newTabURL)
	if err != nil {
		return errors.Wrap(err, "failed to open new tab")
	}
	defer conn.Close()
	// Check that the shortcut is present.
	shortcutLink := nodewith.Name(shortcutName).Role(role.Link)
	if err := ui.WaitUntilExists(shortcutLink)(ctx); err != nil {
		return errors.Wrap(err, "failed to find shortcut")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close new tab")
	}
	return nil
}

// setupDownloads downloads an arbitrary file.
func setupDownloads(ctx context.Context, ui *uiauto.Context, br *browser.Browser, kb *input.KeyboardEventWriter) error {
	// Navigate to the downloads page.
	conn, err := br.NewConn(ctx, downloadsURL)
	if err != nil {
		return errors.Wrap(err, "failed to open downloads page")
	}
	defer conn.Close()
	if err := kb.Accel(ctx, "Ctrl+s"); err != nil {
		return errors.Wrap(err, "failed to open download popup")
	}
	saveButton := nodewith.Name("Save").Role(role.Button).Focusable()
	if err := uiauto.Combine("Click 'Save' button",
		ui.WaitUntilExists(saveButton),
		ui.WaitUntilEnabled(saveButton),
		ui.LeftClick(saveButton),
		ui.WaitUntilGone(saveButton),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to click download save button")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close downloads page")
	}
	return nil
}

// verifyDownloads verifies the existence of an entry of a file
// downloaded by setupDownloads on the Downloads page.
func verifyDownloads(ctx context.Context, ui *uiauto.Context, br *browser.Browser) error {
	// Navigate to the downloads page.
	conn, err := br.NewConn(ctx, downloadsURL)
	if err != nil {
		return errors.Wrap(err, "failed to open downloads page")
	}
	defer conn.Close()
	// Check that the download page shows the previous download (of itself).
	downloadedFile := nodewith.NameStartingWith(downloadsPageTitle).Role(role.Link)
	if err := ui.WaitUntilExists(downloadedFile)(ctx); err != nil {
		return errors.Wrap(err, "failed to find previously downloaded file")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close downloads page")
	}
	return nil
}

// setupHistoryEntry visits `pageURL` and makes sure that `pageTitle` appears on history page.
func setupHistoryEntry(ctx context.Context, ui *uiauto.Context, br *browser.Browser, pageURL, pageTitle string) error {
	// Visit the page and create a history entry.
	conn, err := br.NewConn(ctx, pageURL)
	if err != nil {
		return errors.Wrap(err, "failed to open page")
	}
	defer conn.Close()
	if err := waitForHistoryEntry(ctx, ui, br, pageTitle, true); err != nil {
		return errors.Wrap(err, "failed to find history entry")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close "+pageURL)
	}

	return nil
}

// verifyHistoryEntry verifies that `pageTitle` is present on history page.
func verifyHistoryEntry(ctx context.Context, ui *uiauto.Context, br *browser.Browser, pageTitle string) error {
	// Check that the browsing history contains the visited page.
	if err := waitForHistoryEntry(ctx, ui, br, pageTitle, false); err != nil {
		return errors.Wrap(err, "failed to find history entry")
	}

	return nil
}

const (
	// chrome://downloads page title.
	downloadsPageTitle = "Downloads"
	// Arbitrary cookie.
	cookie = "MyCookie1234=abcd"
	// Arbitrary localStorage key.
	localStorageKey = "myCat"
	// Arbitrary localStorage value.
	localStorageValue = "Meow"
	// Arbitrary user id.
	indexedDBUserID = 123
	// Arbitrary user email.
	indexedDBUserEmail = "test@gmail.com"
)

// setupExternalPageActivity visits a page and stores data associated with the page with cookies, localStorage and indexedDB APIs.
func setupExternalPageActivity(ctx context.Context, ui *uiauto.Context, br *browser.Browser, s *testing.State, pageURL string) error {
	conn, err := br.NewConn(ctx, pageURL)
	if err != nil {
		return errors.Wrap(err, "failed to open page")
	}
	defer conn.Close()
	// Set cookies on the page.
	if err := conn.Call(ctx, nil, `(cookie) => document.cookie = cookie`, cookie); err != nil {
		return errors.Wrap(err, "failed to set cookie")
	}
	// Set localStorage on the page.
	if err := conn.Call(ctx, nil, `(key, value) => localStorage.setItem(key, value)`, localStorageKey, localStorageValue); err != nil {
		return errors.Wrap(err, "failed to set localStorage value")
	}
	// Create indexedDB value on the page.
	insertIndexedDBDataJS, err := os.ReadFile(s.DataPath("migrate/indexeddb_set.js"))
	if err != nil {
		return errors.Wrap(err, "failed to read IndexedDB setter script")
	}
	if err := conn.Call(ctx, nil, string(insertIndexedDBDataJS), indexedDBUserID, indexedDBUserEmail); err != nil {
		return errors.Wrap(err, "insertIndexedDBDataJS failed")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close page")
	}

	return nil
}

// verifyExternalPageActivity verifies previously set up user activity in setupExternalPageActivity.
func verifyExternalPageActivity(ctx context.Context, ui *uiauto.Context, br *browser.Browser, kb *input.KeyboardEventWriter, s *testing.State, pageURL, pageTitle string) error {
	// Check if the cookie, localStorage and indexedDB values are set.
	conn, err := br.NewConn(ctx, pageURL)
	if err != nil {
		return errors.Wrap(err, "failed to open page")
	}
	defer conn.Close()
	contained := false
	if err := conn.Call(ctx,
		&contained,
		`(cookie) => { return document.cookie.split('; ').includes(cookie); }`, cookie); err != nil {
		return errors.Wrap(err, "failed to get cookies")
	}
	if !contained {
		return errors.New("Cookie set previously could not be found")
	}
	if err := conn.Call(ctx, &contained,
		`(key, value) => { return localStorage.getItem(key) == value; }`, localStorageKey, localStorageValue); err != nil {
		return errors.Wrap(err, "failed to get localStorage value")
	}
	if !contained {
		return errors.Wrap(err, "localStorage value set previously could not be found")
	}
	checkIndexedDBDataJS, err := os.ReadFile(s.DataPath("migrate/indexeddb_check.js"))
	if err != nil {
		return errors.Wrap(err, "failed to read IndexedDB checker script")
	}
	if err := conn.Call(ctx, nil, string(checkIndexedDBDataJS), indexedDBUserID, indexedDBUserEmail); err != nil {
		return errors.Wrap(err, "checkIndexedDBDataJS failed")
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close page")
	}
	return nil
}

// waitForHistoryEntry verifies that the page is listed on the History page.
func waitForHistoryEntry(ctx context.Context, ui *uiauto.Context, br *browser.Browser, pageTitle string, allowReload bool) error {
	conn, err := br.NewConn(ctx, historyURL)
	if err != nil {
		return errors.Wrap(err, "failed to open history page")
	}
	defer conn.Close()
	link := nodewith.Name(pageTitle).Role(role.Link)
	err = ui.WaitUntilExists(link)(ctx)
	if err != nil && allowReload {
		// If the page in question has just been visited, sometimes the
		// history page needs to be reloaded before the entry shows up
		// there. So reload and try again with a longer timeout.
		err = uiauto.Combine("find history entry", br.ReloadActiveTab, ui.WithTimeout(30*time.Second).WaitUntilExists(link))(ctx)
	}
	if err != nil {
		return err
	}
	if err := conn.CloseTarget(ctx); err != nil {
		return errors.Wrap(err, "failed to close target")
	}

	return nil
}

// setupTabPageHistory opens a tab, visits pageURL first and visits the downloads page to create tab history.
// The opened tab is left unclosed so that it can be verified after migration.
func setupTabPageHistory(ctx context.Context, ui *uiauto.Context, br *browser.Browser, pageURL string) error {
	conn, err := br.NewConn(ctx, pageURL)
	if err != nil {
		return errors.Wrap(err, "failed to open page "+pageURL)
	}

	// Navigate to the Downloads page to create a tab history.
	if err := conn.Navigate(ctx, downloadsURL); err != nil {
		return errors.Wrap(err, "failed to navigate to downloads page")
	}

	// Do not close the tab and return.
	return nil
}

// verifyTabPageHistory verifies that the active tab has the tab history created by setupTabPageHistory.
// The active tab should be the tab created by setupTabPageHistory when this function is called.
func verifyTabPageHistory(ctx context.Context, ui *uiauto.Context, br *browser.Browser, kb *input.KeyboardEventWriter, pageTitle string) error {
	// Verify that the currently opened page is the downloads page.
	title := nodewith.Name(downloadsPageTitle).First()
	if err := ui.WaitUntilExists(title)(ctx); err != nil {
		return errors.Wrap(err, "download page is not present")
	}

	// Check that going back in history once brings us to the page with
	// `pageTitle`.
	if err := kb.Accel(ctx, "Alt+Left"); err != nil {
		return errors.Wrap(err, "failed to press alt+left")
	}
	title = nodewith.Name(pageTitle).First()
	if err := ui.WaitUntilExists(title)(ctx); err != nil {
		return errors.Wrap(err, "failed to go to the previously visited page")
	}

	return nil
}
