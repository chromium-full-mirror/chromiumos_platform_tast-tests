// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package lacros

import (
	"context"
	"time"

	"chromiumos/tast/ctxutil"
	"chromiumos/tast/errors"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/ash"
	"chromiumos/tast/local/chrome/lacros"
	"chromiumos/tast/local/chrome/lacros/lacrosfixt"
	"chromiumos/tast/local/chrome/lacros/lacrosinfo"
	"chromiumos/tast/local/chrome/lacros/lacrosproc"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/lockscreen"
	"chromiumos/tast/local/chrome/userutil"
	"chromiumos/tast/local/input"
	"chromiumos/tast/local/upstart"
	"chromiumos/tast/testing"
)

type loginScreenLaunchTestParam struct {
	lacrosSelection lacros.Selection
	lacrosMode      lacros.Mode
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
		BugComponent: "crbug:OS>LaCrOS",
		SoftwareDeps: []string{"chrome", "lacros"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
			"ui.gaiaPoolDefault",
		},

		// Login time + User ownership + Wait for password entry + Wait for Lacros processes:
		Timeout: 2*chrome.GAIALoginTimeout + userutil.TakingOwnershipTimeout + time.Minute + 20*time.Second,

		Params: []testing.Param{{
			Name:      "rootfs",
			ExtraAttr: []string{"group:mainline", "informational"},
			Val: loginScreenLaunchTestParam{
				lacros.Rootfs,
				lacros.LacrosOnly,
			},
		},
		// Disabled, per b/246818834.
		// {
		//	Name: "omaha",
		//	Val: loginScreenLaunchTestParam{
		//		lacros.Omaha,
		//		lacros.LacrosOnly,
		//	},
		// }
		},
	})
}

// initUserPod logs in and out to create a user pod on the login screen.
func initUserPod(ctx context.Context, gaiaPoolDefault string) (chrome.Creds, error) {
	cr, err := chrome.New(ctx, chrome.GAIALoginPool(gaiaPoolDefault))
	if err != nil {
		return chrome.Creds{}, errors.Wrap(err, "chrome login failed")
	}
	defer cr.Close(ctx)
	creds := cr.Creds()

	// This is needed for reven tests, as login flow there relies on the existence of a device setting.
	if err := userutil.WaitForOwnership(ctx, cr); err != nil {
		return chrome.Creds{}, errors.Wrap(err, "user did not become device owner")
	}

	if err := upstart.RestartJob(ctx, "ui"); err != nil {
		return chrome.Creds{}, errors.Wrap(err, "failed to restart ui")
	}

	return creds, err
}

func setupChromeOpts(signinProfileTestExtensionManifestKey string,
	lacrosSelection lacros.Selection, lacrosMode lacros.Mode) ([]chrome.Option, error) {
	// chrome.NoLogin() and chrome.KeepState() are needed to show the login
	// screen with a user pod (instead of the OOBE login screen).
	// |signinProfileTestExtensionManifestKey| is needed to launch Chrome at OOBE.
	// We disable profile migration to prevent Ash from restarting and breaking the test connection.
	options := []chrome.Option{
		chrome.NoLogin(),
		chrome.KeepState(),
		chrome.LoadSigninProfileExtension(signinProfileTestExtensionManifestKey),
		chrome.EnableFeatures("LacrosLaunchAtLoginScreen"),
		chrome.EnableFeatures("LacrosProfileMigrationForceOff"),
	}

	// Add Lacros options.
	lacrosCfg := lacrosfixt.NewConfig(lacrosfixt.Selection(lacrosSelection), lacrosfixt.Mode(lacrosMode))
	lacrosOpts, err := lacrosCfg.Opts()
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Lacros options")
	}

	return append(options, lacrosOpts...), nil
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

// runningLacrosProcs returns a map from PID to executable path of all the Lacros processes running.
func runningLacrosProcs(ctx context.Context, lacrosPath string) (map[int32]string, error) {
	procs, err := lacrosproc.ProcsFromPath(ctx, lacrosPath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to get Lacros process listing")
	}

	lacrosProcs := make(map[int32]string)
	for _, proc := range procs {
		if exe, err := proc.Exe(); err == nil {
			lacrosProcs[proc.Pid] = exe
		}
	}

	return lacrosProcs, nil
}

// waitForLacrosProcs waits until Lacros processes are running, then returns all of them.
func waitForLacrosProcs(ctx context.Context, lacrosPath string) (lacrosProcs map[int32]string, err error) {
	// NOTE: depending on timing, it is possible, although not likely, that this function
	// will return only the first Lacros process, before the children processes are spawned.
	// That's ok, as that process will be the browser process and if it persists across logins,
	// it means pre-launching and resuming succeeded.
	if err = testing.Poll(ctx, func(ctx context.Context) error {
		lacrosProcs, err = runningLacrosProcs(ctx, lacrosPath)
		if err != nil {
			return err
		}
		if len(lacrosProcs) == 0 {
			return errors.New("lacros is not yet running (no lacros processes)")
		}
		return nil
	}, &testing.PollOptions{Timeout: 10 * time.Second, Interval: time.Second}); err != nil {
		return lacrosProcs, errors.Wrap(err, "lacros is not running (no lacros processes)")
	}
	return lacrosProcs, nil
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

	// Setup Chrome options.
	params := s.Param().(loginScreenLaunchTestParam)
	options, err := setupChromeOpts(s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		params.lacrosSelection, params.lacrosMode)
	if err != nil {
		s.Fatal("Failed to setup Chrome options: ", err)
	}

	// Shorten context a bit to allow for cleanup.
	closeCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Launch Chrome.
	cr, err := chrome.New(ctx, options...)
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
	if err != nil || len(info.LacrosPath) == 0 {
		s.Fatal("Failed to get Lacros path: ", err)
	}
	lacrosProcsAtLoginScreen, err := waitForLacrosProcs(ctx, info.LacrosPath)
	if err != nil {
		s.Fatal("Failed to get Lacros processes at login screen: ", err)
	}
	testing.ContextLog(ctx, "Lacros processes at login screen:")
	for pid, exe := range lacrosProcsAtLoginScreen {
		testing.ContextLogf(ctx, "  %d: %s", pid, exe)
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
	lacrosProcsAfterLogin, err := runningLacrosProcs(ctx, info.LacrosPath)
	if err != nil {
		s.Fatal("Failed to get Lacros processes after login: ", err)
	}
	testing.ContextLog(ctx, "Lacros processes after login:")
	for pid, exe := range lacrosProcsAfterLogin {
		testing.ContextLogf(ctx, "  %d: %s", pid, exe)
	}
	// Check that they are a superset of the ones that were running at login screen.
	if !isSubset(lacrosProcsAtLoginScreen, lacrosProcsAfterLogin) {
		s.Fatal("Processes running after login are not the ones that were running at login screen")
	}

	// Check that Lacros's connection works.
	if _, err = lacros.Connect(ctx, tConn); err != nil {
		s.Fatal("Could not connect to Lacros after login: ", err)
	}
}
