// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package filemanager

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/smb"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SMBFromSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify SMB mount can be added from OS Settings",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@chromium.org",
		},
		Attr: []string{
			"group:mainline",
			"informational",
		},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "smbStarted",
		SearchFlags: []*testing.StringPair{
			{
				Key:   "feature_id",
				Value: "screenplay-f72fb345-8214-4dc8-8c40-92a81a58f9de",
			},
		},
	})
}

func SMBFromSettings(ctx context.Context, s *testing.State) {
	fixt := s.FixtValue().(smb.FixtureData)

	// Give 5 seconds for screenshot.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	// Write a file to the folder that is being shared via samba.
	const textFile = "test.txt"
	testFileLocation := filepath.Join(fixt.GuestSharePath, textFile)
	if err := ioutil.WriteFile(testFileLocation, []byte("blahblah"), 0644); err != nil {
		s.Fatalf("Failed to create file %q: %s", testFileLocation, err)
	}
	defer os.Remove(testFileLocation)

	// Open the test API.
	tconn, err := fixt.Chrome.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Get a handle to the input keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard handle: ", err)
	}
	defer kb.Close(ctx)
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)
	addFileShareButton := nodewith.Name("Add file share").Role(role.Button)
	if _, err = ossettings.LaunchAtPageURL(ctx, tconn, fixt.Chrome, "smbShares", ui.Exists(addFileShareButton)); err != nil {
		s.Fatal("Failed to launch Settings: ", err)
	}

	if err := uiauto.Combine("add guestshare via OS Settings",
		ui.LeftClick(addFileShareButton),
		smb.AddFileShareAction(ui, kb, true /*=rememberPassword*/, smb.GuestShareName, "" /*=username*/, "" /*=password*/),
	)(ctx); err != nil {
		s.Fatal("Failed to add SMB share via OS Settings: ", err)
	}

	files, err := filesapp.App(ctx, tconn, apps.FilesSWA.ID)
	if err != nil {
		s.Fatal("Failed to connect to existing Files app: ", err)
	}

	if err := uiauto.Combine("ensure smb test file is visible",
		files.OpenPath(filesapp.FilesTitlePrefix+smb.GuestShareName, smb.GuestShareName),
		files.WaitForFile(textFile),
	)(ctx); err != nil {
		s.Fatal("Failed to ensure SMB test file is visible: ", err)
	}
}
