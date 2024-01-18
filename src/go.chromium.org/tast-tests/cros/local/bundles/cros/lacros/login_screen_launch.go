// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosinfo"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
)

type loginScreenLaunchTestParam struct {
	browserType     browser.Type
	lacrosSelection lacros.Selection
	forkZygotes     bool
	keepAlive       bool // Ignored when browserType == TypeAsh.
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LoginScreenLaunch,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Tests that Lacros correctly prelaunches at login screen",
		Contacts: []string{
			"lacros-team@google.com",
			"andreaorru@chromium.org",
			"hidehiko@chromium.org",
		},
		BugComponent: "b:1456869",
		SoftwareDeps: []string{"chrome", "chrome_internal", "lacros"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"ui.gaiaPoolDefault",
		},

		// Login time + User ownership + Wait for password entry + Wait for Lacros processes:
		Timeout: 2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute + 20*time.Second,

		Params: []testing.Param{
			{
				Name:      "rootfs",
				ExtraAttr: []string{"group:mainline", "informational"},
				Val: loginScreenLaunchTestParam{
					browser.TypeLacros,
					lacros.Rootfs,
					false, // Don't fork Zygotes.
					false, // keepAlive disabled
				},
			},
			{
				Name:      "rootfs_zygotes",
				ExtraAttr: []string{"group:mainline", "informational"},
				Val: loginScreenLaunchTestParam{
					browser.TypeLacros,
					lacros.Rootfs,
					true,  // Fork Zygotes.
					false, // keepAlive disabled
				},
			},
			{
				Name:      "rootfs_keepalive",
				ExtraAttr: []string{"group:mainline", "informational"},
				Val: loginScreenLaunchTestParam{
					browser.TypeLacros,
					lacros.Rootfs,
					false, // Don't fork Zygotes.
					true,  // keepAlive enabled
				},
			},
			{
				Name:      "rootfs_disabled",
				ExtraAttr: []string{"group:mainline", "informational"},
				Val: loginScreenLaunchTestParam{
					browser.TypeAsh,
					lacros.Rootfs,
					false, // Don't fork Zygotes.
					false, // ignored
				},
			},
		},
	})
}

// initUserPod logs in and out to create a user pod on the login screen.
func initUserPod(ctx context.Context, gaiaPoolDefault string) (chrome.Creds, error) {
	cr, err := chrome.New(ctx, chrome.GAIALoginPool(gaiaPoolDefault))
	if err != nil {
		return chrome.Creds{}, errors.Wrap(err, "chrome login failed")
	}
	defer func() {
		if cr != nil {
			cr.Close(ctx)
		}
	}()
	creds := cr.Creds()

	// This is needed for reven tests, as login flow there relies on the existence of a device setting.
	if err := userutil.WaitForOwnership(ctx, cr); err != nil {
		return chrome.Creds{}, errors.Wrap(err, "user did not become device owner")
	}

	// Close before restarting.
	cr.Close(ctx)
	cr = nil

	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to restart ui")
	}

	return creds, err
}

// waitForPasswordEntry waits for the login screen to be ready for password entry.
func waitForPasswordEntry(ctx context.Context, tConn *chrome.TestConn) error {
	if st, err := lockscreen.WaitState(ctx, tConn, func(st lockscreen.State) bool { return st.ReadyForPassword }, 30*time.Second); err != nil {
		return errors.Wrapf(err, "failed waiting for the login screen to be ready for password entry: last state: %+v", st)
	}
	if err := lockscreen.WaitForPasswordEntry(ctx, tConn, 30*time.Second); err != nil {
		return errors.Wrap(err, "failed waiting for the login screen to be ready for password entry")
	}
	return nil
}

// inputPassword enters the password at the login screen, and logs in.
func inputPassword(ctx context.Context, tConn *chrome.TestConn, creds chrome.Creds) error {
	// Get keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		return errors.Wrap(err, "failed to get keyboard")
	}
	defer kb.Close(ctx)

	// Simulate entering the password.
	if err := lockscreen.EnterPassword(ctx, tConn, creds.User, creds.Pass, kb); err != nil {
		return errors.Wrap(err, "failed to enter password")
	}

	return nil
}

// waitForShelf checks if the login was successful via API, and by looking for the shelf in the UI.
func waitForShelf(ctx context.Context, tConn *chrome.TestConn) error {
	if err := lockscreen.WaitForLoggedIn(ctx, tConn, chrome.LoginTimeout); err != nil {
		return errors.Wrap(err, "failed to login")
	}

	if err := ash.WaitForShelf(ctx, tConn, 30*time.Second); err != nil {
		return errors.Wrap(err, "shelf did not appear after logging in")
	}

	return nil
}

// runningLacrosProcs returns a map from PID to executable path of all the `chrome` Lacros processes running.
// Excludes other binaries such as `chrome_crashpad_handler` and `nacl_helper`.
func runningLacrosProcs(ctx context.Context, lacrosPath string) (map[int32]string, error) {
	procs, err := lacrosproc.ProcsFromPath(ctx, lacrosPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Lacros process listing")
	}

	lacrosProcs := make(map[int32]string)
	for _, proc := range procs {
		if name, err := proc.Name(); err == nil && name == "chrome" {
			if exe, err := proc.Exe(); err == nil {
				lacrosProcs[proc.Pid] = exe
			}
		}
	}

	return lacrosProcs, nil
}

// waitForLacrosProcs waits until at least minProcesses Lacros processes are running, then returns all of them.
func waitForLacrosProcs(ctx context.Context, lacrosPath string, minProcesses int) (lacrosProcs map[int32]string, err error) {
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		lacrosProcs, err = runningLacrosProcs(ctx, lacrosPath)
		if err != nil {
			return err
		}
		if len(lacrosProcs) < minProcesses {
			return errors.Errorf("expected %d lacros processes, %d found", minProcesses, len(lacrosProcs))
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return lacrosProcs, errors.Wrap(err, "lacros processes not running")
	}
	return lacrosProcs, nil
}

// waitForLacrosProcsTermination waits until Lacros processes are terminated.
func waitForLacrosProcsTermination(ctx context.Context, lacrosPath string) (lacrosProcs map[int32]string, err error) {
	err = testing.Poll(ctx, func(ctx context.Context) error {
		lacrosProcs, err = runningLacrosProcs(ctx, lacrosPath)
		if err != nil {
			return err
		}
		if len(lacrosProcs) != 0 {
			return errors.New("lacros is still running (some processes did not terminate)")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second})
	return lacrosProcs, err
}

// isSubset checks if |subset| is a subset of |superset|.
func isSubset(subset, superset map[int32]string) bool {
	for key, value := range subset {
		if supersetValue, ok := superset[key]; !ok || supersetValue != value {
			return false
		}
	}
	return true
}

func LoginScreenLaunch(ctx context.Context, s *testing.State) {
	// Create user pod on login screen.
	creds, err := initUserPod(ctx, s.RequiredVar("ui.gaiaPoolDefault"))
	if err != nil {
		s.Fatal("Failed to create user pod on login screen: ", err)
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Setup Chrome options.
	// chrome.NoLogin() and chrome.KeepState() are needed to show the login
	// screen with a user pod (instead of the OOBE login screen).
	// |signinProfileTestExtensionManifestKey| is needed to launch Chrome at OOBE.
	// We disable profile migration to prevent Ash from restarting and breaking the test connection.
	options := []chrome.Option{
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
		chrome.EnableFeatures("LacrosLaunchAtLoginScreen"),
		chrome.EnableFeatures("LacrosProfileMigrationForceOff"),
		// Prelaunch Lacros regardless of whether there are users with Lacros enabled.
		// We restart Chrome quickly after the first login, so the preference is
		// sometimes not written to disk.
		chrome.ExtraArgs("--force-lacros-launch-at-login-screen-for-testing"),
	}

	// Fork zygotes at login screen if the corresponding param is enabled.
	params := s.Param().(loginScreenLaunchTestParam)
	if params.forkZygotes {
		options = append(options, chrome.EnableFeatures("LacrosForkZygotesAtLoginScreen"))
	}

	// Setup Lacros configuration.
	lacrosCfg := lacrosfixt.NewConfig(
		lacrosfixt.Selection(params.lacrosSelection),
		lacrosfixt.KeepAlive(params.keepAlive))

	// Launch Chrome.
	cr, err := browserfixt.NewChrome(ctx, params.browserType, lacrosCfg, options...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(closeCtx)

	// Setup login test API connection.
	tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating login test API connection failed: ", err)
	}
	defer faillog.DumpUITreeOnError(closeCtx, s.OutDir(), s.HasError, tLoginConn)

	// Wait for the login prompt to be available.
	if err = waitForPasswordEntry(ctx, tLoginConn); err != nil {
		s.Fatal("Failed waiting for the login prompt: ", err)
	}

	// Gather the Lacros processes that are running at login screen.
	info, err := lacrosinfo.Snapshot(ctx, tLoginConn)
	if err != nil {
		s.Fatal("Failed to get lacrosinfo.Snapshot: ", err)
	}
	if len(info.LacrosPath) == 0 {
		s.Fatal("Failed to get Lacros path")
	}

	// If Zygotes are forking at login screen, there will be 3 additional Chrome processes.
	expectedProcesses := 1
	if params.forkZygotes {
		expectedProcesses += 3
	}
	lacrosProcsAtLoginScreen, err := waitForLacrosProcs(ctx, info.LacrosPath, expectedProcesses)
	if err != nil {
		s.Fatal("Failed to get Lacros processes at login screen: ", err)
	}
	// Log the running Lacros processes.
	s.Log("Lacros processes at login screen:")
	for pid, exe := range lacrosProcsAtLoginScreen {
		s.Logf("  %d: %s", pid, exe)
	}

	// Input the password and login.
	if err = inputPassword(ctx, tLoginConn, creds); err != nil {
		s.Fatal("Failed to input password and login: ", err)
	}

	// Ensure login was successful and shelf is visible.
	tConn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}
	if err = waitForShelf(ctx, tConn); err != nil {
		s.Fatal("Failed waiting for the shelf to be visible: ", err)
	}

	// Gather the Lacros processes running after login has been completed.
	if params.browserType == browser.TypeLacros {
		// If Lacros is enabled for the user, Lacros should be running in the session.
		lacrosProcsAfterLogin, err := waitForLacrosProcs(ctx, info.LacrosPath, len(lacrosProcsAtLoginScreen))
		// Couldn't get the processes, or there are no Lacros processes.
		if err != nil {
			s.Fatal("Failed to get Lacros processes after login: ", err)
		}
		// Log the running Lacros processes.
		s.Log("Lacros processes after login:")
		for pid, exe := range lacrosProcsAfterLogin {
			s.Logf("  %d: %s", pid, exe)
		}
		// Check that the processes are a superset of the ones that were running at login screen.
		if !isSubset(lacrosProcsAtLoginScreen, lacrosProcsAfterLogin) {
			s.Fatal("Processes running after login are not the ones that were running at login screen")
		}
		// Open Lacros from shelf.
		browser, err := apps.PrimaryBrowser(ctx, tConn)
		if err != nil {
			s.Fatal("Failed to get browser app: ", err)
		}
		if ash.LaunchAppFromShelf(ctx, tConn, browser.Name, browser.ID); err != nil {
			s.Fatal("Failed to launch Lacros from shelf: ", err)
		}
		// Check that the Lacros window is visible.
		if err := lacros.WaitForLacrosWindow(ctx, tConn, "New Tab"); err != nil {
			s.Fatal("Failed waiting for Lacros window to be visible: ", err)
		}
		// Check that Lacros's connection works.
		if _, err = lacros.Connect(ctx, tConn); err != nil {
			s.Fatal("Could not connect to Lacros: ", err)
		}
	} else {
		// If Lacros is disabled for the user, Lacros should have been terminated.
		lacrosProcsAfterLogin, err := waitForLacrosProcsTermination(ctx, info.LacrosPath)
		// Couldn't get the processes, or there are still Lacros processes running.
		if err != nil {
			// Log the running Lacros processes.
			s.Log("Lacros processes after login:")
			for pid, exe := range lacrosProcsAfterLogin {
				s.Logf("  %d: %s", pid, exe)
			}
			s.Fatal("Failed to detect Lacros processes termination after login: ", err)
		}
	}
}
