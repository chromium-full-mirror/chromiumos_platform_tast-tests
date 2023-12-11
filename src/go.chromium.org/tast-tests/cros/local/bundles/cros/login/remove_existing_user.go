// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package login

import (
	"context"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         RemoveExistingUser,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Remove user pods from start screen",
		Contacts: []string{
			"cros-lurs@google.com",
			"dkuzmin@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1207311", // ChromeOS > Software > Commercial (Enterprise) > Identity > LURS
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:hw_agnostic"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		Timeout: 5*chrome.LoginTimeout + userutil.TakingOwnershipTimeout + 25*time.Second,
	})
}

func RemoveExistingUser(ctx context.Context, s *testing.State) {
	const (
		user1    = "user1@gmail.com"
		user2    = "user2@gmail.com"
		user3    = "user3@gmail.com"
		password = "password"
	)
	cleanUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 20*time.Second)
	defer cancel()

	if err := userutil.CreateDeviceOwner(ctx, user1, password); err != nil {
		s.Fatal("Failed to create new user1: ", err)
	}
	if err := userutil.CreateUser(ctx, user2, password, chrome.KeepState()); err != nil {
		s.Fatal("Failed to create new user2: ", err)
	}
	if err := userutil.CreateUser(ctx, user3, password, chrome.KeepState()); err != nil {
		s.Fatal("Failed to create new user3: ", err)
	}

	cr, err := chrome.New(
		ctx,
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanUpCtx)

	// Connect to login extension.
	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanUpCtx, s.OutDir(), s.HasError, tLoginConn)

	if err := userutil.RemoveUserOnLoginScreen(ctx, tLoginConn, cr, user3); err != nil {
		s.Fatal("Failed to remove user on login screen: ", err)
	}

	if err := userutil.CheckDeviceOwnerIsNotRemoved(ctx, tLoginConn, cr, user1); err != nil {
		s.Fatal("Failed to check owner is not removed: ", err)
	}

	// Check that there is no user3 in LoggedInUsers list.
	knownEmails, err := userutil.GetKnownEmailsFromLocalState()
	if err != nil {
		s.Fatal("Failed to get known emails from local state: ", err)
	}
	if knownEmails[user3] {
		s.Fatal("Removed user is still in LoggedInUsers list")
	}

	// Check that cryptohome for user3 was deleted.
	path, err := cryptohome.UserPath(ctx, user3)
	if _, err := os.Stat(path); err == nil {
		s.Fatal("Cryptohome directory still exists under ", path)
	} else if !os.IsNotExist(err) {
		s.Fatal("Unexpected error: ", err)
	}

	ui := uiauto.New(tLoginConn)
	// Wait for user pods to be available.
	if err := ui.WaitUntilExists(nodewith.Name(user1).Role(role.Button))(ctx); err != nil {
		s.Fatal("Failed to wait for user pods to be available after reboot: ", err)
	}
	if err := ui.WaitUntilExists(nodewith.Name(user2).Role(role.Button))(ctx); err != nil {
		s.Fatal("Failed to wait for user pods to be available after reboot: ", err)
	}
	// Check that there is no user pod for user3.
	if err := ui.Gone(nodewith.Name(user3).Role(role.Button))(ctx); err != nil {
		s.Fatal("Removed user pod for " + user3 + " still exists")
	}
}
