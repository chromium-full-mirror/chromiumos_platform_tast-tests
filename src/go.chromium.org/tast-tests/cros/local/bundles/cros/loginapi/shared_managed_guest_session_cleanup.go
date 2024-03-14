// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package loginapi

import (
	"context"
	"net/http"
	"net/http/httptest"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

const (
	// HTML file is the file containing the HTML+JS that will be opened and then closed during the cleanup.
	cleanupTestPageHTML = "cleanup_test_page.html"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SharedManagedGuestSessionCleanup,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test chrome.login.endSharedSession Extension API properly performs cleanup",
		Contacts: []string{
			"chromeos-commercial-identity@google.com",
			"mpetrisor@chromium.org",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Identity > Imprivata
		BugComponent: "b:1253162",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"reboot", "chrome", "vpd"},
		Fixture:      fixture.FakeDMSEnrolled,
		Data:         []string{cleanupTestPageHTML},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceLoginScreenExtensions{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.DeviceRestrictedManagedGuestSessionEnabled{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.ExtensionInstallForcelist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.RestrictedManagedGuestSessionExtensionCleanupExemptList{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityJS),
			{
				Key: "feature_id",
				// Clean shared MGS on clinician logout (COM_HEALTH_CUJ2_TASK1_WF1).
				Value: "screenplay-3422ba87-53ab-4a6b-9ee2-135ad7eca0f5",
			},
		},
		Params: []testing.Param{{
			Name: "ash",
			Val:  browser.TypeAsh,
		}, {
			Name:              "lacros",
			Val:               browser.TypeLacros,
			ExtraSoftwareDeps: []string{"lacros"},
		}},
	})
}

// SharedManagedGuestSessionCleanup tests that chrome.login.endSession performs
// its cleanup operations correctly. The following cleanups are tested:
//  1. Browsing data: This is tested by opening a browser page, setting a
//     cookie, and checking that both the browser history and cookie are cleared
//     after cleanup.
//  2. Open windows: This is tested by opening a browser tab and checking that
//     the tab is closed.
//  3. Extensions: This is tested by checking that the background page
//     connection is closed after cleanup. This is not a direct check since we
//     cannot test if an extension has been reinstalled.
//     The RestrictedManagedGuestSessionExtensionCleanupExemptList policy is also
//     tested here.
//  4. Clipboard: This is tested by setting clipboard data and checking that it
//     is cleared.
//
// Printing is not tested due to the set up needed and will be covered in a
// browser test in Chrome instead.
func SharedManagedGuestSessionCleanup(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	accountID := "foo@managedchrome.com"

	// ID for Chromebook Recovery Utility app. Note this app is arbitrarily chosen
	// and is used to test the
	// RestrictedManagedGuestSessionExtensionCleanupExemptList policy.
	testAppID := "jndclpdbaamdhonoechobihbbiimdgai"
	// ID for the Test API extension.
	testAPIExtensionID := "behllobkkfkfnphdnhnkndlbkcpglgmj"

	opts := []mgs.Option{
		mgs.Accounts(accountID),
		mgs.AddPublicAccountPolicies(accountID, []policy.Policy{
			&policy.ExtensionInstallForcelist{
				Val: []string{mgs.InSessionExtensionID, testAppID},
			},
			&policy.RestrictedManagedGuestSessionExtensionCleanupExemptList{
				Val: []string{mgs.InSessionExtensionID, testAPIExtensionID},
			},
		}),
		mgs.ExtraPolicies([]policy.Policy{
			&policy.DeviceLoginScreenExtensions{
				Val: []string{mgs.LoginScreenExtensionID},
			},
			&policy.DeviceRestrictedManagedGuestSessionEnabled{
				Val: true,
			},
		}),
		mgs.ExtraChromeOptions(
			chrome.ExtraArgs("--force-devtools-available"),
			chrome.LacrosExtraArgs("--force-devtools-available"),
		),
	}

	bt := s.Param().(browser.Type)
	if bt == browser.TypeLacros {
		opts = append(opts, mgs.AddPublicAccountPolicies(accountID, []policy.Policy{
			&policy.LacrosAvailability{Val: "lacros-only"},
		}))
	}

	m, cr, err := mgs.New(ctx, fdms, opts...)
	if err != nil {
		s.Error("Failed to start Chrome on Signin screen with MGS accounts: ", err)
	}
	defer m.Close(ctx)

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to session manager: ", err)
	}

	sw, err := sm.WatchSessionStateChanged(ctx, "started")
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close(ctx)

	loginScreenBGURL := chrome.ExtensionBackgroundPageURL(mgs.LoginScreenExtensionID)
	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(loginScreenBGURL))
	if err != nil {
		s.Fatal("Failed to connect to login screen background page: ", err)
	}
	defer conn.Close()

	// Launch a shared managed guest session.
	password := "password"
	if err := conn.Call(ctx, nil, `(password) => new Promise((resolve, reject) => {
		chrome.login.launchSharedManagedGuestSession(password, () => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, password); err != nil {
		s.Fatal("Failed to launch shared MGS: ", err)
	}

	select {
	case <-sw.Signals:
		// Pass
	case <-ctx.Done():
		s.Fatal("Timeout before getting SessionStateChanged signal: ", err)
	}

	inSessionBGURL := chrome.ExtensionBackgroundPageURL(mgs.InSessionExtensionID)
	inSessionConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(inSessionBGURL))
	if err != nil {
		s.Fatal("Failed to connect to in-session background page: ", err)
	}
	defer inSessionConn.Close()

	swLocked, err := sm.WatchScreenIsLocked(ctx)
	if err != nil {
		s.Fatal("Failed watch for screen lock: ", err)
	}

	testAppBGURL := chrome.ExtensionBackgroundPageURL(testAppID)
	testAppConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(testAppBGURL))
	if err != nil {
		s.Fatal("Failed to connect to test app background page: ", err)
	}
	defer testAppConn.Close()

	// Store arbitrary data in localStorage of the test app.
	if err := testAppConn.Eval(ctx, `new Promise((resolve, reject) => {
		if (chrome.storage === undefined) {
			resolve();
		}
		chrome.storage.local.set({foo: 1}, () => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, nil); err != nil {
		s.Fatal("Failed to set localStorage for test app: ", err)
	}

	server := httptest.NewServer(http.FileServer(s.DataFileSystem()))
	defer server.Close()

	// Open a webpage with cookies, load and unload operations.
	pageConn, err := cr.NewConn(ctx, server.URL+"/"+cleanupTestPageHTML)
	if err != nil {
		s.Fatal("Failed to open test page: ", err)
	}
	defer pageConn.Close()

	tConn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	// The unload popup only shows up if there was user interaction on the page
	// (e.g. any click on the page). The cleanup should also be able to close
	// webpages with onload actions.
	ui := uiauto.New(tConn)
	if err := ui.DoDefault(nodewith.Name("Shared MGS Cleanup Test page").First())(ctx); err != nil {
		s.Fatal("Failed to click the test page: ", err)
	}

	// Set clipboard data.
	if err := ash.SetClipboard(ctx, tConn, "clipboard string"); err != nil {
		s.Fatal("Failed to set clipboard: ", err)
	}

	// Call login.endSharedSession() to end the shared session and trigger
	// cleanup. At the end of the cleanup, the screen will be locked.
	if err := inSessionConn.Eval(ctx, `new Promise((resolve, reject) => {
		chrome.login.endSharedSession(() => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, nil); err != nil {
		s.Fatal("Failed to end shared session: ", err)
	}

	select {
	case <-swLocked.Signals:
		// Pass
	case <-ctx.Done():
		s.Fatal("Timeout before getting session locked signal: ", err)
	}

	swUnlocked, err := sm.WatchScreenIsUnlocked(ctx)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer swUnlocked.Close(ctx)

	// Previous conn is closed since it is a login screen extension which
	// closes when the session starts.
	conn2, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(loginScreenBGURL))
	if err != nil {
		s.Fatal("Failed to connect to login screen background page on lock screen: ", err)
	}
	defer conn2.Close()

	// Enter a new shared session.
	if err := conn2.Call(ctx, nil, `(password) => new Promise((resolve, reject) => {
		chrome.login.enterSharedSession(password, () => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, password); err != nil {
		s.Fatal("Failed to enter new shared session: ", err)
	}

	select {
	case <-swUnlocked.Signals:
		// Pass
	case <-ctx.Done():
		s.Fatal("Timeout before getting session unlocked signal: ", err)
	}

	// Check the inSessionConn is still alive. This indicates that the
	// RestrictedManagedGuestSessionExtensionCleanupExemptList was successfully
	// applied.
	if err := checkConnIsAlive(ctx, inSessionConn); err != nil {
		s.Fatal("In-session extension conn closed unexpectedly: ", err)
	}

	// Cleanup should have closed the test app connection.
	if err := checkConnIsAlive(ctx, testAppConn); err == nil {
		s.Fatal("Test app conn was not closed: ", err)
	}

	// Create new connection for the test app since the old one was closed.
	testAppConn2, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(testAppBGURL))
	if err != nil {
		s.Fatal("Failed to connect to test app background page: ", err)
	}
	defer testAppConn2.Close()

	// Check that localStorage is cleared.
	if err := testAppConn2.Eval(ctx, `new Promise((resolve, reject) => {
		if (chrome.storage === undefined) {
			resolve();
		}
		chrome.storage.local.get((data) => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			if (typeof data !== 'object'  || Object.keys(data).length !== 0) {
				reject(new Error("Expected {}, got: " + JSON.stringify(data)));
				return;
			}
			resolve();
		});
	})`, nil); err != nil {
		s.Fatal("Local storage for test app was not cleared: ", err)
	}

	// Cleanup should have closed all open browser windows.
	if err := pageConn.Eval(ctx, "undefined", nil); err == nil {
		s.Fatal("Page conn was not closed: ", err)
	}

	// Try to restore browser tabs from previous session.
	keyboard, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)
	if err := keyboard.Accel(ctx, "Ctrl+Shift+t"); err != nil {
		s.Fatal("Failed to run keyboard command for restoring tabs: ", err)
	}

	// Check that no browser tabs from previous session got restored.
	pages, err := cr.FindTargets(ctx, chrome.MatchAllPages())
	if err != nil {
		s.Fatal("Failed to collect info about Chrome webpages: ", err)
	}
	if len(pages) > 0 {
		s.Fatal("Expected no restored tabs but found ", len(pages))
	}

	// Open the browsing history page.
	historyConn, err := cr.NewConn(ctx, "chrome://history")
	if err != nil {
		s.Fatal("Failed to open chrome://history: ", err)
	}
	defer historyConn.Close()

	// Check that there are no history entries. EnsureGoneFor is needed as the
	// UI tree is not immediately populated so the node will not be present
	// initially.
	if err := ui.EnsureGoneFor(nodewith.HasClass("website-link").Role(role.Link), 5*time.Second)(ctx); err != nil {
		s.Fatal("Browser history was not cleared: ", err)
	}

	clipboardText, err := ash.ClipboardTextData(ctx, tConn)
	if err != nil {
		s.Fatal("Failed to get clipboard text: ", err)
	}
	if clipboardText != "" {
		s.Fatal("Clipboard was not cleared: ", clipboardText)
	}
}

func checkConnIsAlive(ctx context.Context, conn *chrome.Conn) error {
	result := false
	if err := conn.Eval(ctx, "true", &result); err != nil {
		return err
	}
	if !result {
		return errors.New("eval 'true' returned false")
	}
	return nil
}
