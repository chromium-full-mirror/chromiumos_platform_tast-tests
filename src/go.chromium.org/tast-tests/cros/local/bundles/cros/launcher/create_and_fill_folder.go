// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package launcher

import (
	"context"
	"io/ioutil"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CreateAndFillFolder,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test adding items to a folder in the launcher",
		Contacts: []string{
			"chromeos-launcher@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1288350",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-c6476142-1c4a-4e51-8e9b-bb5eaad4bba7",
		}},
		Timeout: 4 * time.Minute,
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

// CreateAndFillFolder tests that a folder can be filled to the maximum allowed size.
func CreateAndFillFolder(ctx context.Context, s *testing.State) {
	extDirBase, err := ioutil.TempDir("", "")
	if err != nil {
		s.Fatal("Failed to create a temporary directory: ", err)
	}
	defer os.RemoveAll(extDirBase)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Create 50 fake apps and get the the options to add to the new chrome session.
	opts, err := ash.GeneratePrepareFakeAppsOptions(extDirBase, 50)
	if err != nil {
		s.Fatal("Failed to create 50 fake apps")
	}

	// Creating fake apps and logging into a new session in this test ensures that enough apps will be available to folder.
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Chrome login failed: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	testCase := s.Param().(launcher.TestCase)
	tabletMode := testCase.TabletMode

	cleanup, err := launcher.SetUpLauncherTest(ctx, tconn, tabletMode, true /*stabilizeAppCount*/)
	if err != nil {
		s.Fatal("Failed to set up launcher test case: ", err)
	}
	defer cleanup(cleanupCtx)

	if err := launcher.CreateFolder(ctx, tconn); err != nil {
		s.Fatal("Failed to create folder app: ", err)
	}

	// The folder already has 2 items. Add 46 more items to get to the maximum folder size of 48 apps.
	if err := launcher.AddItemsToFolder(ctx, tconn, launcher.UnnamedFolderFinder, 46, tabletMode); err != nil {
		s.Fatal("Failed to add items to folder: ", err)
	}

	// Check that the number of apps in the folder is 48.
	size, err := launcher.GetFolderSize(ctx, tconn, launcher.UnnamedFolderFinder)
	if err != nil {
		s.Fatal("Failed to get the folder size: ", err)
	}
	if size != 48 {
		s.Fatalf("Unexpected number of items in folder, got %d, want %d", size, 48)
	}

	// Attempt to add one more item to the folder.
	if err := launcher.AddItemsToFolder(ctx, tconn, launcher.UnnamedFolderFinder, 1, tabletMode); err != nil {
		s.Fatal("Failed to add items to folder: ", err)
	}

	// Because the folder was already filled to the maximum size, the number of apps in the folder should still be 48.
	// Check that the folder size remains at the max of 48.
	size, err = launcher.GetFolderSize(ctx, tconn, launcher.UnnamedFolderFinder)
	if err != nil {
		s.Fatal("Failed to get the folder size: ", err)
	}
	if size != 48 {
		s.Fatalf("Unexpected number of items in folder, got %d, want %d", size, 48)
	}
}
