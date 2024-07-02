// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package wmp

import (
	"context"
	"encoding/json"
	"io/ioutil"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/externaldata"
	"go.chromium.org/tast-tests/cros/local/saveddesks"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         AdminTemplatesLaunch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks admin templates can be launched",
		Contacts: []string{
			"chromeos-wm@google.com",
			"cros-commercial-productivity-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"zhumatthew@google.com",
		},
		// Chrome OS Server Projects > Enterprise Management > Commercial Productivity
		BugComponent: "b:1020793",
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Timeout:      chrome.GAIALoginTimeout + arc.BootTimeout + 180*time.Second,
		VarDeps:      []string{ui.GaiaPoolDefaultVarName},
		Data:         []string{"admin_desk_template.json"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.PreconfiguredDeskTemplates{}, pci.VerifiedFunctionalityUI),
			pci.SearchFlag(&policy.DeskTemplatesEnabled{}, pci.VerifiedFunctionalityUI),
		},
		Fixture: fixture.ChromeAdminDeskTemplatesLoggedIn,
	})
}

func AdminTemplatesLaunch(ctx context.Context, s *testing.State) {
	const adminDeskTemplateName = "Test Admin template for default user"

	// Reserve five seconds for various cleanup.
	cleanupCtx := ctx
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure clamshell mode: ", err)
	}
	defer cleanup(cleanupCtx)
	// Open admin desk template for view.
	templateJSON, err := getJSONFileFromFilePath(s.DataPath("admin_desk_template.json"))

	eds, err := externaldata.NewServer(ctx)
	if err != nil {
		s.Fatal("Failed to create server: ", err)
	}
	defer eds.Stop(ctx)

	iurl, ihash := eds.ServePolicyData(templateJSON)

	defer ash.SetOverviewModeAndWait(cleanupCtx, tconn, false)
	defer ash.CleanUpDesks(cleanupCtx, tconn)
	s.AttachErrorHandlers(
		func(errMsg string) {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")
		},
		func(errMsg string) {
			faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_fatal")
		})

	if _, err := apps.PrimaryBrowser(ctx, tconn); err != nil {
		s.Fatal("Could not find the primary browser app info: ", err)
	}

	policiesToServe := []policy.Policy{
		&policy.PreconfiguredDeskTemplates{Val: &policy.PreconfiguredDeskTemplatesValue{Url: iurl, Hash: ihash}},
		&policy.DeskTemplatesEnabled{Val: true},
	}
	const subtestName = "nonempty"
	{
		s.Run(ctx, subtestName, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+subtestName)
			ac := uiauto.New(tconn)

			// Close all existing windows.
			if err := ash.CloseAllWindows(ctx, tconn); err != nil {
				s.Fatal("Failed to close all windows: ", err)
			}

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies for desk templates enabled policy.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, policiesToServe); err != nil {
				s.Fatal("Failed to update desk templates enabled policies: ", err)
			}

			// Wait for admin template sync.
			ash.WaitForSavedDeskSync(ctx, ac)

			// Enters overview mode.
			if err := ash.SetOverviewModeAndWait(ctx, tconn, true); err != nil {
				s.Fatal("Failed to set overview mode: ", err)
			}

			// Find the "Library" button.
			templatesButton := nodewith.Name("Library")
			desksTemplatesGridView := nodewith.ClassName("SavedDeskLibraryView")
			// Show admin desk template.
			if err := uiauto.Combine(
				"show the admin desk template",
				ac.DoDefault(templatesButton),
				// Wait for the desk templates grid shows up.
				ac.WaitUntilExists(desksTemplatesGridView),
			)(ctx); err != nil {
				s.Fatal("Failed to show admin desk templates: ", err)
			}

			// Check that the admin template is there.
			savedDeskIndex, err := ash.GetSavedDeskIndexForName(ctx, ac, adminDeskTemplateName)
			if err != nil {
				s.Fatal("Failed to find saved desks: ", err)
			}

			savedDeskItemView := nodewith.ClassName("SavedDeskItemView").Nth(savedDeskIndex)
			if err := ac.Exists(nodewith.Name("Shared by your administrator").Ancestor(savedDeskItemView))(ctx); err != nil {
				s.Fatalf("Failed to find an admin desk template with the name %s", adminDeskTemplateName)
			}

			// Launch the admin template.
			if err := ash.LaunchSavedDesk(ctx, ac, adminDeskTemplateName, 0); err != nil {
				s.Fatal("Failed to launch admin template: ", err)
			}

			browserApp, err := apps.PrimaryBrowser(ctx, tconn)
			if err != nil {
				s.Fatal("Could not find the primary browser app info: ", err)
			}
			appsList := []apps.App{browserApp, browserApp}

			// Wait for apps to launch.
			if err := saveddesks.WaitforAppsToLaunch(ctx, tconn, ac, appsList); err != nil {
				s.Fatal("Failed to wait for apps to launch: ", err)
			}
			// Wait for apps to be visible.
			if err := saveddesks.WaitforAppsToBeVisible(ctx, tconn, ac, appsList); err != nil {
				s.Fatal("Failed to wait for apps to be visible: ", err)
			}

			defer func() {
				if err := ash.CloseAllWindows(ctx, tconn); err != nil {
					s.Fatal("Failed to close all windows at cleanup: ", err)
				}
			}()

			// Exit overview mode and wait.
			if err = ash.SetOverviewModeAndWait(ctx, tconn, false); err != nil {
				s.Fatal("Failed to exit overview mode: ", err)
			}

			// Verifies that there are the app windows.
			ws, err := ash.GetAllWindows(ctx, tconn)
			if err != nil {
				s.Fatal("Failed to get all open windows: ", err)
			}
			if len(ws) != 2 {
				s.Fatalf("Got %v window(s), should have %v windows", len(ws), 2)
			}
		})
	}
}

// getJSONFileFromFilePath returns bytes of json file with the file path.
func getJSONFileFromFilePath(filePath string) ([]byte, error) {
	byteValue, _ := ioutil.ReadFile(filePath)
	var jsonResult interface{}
	json.Unmarshal(byteValue, &jsonResult)
	return json.Marshal(jsonResult)
}
