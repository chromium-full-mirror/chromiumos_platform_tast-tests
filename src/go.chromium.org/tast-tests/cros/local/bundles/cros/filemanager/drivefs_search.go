// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"io/ioutil"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/drivefs"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DrivefsSearch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that exact file search for Google Drive returns correct value",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"austinct@chromium.org",
			"benreich@chromium.org",
		},
		SoftwareDeps: []string{
			"chrome",
			"chrome_internal",
			"drivefs",
			"gaia",
		},
		Attr: []string{
			"group:drivefs-cq",
			"group:hw_agnostic",
			"group:mainline",
			"informational",
		},
		Fixture: "driveFsStarted",
	})
}

func DrivefsSearch(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(*drivefs.FixtureData)
	tconn := fixt.TestAPIConn
	dfs := fixt.DriveFs

	// This test case is exercising the full-text search of DriveFS, keeping the name
	// fairly unique to avoid having it match as content search (not just file name).
	const fileName = "verify-full-text-search-functionality-drivefs"
	if err := ioutil.WriteFile(dfs.MyDrivePath(fileName), []byte("fake-content"), 0644); err != nil {
		s.Fatalf("Could not create the test file at %q: %v", dfs.MyDrivePath(fileName), err)
	}
	// Don't delete the test file after the test as there may not be enough time
	// after the test for the deletion to be synced to Drive.

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
	defer dfs.SaveLogsOnError(cleanupCtx, s.HasError)

	// Launch Files App
	filesApp, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Could not launch the Files App: ", err)
	}

	// Navigate to Google Drive via the Files App ui.
	if err := filesApp.OpenDrive()(ctx); err != nil {
		s.Fatal("Could not open Google Drive folder: ", err)
	}

	// Searches files app for the search term passed in.
	// Do not have to wait for the file to sync as search happens on remote, not locally.
	ew, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Could not create keyboard: ", err)
	}
	if err := filesApp.Search(ew, fileName)(ctx); err != nil {
		s.Fatalf("Failed search for %q: %v", fileName, err)
	}

	// Get all the containers that contain the file names.
	result := nodewith.ClassName("filename-label").Role(role.GenericContainer)

	// We expect 1 file to be returned given the filename length of the seed file.
	if err := filesApp.WithPollOpts(testing.PollOptions{Timeout: 15 * time.Second, Interval: 2 * time.Second}).WaitUntilExists(result)(ctx); err != nil {
		s.Fatal("Failed waiting for search results: ", err)
	}

	// Get the actual text from the container.
	text := nodewith.Role(role.StaticText).Ancestor(result)

	info, err := filesApp.Info(ctx, text)
	if err != nil {
		s.Fatal("Failed getting info: ", err)
	}
	if info.Name != fileName {
		s.Fatalf("Failed as search result is incorrect: got %q, want %q: ", info.Name, fileName)
	}
}
