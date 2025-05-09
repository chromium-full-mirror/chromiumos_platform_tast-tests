// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: AllowedApps,
		Desc: "Verifies that managed user sessions only show allowed applications in the launcher",
		Contacts: []string{
			"chromeos-commercial-remote-management@google.com",
			"aidazolic@google.com", // Test author
		},
		BugComponent: "b:1111617", // ChromeOS > Software > Commercial (Enterprise) > App Platforms
		SoftwareDeps: []string{"reboot", "chrome"},
		Attr: []string{
			"group:complementary",
			"group:golden_tier",
			"group:hardware",
			"group:medium_low_tier",
			"group:hw_agnostic",
			"group:mainline",
			"informational",
			"group:criticalstaging",
		},
		Timeout: 3 * time.Minute,
		Fixture: fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SystemFeaturesDisableList{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.SystemFeaturesDisableMode{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func AllowedApps(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	pb := policy.NewBlob()
	pb.AddPolicies([]policy.Policy{systemFeaturesDisableList(), &policy.SystemFeaturesDisableMode{Val: "hidden"}})

	if err := policyutil.ServeBlobAndRefresh(ctx, fakeDMS, cr, pb); err != nil {
		s.Fatal("Failed to serve and refresh: ", err)
	}

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
			s.Error("Found disallowed app in user session launcher: ", app.Name, ", id: ", app.AppID)
		}
	}
}

// systemFeaturesDisableList returns the SystemFeaturesDisableList policy set with all its possible enum values.
func systemFeaturesDisableList() policy.Policy {
	return &policy.SystemFeaturesDisableList{Val: []string{
		"camera",
		"os_settings",
		"browser_settings",
		"scanning",
		"crosh",
		"recorder",
		"web_store",
		"canvas",
		"explore",
		"gallery",
		"terminal",
		"print_jobs",
		"key_shortcuts",
		"youtube",
		"google_maps",
		"gmail",
		"google_docs",
		"google_slides",
		"google_sheets",
		"google_drive",
		"google_keep",
		"google_calendar",
		"google_chat",
		"calculator",
		"text_editor",
	}}
}

// isAppAllowed returns true if the app is allowed to be displayed in the launcher (essential or blockable by other policies).
// Consider adjusting the SystemFeaturesDisableList policy rather than this allowlist when new launcher apps cause test failures.
func isAppAllowed(app *ash.ChromeApp) bool {
	allowedApps := []apps.App{
		apps.Chrome,
		apps.FilesSWA,
		apps.ProjectorV2,        // Disabled by the ProjectorEnabled policy.
		apps.SampleSystemWebApp, // Installed by default in unofficial builds.
	}

	for _, expectedApp := range allowedApps {
		if app.AppID == expectedApp.ID {
			return true
		}
	}
	return false
}
