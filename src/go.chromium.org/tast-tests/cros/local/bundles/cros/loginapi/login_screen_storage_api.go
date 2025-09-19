// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package loginapi

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: LoginScreenStorageAPI,
		Desc: "Test chrome.login.loginScreenStorage Extension API",
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
		},
	})
}

func LoginScreenStorageAPI(ctx context.Context, s *testing.State) {
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

	sw, err := sm.WatchSessionStateChanged(ctx, "started")
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close(ctx)

	conn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.LoginScreenExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to login screen extension: ", err)
	}
	defer conn.Close()

	// Wait for the API to become available.
	if err = conn.WaitForExpr(ctx, `chrome.loginScreenStorage !== undefined`); err != nil {
		conn.Close()
		s.Fatal("Failed to wait for the API to be available: ", err)
	}

	// Wait for the extension to write its settings to the storage before writing ours.
	var currentData string
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := conn.Call(ctx, &currentData, `(loginScreenExtensionId) => new Promise((resolve, reject) => {
			chrome.loginScreenStorage.retrievePersistentData(loginScreenExtensionId, (data) => {
				if (chrome.runtime.lastError) {
					reject(new Error(chrome.runtime.lastError.message));
					return;
				}
				resolve(data);
			});
		})`, mgs.LoginScreenExtensionID); err != nil {
			return errors.Wrap(err, "failed to fetch persistent data")
		}
		if currentData == "" {
			return errors.New("extension settings not stored yet")
		}
		return nil
	}, &testing.PollOptions{Interval: 3 * time.Second, Timeout: 30 * time.Second}); err != nil {
		s.Fatal("Failed to wait for extension to store its settings: ", err)
	}

	storedData := "data"
	if err := conn.Call(ctx, nil, `(extensionIds, data) => new Promise((resolve, reject) => {
		chrome.loginScreenStorage.storePersistentData(extensionIds, data, () => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, []string{mgs.InSessionExtensionID}, storedData); err != nil {
		s.Fatal("Failed to store persistent data: ", err)
	}

	storedCredentials := "credentials"
	if err := conn.Call(ctx, nil, `(extensionId, credentials) => new Promise((resolve, reject) => {
		chrome.loginScreenStorage.storeCredentials(extensionId, credentials, () => {
			if (chrome.runtime.lastError) {
				reject(new Error(chrome.runtime.lastError.message));
				return;
			}
			resolve();
		});
	})`, mgs.InSessionExtensionID, storedCredentials); err != nil {
		s.Fatal("Failed to store credentials: ", err)
	}

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

	inSessionConn, err := cr.NewConnForTarget(ctx, chrome.MatchTargetURL(mgs.InSessionExtensionURL))
	if err != nil {
		s.Fatal("Failed to connect to in-session extension: ", err)
	}
	defer inSessionConn.Close()

	if err = inSessionConn.WaitForExpr(ctx, `chrome.loginScreenStorage !== undefined`); err != nil {
		inSessionConn.Close()
		s.Fatal("Failed to wait for the API to be available: ", err)
	}
	defer inSessionConn.Close()

	var retrievedData string
	if err := inSessionConn.Call(ctx, &retrievedData,
		`(loginScreenExtensionId) => new Promise((resolve, reject) => {
			chrome.loginScreenStorage.retrievePersistentData(loginScreenExtensionId, (data) => {
				if (chrome.runtime.lastError) {
					reject(new Error(chrome.runtime.lastError.message));
					return;
				}
				resolve(data);
			});
		})`, mgs.LoginScreenExtensionID); err != nil {
		s.Fatal("Failed to retrieve persistent data: ", err)
	}

	if retrievedData != storedData {
		s.Errorf("Wrong data retrieved, expected: %s, actual: %s", storedData, retrievedData)
	}

	var retrievedCredentials string
	if err := inSessionConn.Call(ctx, &retrievedCredentials,
		`(loginScreenExtensionId) => new Promise((resolve, reject) => {
			chrome.loginScreenStorage.retrieveCredentials((credentials) => {
				if (chrome.runtime.lastError) {
					reject(new Error(chrome.runtime.lastError.message));
					return;
				}
				resolve(credentials);
			});
		})`, mgs.LoginScreenExtensionID); err != nil {
		s.Fatal("Failed to retrieve credentials: ", err)
	}

	if retrievedCredentials != storedCredentials {
		s.Errorf("Wrong credentials retrieved, expected: %s, actual: %s", storedCredentials, retrievedCredentials)
	}
}
