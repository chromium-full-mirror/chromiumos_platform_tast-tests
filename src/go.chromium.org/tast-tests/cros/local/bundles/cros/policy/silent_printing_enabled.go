// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

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
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/printpreview"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: SilentPrintingEnabled,
		Desc: "Behavior of SilentPrintingEnabled policy, checking the correspoding menu item restriction and printing preview dialog after setting the policy",
		Contacts: []string{
			"chromeos-commercial-printing@google.com",
			"poromov@chromium.org",
		},
		// ChromeOS > Software > Commercial (Enterprise) > Printing
		BugComponent: "b:1111614",
		SoftwareDeps: []string{"chrome"},
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		Fixture: fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.SilentPrintingEnabled{}, pci.VerifiedFunctionalityUI),
		},
	})
}

// SilentPrintingEnabled tests the SilentPrintingEnabled policy.
func SilentPrintingEnabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Connect to Test API to use it with the UI library.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	for _, param := range []struct {
		name          string
		printSilently bool                          // printSilently indicates whether it should be possible to print the page.
		value         *policy.SilentPrintingEnabled // value is the value of the policy.
	}{
		{
			name:          "unset",
			printSilently: false,
			value:         &policy.SilentPrintingEnabled{Stat: policy.StatusUnset},
		},
		{
			name:          "enabled",
			printSilently: true,
			value:         &policy.SilentPrintingEnabled{Val: true},
		},
		{
			name:          "disabled",
			printSilently: false,
			value:         &policy.SilentPrintingEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {
			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Clear Downloads directory.
			downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
			if err != nil {
				s.Fatal("Failed to get user's Download path: ", err)
			}
			files, err := os.ReadDir(downloadsPath)
			if err != nil {
				s.Fatal("Failed to get files from Downloads directory")
			}
			for _, file := range files {
				if err = os.RemoveAll(filepath.Join(downloadsPath, file.Name())); err != nil {
					s.Fatal("Failed to remove file: ", file.Name())
				}
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.value}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree_"+param.name)

			// Open an empty page in order to show Chrome UI.
			conn, err := cr.NewConn(ctx, "")
			if err != nil {
				s.Fatal("Failed to create an empty page: ", err)
			}
			defer conn.Close()

			// Wait for browser window.
			if err := ash.WaitForCondition(ctx, tconn, ash.BrowserTypeMatch(), nil); err != nil {
				s.Fatal("Unexpected window state: ", err)
			}

			// Define keyboard to type keyboard shortcut.
			kb, err := input.Keyboard(ctx)
			if err != nil {
				s.Fatal("Failed to get the keyboard: ", err)
			}
			defer kb.Close(ctx)

			// Type the shortcut.
			if err := kb.Accel(ctx, "Ctrl+P"); err != nil {
				s.Fatal("Failed to type printing hotkey: ", err)
			}

			// Check if printing dialog has appeared.
			ui := uiauto.New(tconn)
			finder := printpreview.PrintPreviewNode
			if err := ui.WithTimeout(10 * time.Second).WaitUntilExists(finder)(ctx); err != nil {
				// If function above failed, it could be either a timeout or an actual error. Check once again.
				_, err = ui.IsNodeFound(ctx, finder)
				// If the dialog does not exist by now, we assume that it will never be displayed.
				if err != nil {
					s.Fatal("Failed to check for printing windows existance: ", err)
				}
			}

			// GoBigSleepLint: Allow some time before validating silent printing
			// was done or not. A testing.Poll cannot be used here because we
			// won't be able to validate that printing was not done with that.
			testing.Sleep(ctx, 5*time.Second)

			getStringForGone := func(gone bool) string {
				if gone {
					return "gone"
				}
				return "present"
			}

			if previewDialogGone := ui.Gone(finder)(ctx) == nil; previewDialogGone != param.printSilently {
				s.Fatalf("Print preview dialog expected to be %s, but was %s", getStringForGone(param.printSilently), getStringForGone(previewDialogGone))
			}

			// Now check whether the file was saved silently or not.
			matches, err := filepath.Glob(filepath.Join(downloadsPath, "*.pdf"))
			if err != nil {
				s.Fatal("Failed to glob PDF files in Downloads: ", err)
			}
			lenMatches := len(matches)
			if lenMatches > 1 {
				s.Fatal("Got more than one printed file in Downloads")
			}
			fileSaved := lenMatches == 1
			getStringForFound := func(found bool) string {
				if found {
					return "found"
				}
				return "not found"
			}
			if fileSaved != param.printSilently {
				s.Fatalf("File expected to be %s, but was %s", getStringForFound(param.printSilently), getStringForFound(fileSaved))
			}
		})
	}
}
