// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package mgs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/mgs"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AllowedApps,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Verifies that managed guest sessions only show allowed applications in the launcher",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"eariassoto@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:complementary",
			"group:golden_tier",
			"group:hardware",
			"group:medium_low_tier",
		},
		Timeout: 3 * time.Minute,
		Fixture: fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SystemFeaturesDisableList{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SystemFeaturesDisableMode{}, pci.VerifiedFunctionalityUI),
		}})
}

func AllowedApps(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	mgs, cr, err := mgs.New(
		ctx,
		fdms,
		mgs.DefaultAccount(),
		mgs.AutoLaunch(mgs.MgsAccountID),
		mgs.AddPublicAccountPolicies(mgs.MgsAccountID, []policy.Policy{
			systemFeaturesDisableList(),
			&policy.SystemFeaturesDisableMode{Val: "hidden"},
		}),
	)
	if err != nil {
		s.Fatal("Failed to start MGS: ", err)
	}
	defer func(ctx context.Context) {
		if err := mgs.Close(ctx); err != nil {
			s.Fatal("Failed to close MGS: ", err)
		}
	}(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, false /*tabletMode*/, true /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	appsInLauncher, err := ash.AppsInLauncher(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to get apps in launcher: ", err)
	}

	for _, app := range appsInLauncher {
		if !isAppAllowed(app) {
			s.Fatal("Found disallowed app in MGS launcher: ", app.Name)
		}
	}
}

// systemFeaturesDisableList returns the SystemFeaturesDisableList policy set with all its possible enum values.
func systemFeaturesDisableList() policy.Policy {
	return &policy.SystemFeaturesDisableList{Val: []string{
		"camera",
		"browser_settings",
		"os_settings",
		"scanning",
		"web_store",
		"canvas",
		"explore",
		"gallery",
		"terminal",
	}}
}

// isAppAllowed checks if the app is allowed to be displayed in the launcher.
func isAppAllowed(app *ash.ChromeApp) bool {
	allowedApps := []apps.App{
		apps.PrintManagement,
		apps.FilesSWA,
		apps.ShortcutCustomization,
		apps.Chrome,
	}
	for _, expectedApp := range allowedApps {
		if app.AppID == expectedApp.ID {
			return true
		}
	}
	return false
}
