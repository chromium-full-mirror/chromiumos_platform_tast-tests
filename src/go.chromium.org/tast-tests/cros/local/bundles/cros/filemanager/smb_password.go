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

	"go.chromium.org/tast-tests/cros/common/action"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/filesapp"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/smb"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SMBPassword,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify an SMB share can be mounted and password remembered after Chrome restart",
		BugComponent: "b:167289",
		Contacts: []string{
			"chromeos-files-syd@google.com",
			"benreich@chromium.org",
		},
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "smbStartedWithoutChrome",
		Params: []testing.Param{{
			Name: "remember_password",
			Val:  true,
		}, {
			Name: "forget_password",
			Val:  false,
		}},
	})
}

func SMBPassword(ctx context.Context, s *testing.State) {
	const (
		smbUsername = "chronos"
		smbPassword = "test0000"
		shareName   = "secureshare"
		textFile    = "test.txt"
	)

	fixt := s.FixtValue().(smb.FixtureData)
	rememberPassword := s.Param().(bool)
	cr, err := chrome.New(ctx)
	if err != nil {
		s.Fatal("Failed to login to Chrome: ", err)
	}

	// Give 10 seconds to perform cleanup as the fixture does not manage
	// cleanup due to restarting the Chrome instance.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Open the test API.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the test API connection: ", err)
	}

	// Launch the files application.
	files, err := filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to launch the Files app: ", err)
	}

	// Unmount the SMB mount and close Chrome.
	defer func() {
		faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)
		if err := files.Close(cleanupCtx); err != nil {
			s.Log("Failed to close existing Files app window: ", err)
		}
		if cr != nil {
			if err := smb.UnmountAllSmbMounts(cleanupCtx, cr); err != nil {
				s.Fatal("Failed to unmount all SMB mounts: ", err)
			}
			if err := cr.Close(cleanupCtx); err != nil {
				s.Log("Failed to close Chrome: ", err)
			}
		}
	}()

	// Write a file to the folder that is being shared via samba.
	testFileLocation := filepath.Join(fixt.GuestSharePath, textFile)
	if err := ioutil.WriteFile(testFileLocation, []byte("blahblah"), 0644); err != nil {
		s.Fatalf("Failed to create file %q: %s", testFileLocation, err)
	}
	defer os.Remove(testFileLocation)

	// Get a handle to the input keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard handle: ", err)
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn)
	if err := uiauto.Combine("add secureshare via Files context menu",
		action.Retry( /*retries=*/ 5, uiauto.Combine("Repeatedly try open SMB share dialog",
			// Clicking the more menu item again just causes it to hide, use the ESC
			// key to hide it before clicking it again.
			kb.AccelAction("Esc"),
			files.WithTimeout(5*time.Second).ClickMoreMenuItem("Services", "SMB file share"),
		), /*timeout=*/ 5*time.Second),
		smb.AddFileShareAction(ui, kb, rememberPassword, shareName, smbUsername, smbPassword),
		files.OpenPath(filesapp.FilesTitlePrefix+shareName, shareName),
		files.WaitForFile(textFile),
	)(ctx); err != nil {
		s.Fatal("Failed to click add SMB share: ", err)
	}

	// Restart Chrome but ensure the local state is maintained, this emulates
	// a Chrome reboot to ensure the password is remembered.
	cr, err = chrome.New(ctx, chrome.KeepState())
	if err != nil {
		s.Fatal("Failed to login to Chrome: ", err)
	}

	// Open the test API.
	tconn, err = cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Launch the files application.
	files, err = filesapp.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Launching the Files App failed: ", err)
	}

	// Verify the Samba share does not prompt for a password.
	if err := uiauto.Combine("ensure secureshare is still available",
		files.OpenPath(filesapp.FilesTitlePrefix+shareName, shareName),
		enterCredentialsIfPasswordForgotten(tconn, kb, files, rememberPassword, smbUsername, smbPassword),
		files.WaitForFile(textFile),
	)(ctx); err != nil {
		s.Fatal("Failed to ensure secureshare is still available: ", err)
	}
}

// enterCredentialsIfPasswordForgotten ensures the password prompt is shown
// after a restart and then enter the credentials and press enter.
func enterCredentialsIfPasswordForgotten(tconn *chrome.TestConn, kb *input.KeyboardEventWriter, files *filesapp.FilesApp, rememberPassword bool, smbUsername, smbPassword string) uiauto.Action {
	return func(ctx context.Context) error {
		if rememberPassword {
			return nil
		}

		ui := uiauto.New(tconn)
		usernameTextbox := nodewith.Role(role.TextField).Name("Username")
		refreshButton := nodewith.Role(role.Button).Name("Refresh")

		return uiauto.Combine("wait for dialog and enter credentials",
			ui.WaitUntilExists(usernameTextbox),
			ui.LeftClick(usernameTextbox),
			kb.TypeAction(smbUsername),
			kb.AccelAction("Tab"),
			kb.TypeAction(smbPassword),
			kb.AccelAction("Enter"),
			ui.WaitUntilGone(usernameTextbox),
			files.WaitUntilExists(refreshButton),
			files.LeftClick(refreshButton),
		)(ctx)
	}

}
