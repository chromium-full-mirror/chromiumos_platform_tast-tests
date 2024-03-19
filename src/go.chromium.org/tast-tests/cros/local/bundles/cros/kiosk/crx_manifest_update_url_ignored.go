// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CRXManifestUpdateURLIgnored,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if CRXManifestUpdateURLIgnored policy is correctly reflected in update mechanism of extensions",
		Contacts: []string{
			"chromeos-commercial-chrome-apps-and-extensions@google.com", // Test owner
			"chromeos-kiosk-eng+TAST@google.com",
			"zubeil@google.com", // Original test author
		},
		BugComponent: "b:1253865",
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"reboot", "chrome", "vpd"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceLocalAccounts{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.KioskCRXManifestUpdateURLIgnored{}, pci.VerifiedFunctionalityUI),
		},
		Timeout: time.Duration(len(crxManifestUpdateURLIgnoredTestCases)) * crxManifestUpdateURLIgnoredTestCaseTimeout,
	})
}

// crxManifestUpdateURLIgnoredTestcaseTimeout is the timeout of one test case. Each test case sets
// up, launches, and closes Kiosk, plus 30 seconds buffer time per test case.
const crxManifestUpdateURLIgnoredTestCaseTimeout = kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration + 30*time.Second

// crxManifestUpdateURLIgnoredTestcases is a slice of all test case parameters in this test.
var crxManifestUpdateURLIgnoredTestCases = []struct {
	name             string
	ignoreCrxURL     bool
	originalAppTitle string
	updatedAppTitle  string
	appID            string
	updateURL        string
}{
	// The test uses three mock Extensions, with 2 versions each. This allows testing of the correct
	// update logic in accordance with KioskCRXManifestUpdateURLIgnored policy.

	// crx-File does not contain an update URL. Version should correspond to updateURL in policy.
	{
		name:             "no_update_url_v1",
		ignoreCrxURL:     true,
		originalAppTitle: "Test extension #4",
		updatedAppTitle:  "Test extension #4",
		appID:            "kjecmldfmbflidigcdfdnegjgkgggoih",
		updateURL:        "https://storage.googleapis.com/extension_test/kjecmldfmbflidigcdfdnegjgkgggoih-update-7.xml",
	},
	{
		name:             "no_update_url_v2",
		ignoreCrxURL:     true,
		originalAppTitle: "Test extension #4 (updated)",
		updatedAppTitle:  "Test extension #4 (updated)",
		appID:            "kjecmldfmbflidigcdfdnegjgkgggoih",
		updateURL:        "https://storage.googleapis.com/extension_test/kjecmldfmbflidigcdfdnegjgkgggoih-update-8.xml",
	},
	// Update URL in crx points to V1 of the app. Version should correspond to updateURL in policy.
	{
		name:             "old_version_url_v1",
		ignoreCrxURL:     true,
		originalAppTitle: "Test extension #5",
		updatedAppTitle:  "Test extension #5",
		appID:            "fimgekdokgldflggeacgijngdienfdml",
		updateURL:        "https://storage.googleapis.com/extension_test/fimgekdokgldflggeacgijngdienfdml-update-9.xml",
	},
	{
		name:             "old_version_url_v2",
		ignoreCrxURL:     true,
		originalAppTitle: "Test extension #5 (updated)",
		updatedAppTitle:  "Test extension #5 (updated)",
		appID:            "fimgekdokgldflggeacgijngdienfdml",
		updateURL:        "https://storage.googleapis.com/extension_test/fimgekdokgldflggeacgijngdienfdml-update-10.xml",
	},
	// Update URL in crx points to V2 of the app. It should only upgrade to V2 if policy is false.
	{
		name:             "new_version_url_v1",
		ignoreCrxURL:     true,
		originalAppTitle: "Test extension #6",
		updatedAppTitle:  "Test extension #6",
		appID:            "epeagdmdgnhlibpbnhalblaohdhhkpne",
		updateURL:        "https://storage.googleapis.com/extension_test/epeagdmdgnhlibpbnhalblaohdhhkpne-update-11.xml",
	},
	{
		name:             "new_version_url_v2",
		ignoreCrxURL:     true,
		originalAppTitle: "Test extension #6 (updated)",
		updatedAppTitle:  "Test extension #6 (updated)",
		appID:            "epeagdmdgnhlibpbnhalblaohdhhkpne",
		updateURL:        "https://storage.googleapis.com/extension_test/epeagdmdgnhlibpbnhalblaohdhhkpne-update-12.xml",
	},
	// Since policy is set to false, the App should upgrade automatically.
	{
		name:             "crx_updates",
		ignoreCrxURL:     false,
		originalAppTitle: "Test extension #6",
		updatedAppTitle:  "Test extension #6 (updated)",
		appID:            "epeagdmdgnhlibpbnhalblaohdhhkpne",
		updateURL:        "https://storage.googleapis.com/extension_test/epeagdmdgnhlibpbnhalblaohdhhkpne-update-11.xml",
	},
}

func CRXManifestUpdateURLIgnored(ctx context.Context, s *testing.State) {
	for _, tc := range crxManifestUpdateURLIgnoredTestCases {
		testcaseCtx, cancel := context.WithTimeout(ctx, crxManifestUpdateURLIgnoredTestCaseTimeout)
		defer cancel()

		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			launchKioskAndVerify(testcaseCtx, s, tc.ignoreCrxURL, tc.originalAppTitle, tc.updatedAppTitle, tc.appID, tc.updateURL)
		})
	}
}

func launchKioskAndVerify(ctx context.Context, s *testing.State, ignoreCrxURL bool, originalAppTitle, updatedAppTitle, appID, updateURL string) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	accountID := "foo@managedchrome.com"
	accountType := policy.AccountTypeKioskApp

	kioskPolicy := []policy.Policy{
		&policy.KioskCRXManifestUpdateURLIgnored{Val: ignoreCrxURL},
	}

	account := &policy.DeviceLocalAccounts{
		Val: []policy.DeviceLocalAccountInfo{
			{
				AccountID:   &accountID,
				AccountType: &accountType,
				KioskAppInfo: &policy.KioskAppInfo{
					AppId:     &appID,
					UpdateUrl: &updateURL,
				},
			},
		},
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	signinTestExtensionManifestKey := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		signinTestExtensionManifestKey,
		kioskmode.CustomLocalAccounts(account),
		kioskmode.ExtraPolicies(kioskPolicy),
		kioskmode.ExtraChromeOptions(chrome.LoadSigninProfileExtension(signinTestExtensionManifestKey)),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupCtx)

	if !openExtensionAndCheckTitleChange(ctx, s, kiosk, cr, originalAppTitle, updatedAppTitle) {
		s.Fatal("Missmatch in Version")
	}
}

// openExtensionAndCheckTitleChange opens the kiosk app from login screen using the originalAppTitle and evaluates if the launched app has updatedAppTitle.
func openExtensionAndCheckTitleChange(ctx context.Context, s *testing.State, kiosk *kioskmode.Kiosk, cr *chrome.Chrome, originalAppTitle, updatedAppTitle string) bool {
	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	if err := kioskmode.LaunchAppManually(ctx, tconn, originalAppTitle); err != nil {
		s.Fatal("Failed to start Kiosk app from Sign-in screen: ", err)
	}

	// Wait for Kiosk launch.
	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	// Wait for extension UI to be visible.
	versionNode := nodewith.NameStartingWith("Version: ").First()
	if err := ui.WaitUntilExists(versionNode)(ctx); err != nil {
		s.Fatal("Failed to wait for extension to launch: ", err)
	}

	titleNode := nodewith.ClassName("RootView").Name(updatedAppTitle)
	matchesVersion, err := ui.IsNodeFound(ctx, titleNode)
	if err != nil {
		s.Fatal("Failed to check title node: ", err)
	}

	return matchesVersion
}
