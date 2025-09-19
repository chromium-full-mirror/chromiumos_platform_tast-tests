// Copyright 2021 The ChromiumOS Authors
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
		Func: LaunchManagedGuestSessionWithPassword,
		Desc: "Test chrome.login.launchManagedGuestSession Extension API",
		Contacts: []string{
			"chromeos-commercial-identity@google.com",
			"mpetrisor@chromium.org",
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

func LaunchManagedGuestSessionWithPassword(ctx context.Context, s *testing.State) {
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

	const (
		pw      = "password"
		wrongPw = "wrong password"
	)

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
		s.Fatal("Timeout before getting SessionStateChanged signal: ", err)
	}

	inSessionConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.InSessionExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to in-session extension: ", err)
	}
	defer inSessionConn.Close()

	// Wait for the API to become available.
	if err = inSessionConn.WaitForExpr(ctx, `chrome.login !== undefined`); err != nil {
		inSessionConn.Close()
		s.Fatal("Failed to wait for the API to be available: ", err)
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
		s.Fatal("Timeout before getting session locked signal: ", err)
	}

	swUnlocked, err := sm.WatchScreenIsUnlocked(ctx)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer swUnlocked.Close(ctx)

	// Create a new connection since login screen extensions are closed when
	// the session is active.
	conn, err = cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.LoginScreenExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to login screen extension: ", err)
	}
	defer conn.Close()

	// Wait for the API to become available.
	if err = conn.WaitForExpr(ctx, `chrome.login !== undefined`); err != nil {
		conn.Close()
		s.Fatal("Failed to wait for the API to be available: ", err)
	}

	unlock := func(pw string) error {
		return conn.Call(ctx, nil, `(password) => new Promise((resolve, reject) => {
			chrome.login.unlockManagedGuestSession(password, () => {
				if (chrome.runtime.lastError) {
					reject(new Error(chrome.runtime.lastError.message));
					return;
				}
				resolve();
			});
		})`, pw)
	}

	// Unlock the session with wrong password.
	if err := unlock(wrongPw); err == nil {
		s.Fatal("Unlocked session with wrong password")
	}

	// Unlock the session with the same password.
	if err := unlock(pw); err != nil {
		s.Fatal("Failed to unlock session: ", err)
	}

	select {
	case <-swUnlocked.Signals:
		// Pass
	case <-ctx.Done():
		s.Fatal("Timeout before getting session unlocked signal: ", err)
	}
}
