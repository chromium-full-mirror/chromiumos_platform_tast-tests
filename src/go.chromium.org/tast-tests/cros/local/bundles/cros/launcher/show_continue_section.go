// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/cws"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShowContinueSection,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that a local file shows to Continue Section",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-90e4fecc-d2ea-40dc-b9db-eb9d61089e22",
		}},
		Timeout: 3*time.Minute + cws.InstallationTimeout,
		Params: []testing.Param{{
			Name: "clamshell_mode",
			Val:  launcher.TestCase{TabletMode: false},
		}, {
			Name:              "tablet_mode",
			Val:               launcher.TestCase{TabletMode: true},
			ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay()),
		}},
	})
}

func ShowContinueSection(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	opt := chrome.EnableFeatures("ProductivityLauncher:enable_continue/true")

	// Start a new chrome session to avoid reusing user sessions and verify that the privacy nudge gets shown.
	cr, err := chrome.New(ctx, opt)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)
	testCase := s.Param().(launcher.TestCase)
	tabletMode := testCase.TabletMode

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, tabletMode, false /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	// If the sort nudge is shown, trigger sort to hide the sort nudge.
	if err := launcher.DismissSortNudgeIfExists(ctx, tconn); err != nil {
		s.Fatal("Failed to dismiss sort nudge: ", err)
	}

	// Create temp files and open them via Files app to populate the continue section.
	cleanupFiles, testFileNames, err := launcher.SetupContinueSectionFiles(
		ctx, tconn, cr, tabletMode)
	if err != nil {
		s.Fatal("Failed to set up continue section: ", err)
	}
	defer cleanupFiles()

	if err := launcher.OpenProductivityLauncher(ctx, tconn, tabletMode); err != nil {
		s.Fatal("Failed to open launcher: ", err)
	}

	continueSection := nodewith.ClassName("ContinueSectionView")
	if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(continueSection)(ctx); err != nil {
		s.Fatal("Failed to show continue section: ", err)
	}

	// Dismiss the privacy notice.
	if err := launcher.DismissPrivacyNotice(ctx, tconn); err != nil {
		s.Fatal("Failed to dismiss privacy notice: ", err)
	}

	for i, filePath := range testFileNames {
		// Opening continue section task closes launcher, so the launcher needs to
		// be re-opened for test iterations other than the initial one.
		if !tabletMode && i > 0 {
			if err := launcher.OpenProductivityLauncher(ctx, tconn, tabletMode); err != nil {
				s.Fatal("Failed to open launcher: ", err)
			}
		}
		// If the continue section is shown, then we don't need to try to re open the launcher.
		fileContent := fmt.Sprintf("Test file %d", i)
		if err := openFileFromContinueSection(ctx, tconn, tabletMode, filePath, fileContent); err != nil {
			s.Fatalf("Failed to open task %d - %s: %v", i, filePath, err)
		}
	}
}

func openFileFromContinueSection(ctx context.Context, tconn *chrome.TestConn, tabletMode bool, filePath, fileContent string) error {
	ui := uiauto.New(tconn)
	chromeApp, err := apps.ChromeOrChromium(ctx, tconn)
	if err != nil {
		return errors.Wrap(err, "unable to determine chrome app")
	}

	// If the continue section is shown, then we don't need to try to re open the launcher.
	continueSection := nodewith.ClassName("ContinueSectionView")
	if err := ui.WithTimeout(3 * time.Second).WaitUntilExists(continueSection)(ctx); err != nil {
		return errors.Wrap(err, "continue section not visible")
	}

	continueTask := nodewith.Ancestor(continueSection).Name(filePath)
	if err := uiauto.Combine("Open file task",
		ui.WithTimeout(3*time.Second).WaitUntilExists(continueTask),
		ui.LeftClick(continueTask),
	)(ctx); err != nil {
		return errors.Wrap(err, "failed to open the task on continue section")
	}

	if err := ash.WaitForApp(ctx, tconn, chromeApp.ID, 10*time.Second); err != nil {
		return errors.Wrap(err, "browser window never opened")
	}

	fileContentNode := nodewith.Name(fileContent).First()
	if err := ui.WaitUntilExists(fileContentNode)(ctx); err != nil {
		return errors.Wrap(err, "failed to find the file contents node")
	}

	if err := apps.Close(ctx, tconn, chromeApp.ID); err != nil {
		return errors.Wrap(err, "failed to trigger browser close")
	}

	if err := ash.WaitForAppClosed(ctx, tconn, chromeApp.ID); err != nil {
		return errors.Wrap(err, "browser did not close succesfully")
	}
	return nil
}
