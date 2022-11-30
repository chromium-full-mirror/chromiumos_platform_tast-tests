// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package arcappcompat will have tast tests for android apps on Chromebooks.
package arcappcompat

import (
	"context"
	"time"

	"chromiumos/tast/common/android/ui"
	"chromiumos/tast/local/arc"
	"chromiumos/tast/local/bundles/cros/arcappcompat/pre"
	"chromiumos/tast/local/bundles/cros/arcappcompat/testutil"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
)

// clamshellLaunchForDiscord launches Discord in clamshell mode.
var clamshellLaunchForDiscord = []testutil.TestCase{
	{Name: "Launch app in Clamshell", Fn: launchAppForDiscord, Timeout: testutil.LaunchTestCaseTimeout},
}

// touchviewLaunchForDiscord launches Discord in tablet mode.
var touchviewLaunchForDiscord = []testutil.TestCase{
	{Name: "Launch app in Touchview", Fn: launchAppForDiscord, Timeout: testutil.LaunchTestCaseTimeout},
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         Discord,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Functional test for Discord that installs the app also verifies it is logged in and that the main page is open, checks Discord correctly changes the window state in both clamshell and touchview mode",
		Contacts:     []string{"cros-arc-apps-te@google.com"},
		// ChromeOS > Software > ARC++ > Apps Testing
		BugComponent: "b:1122984",
		Attr:         []string{"group:appcompat"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Name: "clamshell_mode_default",
			Val: testutil.TestParams{
				LaunchTests: clamshellLaunchForDiscord,
				CommonTests: testutil.ClamshellCommonTests,
			},
			ExtraAttr:         []string{"appcompat_default"},
			ExtraSoftwareDeps: []string{"android_p"},
			// TODO(b/189704585): Remove hwdep.SkipOnModel once the solution is found.
			// Skip on tablet only models.
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(testutil.TabletOnlyModels...)),
			Pre:               pre.AppCompatBootedUsingTestAccountPool,
		}, {
			Name: "tablet_mode_default",
			Val: testutil.TestParams{
				LaunchTests: touchviewLaunchForDiscord,
				CommonTests: testutil.TouchviewCommonTests,
			},
			ExtraAttr:         []string{"appcompat_default"},
			ExtraSoftwareDeps: []string{"android_p"},
			// TODO(b/189704585): Remove hwdep.SkipOnModel once the solution is found.
			// Skip on clamshell only models.
			ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen(), hwdep.SkipOnModel(testutil.ClamshellOnlyModels...)),
			Pre:               pre.AppCompatBootedInTabletModeUsingTestAccountPool,
		}, {
			Name: "vm_clamshell_mode_default",
			Val: testutil.TestParams{
				LaunchTests: clamshellLaunchForDiscord,
				CommonTests: testutil.ClamshellCommonTests,
			},
			ExtraAttr:         []string{"appcompat_default"},
			ExtraSoftwareDeps: []string{"android_vm"},
			// TODO(b/189704585): Remove hwdep.SkipOnModel once the solution is found.
			// Skip on tablet only models.
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(testutil.TabletOnlyModels...)),
			Pre:               pre.AppCompatBootedUsingTestAccountPool,
		}, {
			Name: "vm_tablet_mode_default",
			Val: testutil.TestParams{
				LaunchTests: touchviewLaunchForDiscord,
				CommonTests: testutil.TouchviewCommonTests,
			},
			ExtraAttr:         []string{"appcompat_default"},
			ExtraSoftwareDeps: []string{"android_vm"},
			// TODO(b/189704585): Remove hwdep.SkipOnModel once the solution is found.
			// Skip on clamshell only models.
			ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen(), hwdep.SkipOnModel(testutil.ClamshellOnlyModels...)),
			Pre:               pre.AppCompatBootedInTabletModeUsingTestAccountPool,
		}, {
			Name: "clamshell_mode_release",
			Val: testutil.TestParams{
				LaunchTests:  clamshellLaunchForDiscord,
				ReleaseTests: testutil.ClamshellReleaseTests,
			},
			ExtraAttr:         []string{"appcompat_release"},
			ExtraSoftwareDeps: []string{"android_p"},
			// TODO(b/189704585): Remove hwdep.SkipOnModel once the solution is found.
			// Skip on tablet only models.
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(testutil.TabletOnlyModels...)),
			Pre:               pre.AppCompatBootedUsingTestAccountPool,
		}, {
			Name: "tablet_mode_release",
			Val: testutil.TestParams{
				LaunchTests:  touchviewLaunchForDiscord,
				ReleaseTests: testutil.TouchviewReleaseTests,
			},
			ExtraAttr:         []string{"appcompat_release"},
			ExtraSoftwareDeps: []string{"android_p"},
			// TODO(b/189704585): Remove hwdep.SkipOnModel once the solution is found.
			// Skip on clamshell only models.
			ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen(), hwdep.SkipOnModel(testutil.ClamshellOnlyModels...)),
			Pre:               pre.AppCompatBootedInTabletModeUsingTestAccountPool,
		}, {
			Name: "vm_clamshell_mode_release",
			Val: testutil.TestParams{
				LaunchTests:  clamshellLaunchForDiscord,
				ReleaseTests: testutil.ClamshellReleaseTests,
			},
			ExtraAttr:         []string{"appcompat_release"},
			ExtraSoftwareDeps: []string{"android_vm"},
			// TODO(b/189704585): Remove hwdep.SkipOnModel once the solution is found.
			// Skip on tablet only models.
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(testutil.TabletOnlyModels...)),
			Pre:               pre.AppCompatBootedUsingTestAccountPool,
		}, {
			Name: "vm_tablet_mode_release",
			Val: testutil.TestParams{
				LaunchTests:  touchviewLaunchForDiscord,
				ReleaseTests: testutil.TouchviewReleaseTests,
			},
			ExtraAttr:         []string{"appcompat_release"},
			ExtraSoftwareDeps: []string{"android_vm"},
			// TODO(b/189704585): Remove hwdep.SkipOnModel once the solution is found.
			// Skip on clamshell only models.
			ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen(), hwdep.SkipOnModel(testutil.ClamshellOnlyModels...)),
			Pre:               pre.AppCompatBootedInTabletModeUsingTestAccountPool,
		}},
		Timeout: 30 * time.Minute,
		Vars:    []string{"arcappcompat.gaiaPoolDefault"},
		VarDeps: []string{"arcappcompat.Discord.emailid", "arcappcompat.Discord.password"},
	})
}

// Discord test uses library for opting into the playstore and installing app.
// Checks Discord correctly changes the window states in both clamshell and touchview mode.
func Discord(ctx context.Context, s *testing.State) {
	const (
		appPkgName  = "com.discord"
		appActivity = ".main.MainActivity"
	)
	testSet := s.Param().(testutil.TestParams)
	testutil.RunTestCases(ctx, s, appPkgName, appActivity, testSet)
}

// launchAppForDiscord verifies Discord is logged in and
// verify Discord reached main activity page of the app.
func launchAppForDiscord(ctx context.Context, s *testing.State, tconn *chrome.TestConn, a *arc.ARC, d *ui.Device, appPkgName, appActivity string) {
	const (
		homeIconID          = "com.discord:id/tabs_host_bottom_nav_friends_item"
		logInButtonText     = "Log In"
		logInButtonClass    = "android.widget.TextView"
		emailTextFieldID    = "login_login_input"
		passwordTextFieldID = "login_password_input"
		skipButtonText      = "Skip Challenge"
		skipButtonClass     = "android.widget.Button"
	)

	email := s.RequiredVar("arcappcompat.Discord.emailid")
	password := s.RequiredVar("arcappcompat.Discord.password")

	logInButton := d.Object(ui.ClassName(logInButtonClass), ui.Text(logInButtonText))
	if err := logInButton.WaitForExists(ctx, testutil.DefaultUITimeout); err != nil {
		s.Fatal("Button Log In doesn't exists: ", err)
	} else if err := logInButton.Click(ctx); err != nil {
		s.Fatal("Failed to click on Log In: ", err)
	}

	emailTextField := d.Object(ui.ID(emailTextFieldID))
	if err := emailTextField.WaitForExists(ctx, testutil.DefaultUITimeout); err != nil {
		s.Fatal("Email text field doesn't exists: ", err)
	} else if err := emailTextField.SetText(ctx, email); err != nil {
		s.Fatal("Failed to set text on email text field: ", err)
	}

	passwordTextField := d.Object(ui.ID(passwordTextFieldID))
	if err := passwordTextField.WaitForExists(ctx, testutil.DefaultUITimeout); err != nil {
		s.Fatal("Password text field doesn't exists: ", err)
	} else if err := passwordTextField.SetText(ctx, password); err != nil {
		s.Fatal("Failed to set text on password text field: ", err)
	}

	// Need to click Log In button again once we input email and password.
	if err := logInButton.WaitForExists(ctx, testutil.DefaultUITimeout); err != nil {
		s.Fatal("Button Log In doesn't exists after entering credential: ", err)
	} else if err := logInButton.Click(ctx); err != nil {
		s.Fatal("Failed to click on Log In after entering credential: ", err)
	}

	// Check for captcha and pass the test once we see the captcha.
	skipButton := d.Object(ui.ClassName(skipButtonClass), ui.Text(skipButtonText))
	if err := skipButton.WaitForExists(ctx, testutil.DefaultUITimeout); err == nil {
		s.Log("Captcha page exists")
		return
	}

	// Deal with the dialog boxes if we don't see captcha.
	testutil.HandleDialogBoxes(ctx, s, d, appPkgName)

	// Check for homePageVerifier after login .
	homePageVerifier := d.Object(ui.ID(homeIconID))
	if err := homePageVerifier.WaitForExists(ctx, testutil.LongUITimeout); err != nil {
		s.Fatal("homePageVerifier doesn't exist: ", err)
	}
}
