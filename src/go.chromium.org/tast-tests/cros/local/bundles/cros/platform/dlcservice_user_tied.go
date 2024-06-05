// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"context"
	"time"

	dlctest "go.chromium.org/tast-tests/cros/local/bundles/cros/platform/dlc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/dlc"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/session"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

const (
	userTiedID = "user-tied-dlc"

	username = "testuser@gmail.com"
	password = "testpass"

	stoppedState = "stopped"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DLCServiceUserTied,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that user-tied DLC can be installs/uninstalls when user logged-in, and unloads after user logged out",
		Contacts:     []string{"chromeos-core-services@google.com", "yuanpengni@chromium.org"},
		BugComponent: "b:908242",
		SoftwareDeps: []string{"dlc", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Attr:         []string{"group:mainline", "informational"},
	})
}

func DLCServiceUserTied(ctx context.Context, s *testing.State) {
	// Check dlcservice is up and running.
	if err := upstart.EnsureJobRunning(ctx, dlc.JobName); err != nil {
		s.Fatalf("Failed to ensure %s running: %v", dlc.JobName, err)
	}

	// Make sure that the user-tied DLC is not installed using GetInstalled DBus method.
	if isInstalled(ctx, s, userTiedID) {
		s.Fatal("Not continuing as ", userTiedID, " is already installed.")
	}

	// Login to install user-tied DLC.
	cr, err := chrome.New(ctx, chrome.FakeLogin(chrome.Creds{User: username, Pass: password}))
	if err != nil {
		s.Fatal("Failed to connect to Chrome: ", err)
	}
	defer func() {
		if cr != nil {
			cr.Close(ctx)
		}
	}()

	// TODO(b/345250918): Daemon-store mount not propagated to imageloader until it restart.
	upstart.RestartJob(ctx, "imageloader")

	// Install user-tied DLC without a Omaha URL since it is preloaded.
	if err := dlc.Install(ctx, userTiedID, ""); err != nil {
		s.Fatal("Install failed: ", err)
	}
	if err := dlctest.DumpAndVerifyInstalledDLCs(ctx, s.OutDir(), "install_user_tied", userTiedID); err != nil {
		s.Fatal("DumpAndVerifyInstalledDLCs failed: ", err)
	}

	// Logout to unload user-tied DLC.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection")
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to initialize keyboard: ", err)
	}
	defer kb.Close(ctx)

	sm, err := session.NewSessionManager(ctx)
	if err != nil {
		s.Fatal("Failed to connect to session manager: ", err)
	}

	sw, err := sm.WatchSessionStateChanged(ctx, stoppedState)
	if err != nil {
		s.Fatal("Failed to watch for D-Bus signals: ", err)
	}
	defer sw.Close(ctx)

	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		s.Fatal("Failed to logout: ", err)
	}

	s.Logf("Waiting for SessionStateChanged %q D-Bus signal from session_manager", stoppedState)
	select {
	case <-sw.Signals:
		s.Log("Got SessionStateChanged signal")
	case <-ctx.Done():
		s.Fatal("Didn't get SessionStateChanged signal: ", ctx.Err())
	}

	// Close Chrome to do some cleanup.
	// It will log some errors, as the session is closed already.
	cr.Close(ctx)
	cr = nil

	// Wait for UI restart and verify user-tied DLC unloading.
	if cr, err = chrome.New(ctx,
		chrome.ExtraArgs("--skip-force-online-signin-for-testing"),
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	); err != nil {
		s.Fatal("Failed to restart Chrome for testing: ", err)
	}

	if tconn, err = cr.SigninProfileTestAPIConn(ctx); err != nil {
		s.Fatal("Failed to re-establish test API connection")
	}

	if err := lockscreen.WaitForPasswordEntry(ctx, tconn, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for login screen: ", err)
	}

	// User-tied DLC should be unloaded after logout.
	if isInstalled(ctx, s, userTiedID) {
		s.Fatal("Unload failed: ", userTiedID, " is still in installed state")
	}

	// Login again and uninstall user-tied DLC.
	if err := lockscreen.EnterPassword(ctx, tconn, username, password, kb); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	if _, err := lockscreen.WaitState(ctx, tconn, func(st lockscreen.State) bool { return st.LoggedIn }, 10*time.Second); err != nil {
		s.Fatal("Failed to log in: ", err)
	}

	// Uninstall user-tied DLC.
	if err := dlc.Uninstall(ctx, userTiedID); err != nil {
		s.Fatal("Uninstall failed: ", err)
	}
	if isInstalled(ctx, s, userTiedID) {
		s.Fatal("Uninstall failed: ", userTiedID, " is still in installed state")
	}
}

func isInstalled(ctx context.Context, s *testing.State, id string) bool {
	dlcListOutputs, err := dlctest.GetInstalled(ctx)
	if err != nil {
		s.Fatal("GetInstall failed: ", err)
	}
	for _, dlcListOutput := range dlcListOutputs {
		if dlcListOutput.ID == id {
			return true
		}
	}
	return false
}
