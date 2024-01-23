// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package dlp

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/dlp/files"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DataLeakPreventionRulesListFilesArc,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test behavior of DataLeakPreventionRulesList policy with file ARC restriction",
		Contacts: []string{
			"chromeos-dlp@google.com",
			"aidazolic@google.com",
		},
		BugComponent: "b:892101",
		// ChromeOS > Software > Commercial (Enterprise) > DLP (Data Loss Prevention)
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DataLeakPreventionRulesList{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ArcEnabled{}, pci.Served),
		},
		Params: []testing.Param{
			{
				Name:              "arc_container",
				ExtraAttr:         []string{"group:mainline", "informational"},
				Fixture:           fixture.ChromePolicyLoggedInARCFilesUXEnabled,
				ExtraSoftwareDeps: []string{"android_p"},
			}, {
				Name: "arc_vm",
				ExtraAttr: []string{
					"group:golden_tier",
					"group:medium_low_tier",
					"group:hardware",
					"group:complementary",
				},
				Fixture:           fixture.ChromePolicyLoggedInARCFilesUXEnabled,
				ExtraSoftwareDeps: []string{"android_vm"},
			},
		},
		// TODO(b/276847225): Reduce timeout if we can move arc boot to fixture.
		Timeout: 5 * time.Minute,
		Data: []string{
			"download.html",
			"data.txt",
		},
	})
}

func DataLeakPreventionRulesListFilesArc(ctx context.Context, s *testing.State) {
	const (
		bootTimeout = 2 * time.Minute
	)

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fakeDMS := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_error")

	// Update the policy.
	filesArcWarnPolicies := []policy.Policy{&policy.DataLeakPreventionRulesList{
		Val: []*policy.DataLeakPreventionRulesListValue{
			{
				Name:        "Warn before transferring a confidential file to Play files",
				Description: "User should be warned before transferring a confidential file to Play files",
				Sources: &policy.DataLeakPreventionRulesListValueSources{
					Urls: []string{
						"*",
					},
				},
				Destinations: &policy.DataLeakPreventionRulesListValueDestinations{
					Components: []string{
						"ARC",
					},
				},
				Restrictions: []*policy.DataLeakPreventionRulesListValueRestrictions{
					{
						Class: "FILES",
						Level: "WARN",
					},
				},
			},
		},
	},
		&policy.ArcEnabled{Val: true, Stat: policy.StatusSet},
	}

	if err := policyutil.ServeAndVerify(ctx, fakeDMS, cr, filesArcWarnPolicies); err != nil {
		s.Fatal("Failed to serve and verify policies: ", err)
	}

	if err := files.ClearDownloads(ctx, cr); err != nil {
		s.Error("Failed to clear Downloads directory: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}
	// Ensure that there are no windows open.
	if err := ash.CloseAllWindows(ctx, tconn); err != nil {
		s.Fatal("Failed to close all windows: ", err)
	}
	// Ensure that all windows are closed after test.
	defer ash.CloseAllWindows(cleanupCtx, tconn)

	// Setup Arc.
	a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC by policy: ", err)
	}
	defer a.Close(cleanupCtx)

	// Close all prior notifications.
	if err := ash.CloseNotifications(ctx, tconn); err != nil {
		s.Fatal("Failed to close notifications: ", err)
	}

	// Download the file.
	if err := files.DownloadFile(ctx, tconn, cr.Browser(), s.DataFileSystem()); err != nil {
		s.Fatal("Failed to download file: ", err)
	}

	// Open the Files app.
	filesApp, err := files.LaunchFilesAppFullscreen(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files App: ", err)
	}
	defer filesApp.Close(cleanupCtx)

	if err := filesApp.OpenDownloads()(ctx); err != nil {
		s.Fatal("Failed to open Downloads: ", err)
	}

	// Start interacting with the UI.
	ui := uiauto.New(tconn)

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, files.DlFileName, true); err != nil {
		s.Error("File isn't managed when it should be: ", err)
	}

	// Copy the file to Play files then cancel the warning.
	if err := copyToPlayfiles(ctx, ui, filesApp, files.DlFileName); err != nil {
		s.Fatal("Failed to copy the file: ", err)
	}

	if err := files.CancelWarningAndVerify(ctx, ui, tconn, files.DlFileName); err != nil {
		s.Fatal("Failed to cancel the warning: ", err)
	}

	// Copy the file to Play files then proceed the warning.
	if err := copyToPlayfiles(ctx, ui, filesApp, files.DlFileName); err != nil {
		s.Fatal("Failed to copy the file: ", err)
	}

	if err := files.AcceptWarningAndVerify(ctx, ui, tconn, files.DlFileName); err != nil {
		s.Fatal("Failed to proceed the warning: ", err)
	}

	if err := files.IsFileManaged(ctx, ui, tconn, keyboard, files.DlFileName, false); err != nil {
		s.Error("File is managed when it shouldn't be: ", err)
	}
}

// copyToPlayfiles tries to copy a file to Play files/Pictures.
func copyToPlayfiles(ctx context.Context, ui *uiauto.Context, f *filesapp.FilesApp, filename string) error {
	if err := uiauto.Combine("copy the file to Play files/Pictures",
		f.OpenDownloads(),
		f.CopyFileToClipboard(filename),
		f.OpenPlayfiles(),
		f.OpenFile("Pictures"),
		f.ClickDirectoryContextMenuItem("Pictures", "Paste into folder"),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to copy the file to Play files/Pictures")
	}

	return nil
}
