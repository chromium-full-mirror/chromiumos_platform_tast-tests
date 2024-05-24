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
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchSharedManagedGuestSession,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test chrome.login.launchSharedManagedGuestSession Extension API",
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
		SoftwareDeps: []string{"reboot", "chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceLoginScreenExtensions{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.DeviceRestrictedManagedGuestSessionEnabled{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.ExtensionInstallForcelist{}, pci.VerifiedFunctionalityJS),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityJS),
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

func LaunchSharedManagedGuestSession(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	accountID := "foo@managedchrome.com"

	opts := []mgs.Option{
		mgs.Accounts(accountID),
		mgs.AddPublicAccountPolicies(accountID, []policy.Policy{
			&policy.ExtensionInstallForcelist{Val: []string{mgs.InSessionExtensionID}},
		}),
		mgs.ExtraPolicies([]policy.Policy{
			&policy.DeviceLoginScreenExtensions{Val: []string{mgs.LoginScreenExtensionID}},
			&policy.DeviceRestrictedManagedGuestSessionEnabled{Val: true},
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

	sw, err := sm.WatchSessionStateChanged(ctx, "started")
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close(ctx)

	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix(mgs.LoginScreenExtensionURLPrefix))
	if err != nil {
		s.Fatal("Failed to connect to login screen extension: ", err)
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

	inSessionConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix(mgs.InSessionExtensionURLPrefix))
	if err != nil {
		s.Fatal("Failed to connect to in-session extension: ", err)
	}
	defer inSessionConn.Close()

	// Note that this uses lockManagedGuestSession() since locking an MGS is
	// equivalent to locking the shared session.
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

	// Previous conn is closed since it is a login screen extension which
	// closes when the session starts.
	conn2, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURLPrefix(mgs.LoginScreenExtensionURLPrefix))
	if err != nil {
		s.Fatal("Failed to connect to login screen extension on lock screen: ", err)
	}
	defer conn2.Close()

	unlockSessionFunc := `(password) => new Promise((resolve, reject) => {
		chrome.login.unlockSharedSession(password, () => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`

	// Attempt unlock with wrong password.
	wrongPassword := "wrong password"
	if err := conn2.Call(ctx, nil, unlockSessionFunc, wrongPassword); err == nil {
		s.Fatal("Unlock unexpectedly succeeded with wrong password")
	}

	swUnlocked, err := sm.WatchScreenIsUnlocked(ctx)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer swUnlocked.Close(ctx)

	// Unlock with correct password.
	if err := conn2.Call(ctx, nil, unlockSessionFunc, password); err != nil {
		s.Fatal("Failed to unlock session: ", err)
	}

	select {
	case <-swUnlocked.Signals:
		// Pass
	case <-ctx.Done():
		s.Fatal("Timeout before getting session unlocked signal: ", err)
	}
}
