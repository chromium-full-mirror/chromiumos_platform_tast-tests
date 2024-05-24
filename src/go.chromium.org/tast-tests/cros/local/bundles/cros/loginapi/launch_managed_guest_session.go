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
		Func:         LaunchManagedGuestSession,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Test chrome.login.launchManagedGuestSession Extension API",
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

func LaunchManagedGuestSession(ctx context.Context, s *testing.State) {
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

	if err := conn.Eval(ctx, `new Promise((resolve, reject) => {
		chrome.login.launchManagedGuestSession(() => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, nil); err != nil {
		s.Fatal("Failed to launch MGS: ", err)
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
}
