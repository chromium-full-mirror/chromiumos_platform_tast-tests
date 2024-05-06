// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         EphemeralPolicies,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks kiosk state persistance with combinations of policy DeviceEphemeralUsersEnabled and DeviceLocalAccountInfo.EphemeralMode",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceEphemeralUsersEnabled{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceLocalAccounts{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Params: []testing.Param{
			{
				Name: "ash",
				Val:  kioskmode.TestData{IsLacros: false},
			},
			{
				Name:              "lacros",
				Val:               kioskmode.TestData{IsLacros: true},
				ExtraSoftwareDeps: []string{"lacros"},
			},
		},
		// Timeout is (num test cases) * (num stages) * (test time)
		Timeout: 3 * 2 * testCaseTimeout,
	})
}

const testCaseTimeout = kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration

func EphemeralPolicies(ctx context.Context, s *testing.State) {
	isLacros := s.Param().(kioskmode.TestData).IsLacros

	// variant is an AccountID prefix to separate Ash/Lacros profiles and prevent profile migrations
	// (which would trigger a Lacros restart and cause Lacros tests to fail; e.g. in b/277886466).
	variant := "ash"
	if isLacros {
		variant = "lacros"
	}

	// Each test case simulates running a different Kiosk app twice: the first time, to "init" the
	// kiosk homedirs; and the second time, to "verify" if the homedirs were persisted or removed.
	// These test cases are purposely NOT isolated from each other, to test interactions between them.
	// All homedirs are exposed to DeviceEphemeralUsersEnabled being both on and off, and to cryptohome
	// running disk cleanups while the app's homedir is not mounted. This verifies that a kiosk app with
	// EphemeralModeDisable is truly resilient against the DeviceEphemeralUsersEnabled policy.
	testCases := []struct {
		AccountID      string               // AccountID in DeviceLocalAccounts (ID of the kiosk app)
		DevicePolicy   policy.Policy        // DeviceEphemeralUsersEnabled policy
		EphemeralMode  policy.EphemeralMode // EphemeralMode value for the kiosk app
		ExpectedResult string               // Whether the homedir is expected to be "permanent" or "ephemeral"
	}{
		// Warning: FakeDMS requires AccountID to be an email@managedchrome.com
		// See: https://crsrc.org/c/components/policy/test_support/request_handler_for_policy.cc;l=236
		// In production, AccountID can be any string (URLs, for WebKioskApps, etc.)
		{
			// Test the DeviceEphemeralUsersEnabled policy alone.
			AccountID:      variant + "_true_unset@managedchrome.com",
			DevicePolicy:   &policy.DeviceEphemeralUsersEnabled{Val: true},
			EphemeralMode:  policy.EphemeralModeUnset,
			ExpectedResult: "ephemeral",
		},
		{
			// Override the device policy with EphemeralModeDisable.
			AccountID:      variant + "_true_disable@managedchrome.com",
			DevicePolicy:   &policy.DeviceEphemeralUsersEnabled{Val: true},
			EphemeralMode:  policy.EphemeralModeDisable,
			ExpectedResult: "permanent",
		},
		{
			// Override the device policy with EphemeralModeEnable.
			AccountID:      variant + "_false_enable@managedchrome.com",
			DevicePolicy:   &policy.DeviceEphemeralUsersEnabled{Val: false},
			EphemeralMode:  policy.EphemeralModeEnable,
			ExpectedResult: "ephemeral",
		},
	}

	webApp := kioskmode.DefaultWebApp()
	serverURL, cleanupServer := webApp.NewServer(ctx)
	defer cleanupServer()

	localAccounts := policy.DeviceLocalAccounts{Val: []policy.DeviceLocalAccountInfo{}}
	for i := range testCases {
		// Use WebKiosk to simulate logins with different accountIDs. This wouldn't work with
		// ChromeApps because the login would be based on the appID rather than the accountID.
		accountInfo := webApp.NewDeviceLocalAccountInfo(serverURL, testCases[i].AccountID)
		accountInfo.EphemeralMode = &testCases[i].EphemeralMode
		localAccounts.Val = append(localAccounts.Val, accountInfo)
	}

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	for _, stage := range []string{"init", "verify"} {
		for i, tc := range testCases {

			if success := s.Run(ctx, stage+"_"+tc.AccountID, func(ctx context.Context, s *testing.State) {
				ctx, cancel := context.WithTimeout(ctx, testCaseTimeout)
				defer cancel()

				opts := []kioskmode.Option{
					kioskmode.AutoLaunch(tc.AccountID),
					kioskmode.CustomLocalAccounts(&localAccounts),
					kioskmode.ExtraPolicies([]policy.Policy{tc.DevicePolicy}),
					kioskmode.ExtraChromeOptions(
						chrome.ExtraArgs("--kiosk-splash-screen-min-time-seconds=0"),
					),
				}
				if stage == "verify" {
					// KeepState prevents the homedir from being wiped on login.
					opts = append(opts, kioskmode.ExtraChromeOptions(chrome.KeepState()))
				}
				if isLacros {
					// Here we have to pass lacros availabiilty for all kiosk accounts to
					// prevent issues with data migrations.
					for _, testCase := range testCases {
						opts = append(opts, kioskmode.PublicAccountPolicies(testCase.AccountID, []policy.Policy{
							&policy.LacrosAvailability{Val: "lacros_only"},
						}))
					}
				}

				cleanupCtx := ctx
				ctx, cancelCleanup := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
				defer cancelCleanup()

				kiosk, cr, err := kioskmode.New(ctx, fdms, s.RequiredVar("ui.signinProfileTestExtensionManifestKey"), opts...)
				if err != nil {
					s.Fatal("Failed to create Chrome in Kiosk mode: ", err)
				}
				defer func(ctx context.Context) {
					if err := kiosk.Close(ctx); err != nil {
						s.Error("Failed to close kiosk: ", err)
					}
				}(cleanupCtx)

				if err := kiosk.WaitLaunchLogs(ctx); err != nil {
					s.Fatal("Failed to launch Kiosk: ", err)
				}

				// Verify the kiosk homedir is permanent/ephemeral.
				userID := kioskmode.DeviceLocalAccountUserID(&localAccounts.Val[i])
				userPath, err := cryptohome.UserPath(ctx, userID)
				if err != nil {
					s.Fatal("Failed to fetch kiosk UserPath: ", err)
				}
				expectedMountType := cryptohome.Permanent
				if tc.ExpectedResult == "ephemeral" {
					expectedMountType = cryptohome.Ephemeral
				}
				testing.ContextLogf(ctx, "Verifying kiosk homedir is %v: %v", tc.ExpectedResult, userPath)
				if err := cryptohome.WaitForUserMountAndValidateType(ctx, userID, expectedMountType); err != nil {
					s.Fatal("Failed to wait for kiosk mount and validate type: ", err)
				}
				filePath := filepath.Join(userPath, "ephemeral_test")
				switch stage {
				case "init":
					if err := os.WriteFile(filePath, []byte(tc.AccountID), 0644); err != nil {
						s.Error("Failed to write test file: ", err)
					}
				case "verify":
					content, err := os.ReadFile(filePath)
					if err == nil {
						if tc.ExpectedResult == "ephemeral" {
							s.Error("Test file persisted despite ephemeral mount?")
						}
						if string(content) != tc.AccountID {
							s.Errorf("Test file content mismatch: expected %v, got %v", tc.AccountID, content)
						}
					} else if tc.ExpectedResult != "ephemeral" {
						s.Error("Failed to read test file: ", filePath)
					}
				default:
					s.Fatal("Unexpected stage: ", stage)
				}

				if isLacros {
					ctx, cancel := context.WithTimeout(ctx, 15*time.Second)
					defer cancel()

					testing.ContextLog(ctx, "Checking if kiosk is running in Lacros")
					tconn, err := cr.TestAPIConn(ctx)
					if err != nil {
						s.Fatal("Failed to create Test API connection: ", err)
					}
					if _, err = lacrosproc.Root(ctx, tconn); err != nil {
						s.Fatal("Failed to get lacros proc: ", err)
					}
				}
			}); !success {
				s.Fatalf("Failed to run %q subtest, no need to execute follow-up subtests", stage+"_"+tc.AccountID)
			}
		}
	}
}
