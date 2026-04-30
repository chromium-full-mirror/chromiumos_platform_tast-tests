// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package loginapi

import (
	"context"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ExternalLogout,
		Desc: "Tests communication between login-screen and in-session extensions via chrome.login external logout APIs",
		Contacts: []string{
			"mpetrisor@chromium.org",
			"chromeos-commercial-identity@google.com",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Identity > Imprivata
		BugComponent: "b:1253162",
		Attr: []string{
			"group:golden_tier_secondary",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceLoginScreenExtensions{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.ExtensionInstallForcelist{}, pci.VerifiedFunctionalityJS),
			{
				Key: "feature_id",
				// Launch MGS with Password (COM_HEALTH_CUJ1_TASK1_WF1).
				Value: "screenplay-47fd7e2a-db80-46cd-b766-71eaf9705376",
			}, {
				Key: "feature_id",
				// Lock MGS in the patient room (COM_HEALTH_CUJ5_TASK2_WF1).
				Value: "screenplay-017b8790-0f92-4483-b395-78dda7d3fd45",
			}, {
				Key: "feature_id",
				// Unlock MGS in the patient room (COM_HEALTH_CUJ5_TASK4_WF1).
				Value: "screenplay-dd1b7d25-4346-4c74-a59e-bafbd4346c86",
			},
		},
	})
}

func ExternalLogout(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	accountID := "foo@managedchrome.com"

	opts := []mgs.Option{
		mgs.Accounts(accountID),
		mgs.AddPublicAccountPolicies(accountID, []policy.Policy{
			&policy.ExtensionInstallForcelist{Val: []string{mgs.InSessionExtensionID}},
		}),
		mgs.ExtraPolicies([]policy.Policy{
			&policy.DeviceLoginScreenExtensions{Val: []string{mgs.LoginScreenExtensionID}},
		}),
		mgs.ExtraChromeOptions(
			chrome.ExtraArgs("--force-devtools-available"),
		),
	}

	m, cr, err := mgs.New(ctx, fdms, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome on Signin screen with MGS accounts: ", err)
	}
	defer func() {
		if err := m.Close(ctx); err != nil {
			s.Fatal("Failed close MGS: ", err)
		}
	}()

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to session manager: ", err)
	}

	swStart, err := sm.WatchSessionStateChanged(ctx, "started")
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer swStart.Close(ctx)

	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.LoginScreenExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to login screen extension: ", err)
	}
	defer conn.Close()

	// Wait for the API to become available.
	if err = conn.WaitForExpr(ctx, `chrome.login !== undefined`); err != nil {
		conn.Close()
		s.Fatal("Failed to wait for the API to be available: ", err)
	}

	const pw = "password"

	// Launch a MGS with password.
	if err := conn.Call(ctx, nil, `(password) => new Promise((resolve, reject) => {
		chrome.login.launchManagedGuestSession(password, () => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, pw); err != nil {
		s.Fatal("Failed to launch MGS: ", err)
	}

	select {
	case <-swStart.Signals:
		// Pass
	case <-ctx.Done():
		s.Fatal("Timeout before getting SessionStateChanged signal: ", ctx.Err())
	}

	inSessionConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.InSessionExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to in-session extension: ", err)
	}
	defer inSessionConn.Close()

	// Wait for the API to become available in the in-session extension.
	if err = inSessionConn.WaitForExpr(ctx, `chrome.login !== undefined`); err != nil {
		s.Fatal("Failed to wait for the API to be available in in-session extension: ", err)
	}

	// In the in-session extension, listen for logout requests from the login screen
	// and notify when handled.
	const setupInSessionListenerJS = `
		chrome.login.onRequestExternalLogout.addListener(() => {
			// When a logout request is received, notify the login screen extension.
			chrome.login.notifyExternalLogoutDone();
		});
	`
	if err := inSessionConn.Eval(ctx, setupInSessionListenerJS, nil); err != nil {
		s.Fatal("Failed to set up listener for onRequestExternalLogout: ", err)
	}

	swLocked, err := sm.WatchScreenIsLocked(ctx)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer swLocked.Close(ctx)

	// Lock the session.
	if err := inSessionConn.Eval(ctx, `new Promise((resolve, reject) => {
		chrome.login.lockManagedGuestSession(() => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, nil); err != nil {
		s.Fatal("Failed to lock session: ", err)
	}

	select {
	case <-swLocked.Signals:
		// Pass
	case <-ctx.Done():
		s.Fatal("Timeout before getting session locked signal: ", ctx.Err())
	}

	// Create a new connection to the login screen extension on the lock screen.
	lockScreenConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.LoginScreenExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to login screen extension on lock screen: ", err)
	}
	defer lockScreenConn.Close()

	// Wait for the API to become available on the lock screen extension.
	if err = lockScreenConn.WaitForExpr(ctx, `chrome.login !== undefined`); err != nil {
		s.Fatal("Failed to wait for the API to be available on lock screen extension: ", err)
	}

	swUnlocked, err := sm.WatchScreenIsUnlocked(ctx)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer swUnlocked.Close(ctx)

	// On the lock screen, request external logout and wait for the done signal
	// from the in-session extension, then unlock the session.
	const requestExternalLogoutAndUnlockJS = `(password) => new Promise((resolve, reject) => {
		const timeout = setTimeout(() => {
			reject(new Error('onExternalLogoutDone was not received'));
		}, 10*1000); // 10s timeout.

		// Add a listener for the logout done signal.
		chrome.login.onExternalLogoutDone.addListener(() => {
			clearTimeout(timeout);
			chrome.login.unlockManagedGuestSession(password, () => {
				if (chrome.runtime.lastError) {
					reject(new Error(chrome.runtime.lastError.message));
					return;
				}
				resolve();
			});
		});

		// Request the in-session extension to log out.
		chrome.login.requestExternalLogout();
	})`
	if err := lockScreenConn.Call(ctx, nil, requestExternalLogoutAndUnlockJS, pw); err != nil {
		s.Fatal("Failed to request external logout and unlock: ", err)
	}

	select {
	case <-swUnlocked.Signals:
		// Pass
	case <-ctx.Done():
		s.Fatal("Timeout before getting session unlocked signal: ", ctx.Err())
	}
}
