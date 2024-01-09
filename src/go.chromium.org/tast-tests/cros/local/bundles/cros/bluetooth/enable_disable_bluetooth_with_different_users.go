// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/bluetooth/floss"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type enableDisableBluetoothWithDifferentUsersParams struct {
	btImpl          bluetooth.Bluetooth
	enableFeatures  []string
	disableFeatures []string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:           EnableDisableBluetoothWithDifferentUsers,
		LacrosStatus:   testing.LacrosVariantUnneeded,
		LifeCycleStage: testing.LifeCycleOwnerMonitored,
		Desc:           "Checks that the Bluetooth adapter state preference is preserved for the device and users",
		Contacts: []string{
			"cros-connectivity@google.com",
			"gordonseto@google.com",
		},
		BugComponent: "b:1131776", // ChromeOS > Software > System Services > Connectivity > Bluetooth
		Attr:         []string{"group:bluetooth"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Bluetooth()),
		Fixture:      fixture.CleanOwnership,
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Params: []testing.Param{
			{
				Name: "floss_disabled",
				Val: enableDisableBluetoothWithDifferentUsersParams{
					btImpl:          &bluez.BlueZ{},
					disableFeatures: []string{"Floss"},
				},
				ExtraAttr: []string{"bluetooth_flaky"},
			}, {
				Name: "floss_enabled",
				Val: enableDisableBluetoothWithDifferentUsersParams{
					btImpl:         &floss.Floss{},
					enableFeatures: []string{"Floss"},
				},
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
		Timeout: time.Minute * 5,
	})
}

// EnableDisableBluetoothWithDifferentUsers tests that the device's and users' last Bluetooth adapter states are preserved and restored between user sessions.
func EnableDisableBluetoothWithDifferentUsers(ctx context.Context, s *testing.State) {
	// Ensure the adapter state is cleaned up at the end of the test.
	bt := s.Param().(enableDisableBluetoothWithDifferentUsersParams).btImpl
	defer bt.Enable(ctx)

	enableFeatures := chrome.EnableFeatures(s.Param().(enableDisableBluetoothWithDifferentUsersParams).enableFeatures...)
	disableFeatures := chrome.DisableFeatures(s.Param().(enableDisableBluetoothWithDifferentUsersParams).disableFeatures...)

	// Create a device owner.
	user1 := chrome.Creds{User: "test_owner@gmail.com", Pass: "test0000"}
	s.Log("Creating user1 pod: ", user1.User)
	if err := userutil.CreateDeviceOwner(ctx, user1.User, user1.Pass, enableFeatures, disableFeatures); err != nil {
		s.Fatal("Failed to create device owner, user1: ", err)
	}

	// Create a second user.
	user2 := chrome.Creds{User: "test_user2@gmail.com", Pass: "test0000"}
	s.Log("Creating user2 pod: ", user2.User)
	if err := userutil.CreateUser(ctx, user2.User, user2.Pass, chrome.KeepState(), enableFeatures, disableFeatures); err != nil {
		s.Fatal("Failed to create user2: ", err)
	}

	// Go to sign-in screen.
	cleanupCtx := ctx
	signInProfileTestExtension := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")
	cr, err := signOut(ctx, signInProfileTestExtension, enableFeatures, disableFeatures)
	if err != nil {
		s.Fatal("Failed to go to sign-in screen: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Ensure Bluetooth starts enabled.
	if err := bt.Enable(ctx); err != nil {
		s.Fatal("Failed to enable Bluetooth: ", err)
	}
	if err := bt.PollForAdapterState(ctx, true); err != nil {
		s.Fatal("Adapter state not as expected: ", err)
	}

	tconn, err := cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to create keyboard: ", err)
	}

	// Sign into user1.
	if err := signIn(ctx, tconn, kb, user1); err != nil {
		s.Fatal("Failed to sign in to user1: ", err)
	}

	// Confirm adapter is enabled.
	if err := bt.PollForAdapterState(ctx, true); err != nil {
		s.Fatal("Adapter state not as expected: ", err)
	}

	// Disable Bluetooth.
	if err := toggleBluetooth(ctx, tconn, bt, false); err != nil {
		s.Fatal("Failed to disable Bluetooth: ", err)
	}

	// Go to sign-in screen.
	cr, err = signOut(ctx, signInProfileTestExtension, enableFeatures, disableFeatures)
	if err != nil {
		s.Fatal("Failed to go to sign-in screen: ", err)
	}

	// Verify Bluetooth is enabled.
	if err := bt.PollForAdapterState(ctx, true); err != nil {
		s.Fatal("Adapter state not as expected: ", err)
	}

	tconn, err = cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Disable Bluetooth.
	if err := toggleBluetooth(ctx, tconn, bt, false); err != nil {
		s.Fatal("Failed to disable Bluetooth: ", err)
	}

	// Sign into user2.
	if err := signIn(ctx, tconn, kb, user2); err != nil {
		s.Fatal("Failed to sign in to user2: ", err)
	}

	// Confirm adapter is enabled.
	if err := bt.PollForAdapterState(ctx, true); err != nil {
		s.Fatal("Adapter state not as expected: ", err)
	}

	// Disable Bluetooth.
	if err := toggleBluetooth(ctx, tconn, bt, false); err != nil {
		s.Fatal("Failed to disable Bluetooth: ", err)
	}

	// Go to sign-in screen.
	cr, err = signOut(ctx, signInProfileTestExtension, enableFeatures, disableFeatures)
	if err != nil {
		s.Fatal("Failed to go to sign-in screen: ", err)
	}

	// Verify Bluetooth is disabled.
	if err := bt.PollForAdapterState(ctx, false); err != nil {
		s.Fatal("Adapter state not as expected: ", err)
	}

	tconn, err = cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Sign into user1.
	if err := signIn(ctx, tconn, kb, user1); err != nil {
		s.Fatal("Failed to sign in to user1: ", err)
	}

	// Confirm Bluetooth is disabled.
	if err := bt.PollForAdapterState(ctx, false); err != nil {
		s.Fatal("Adapter state not as expected: ", err)
	}

	// Enable Bluetooth.
	if err := toggleBluetooth(ctx, tconn, bt, true); err != nil {
		s.Fatal("Failed to enable Bluetooth: ", err)
	}

	// Go to sign-in screen.
	cr, err = signOut(ctx, signInProfileTestExtension, enableFeatures, disableFeatures)
	if err != nil {
		s.Fatal("Failed to go to sign-in screen: ", err)
	}

	// Verify Bluetooth is still disabled.
	if err := bt.PollForAdapterState(ctx, false); err != nil {
		s.Fatal("Adapter state not as expected: ", err)
	}

	tconn, err = cr.SigninProfileTestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Sign into user2.
	if err := signIn(ctx, tconn, kb, user2); err != nil {
		s.Fatal("Failed to sign in to user2: ", err)
	}

	// Confirm adapter is disabled.
	if err := bt.PollForAdapterState(ctx, false); err != nil {
		s.Fatal("Adapter state not as expected: ", err)
	}
}

// signOut signs out of the current session and goes to the sign-in screen.
func signOut(ctx context.Context, signinProfileTestExtension string, extraOpts ...chrome.Option) (*chrome.Chrome, error) {
	opts := append([]chrome.Option{chrome.NoLogin(),
		chrome.KeepState(),
		chrome.SkipForceOnlineSignInForTesting(),
		chrome.LoadSigninProfileExtension(signinProfileTestExtension)}, extraOpts...)
	cr, err := chrome.New(
		ctx,
		opts...,
	)
	if err != nil {
		return nil, err
	}
	return cr, nil
}

// signIn ensures the user is visible on the sign-in screen and signs into the user.
func signIn(ctx context.Context, tconn *chrome.TestConn, kb *input.KeyboardEventWriter, creds chrome.Creds) error {
	ui := uiauto.New(tconn)
	loginWindow := nodewith.Name("Login Screen").Role(role.Window)
	userButton := nodewith.Name(creds.User).Role(role.Button).Ancestor(loginWindow)
	if err := uiauto.NamedCombine(fmt.Sprintf("ensure user %q is visible", creds.User),
		ui.WaitUntilExists(userButton),
		ui.MakeVisible(userButton),
	)(ctx); err != nil {
		return err
	}

	passwordField, err := lockscreen.PasswordFieldFinder(creds.User)
	if err != nil {
		return errors.Wrap(err, "failed to determine the password field finder")
	}

	testing.ContextLog(ctx, "Start signing in to user ", creds.User)
	if err = ui.LeftClickUntil(userButton, ui.Exists(passwordField))(ctx); err != nil {
		return errors.Wrap(err, "failed to select the user")
	}
	if err = lockscreen.EnterPassword(ctx, tconn, creds.User, creds.Pass, kb); err != nil {
		return errors.Wrap(err, "failed to enter password")
	}
	if err = lockscreen.WaitForLoggedIn(ctx, tconn, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for logged in")
	}

	return nil
}

// toggleBluetooth toggles the Bluetooth adapter state and verifies the new state equals |exp|.
func toggleBluetooth(ctx context.Context, tconn *chrome.TestConn, bt bluetooth.Bluetooth, exp bool) error {
	if err := quicksettings.Show(ctx, tconn); err != nil {
		return errors.Wrap(err, "failed to show the Quick Settings")
	}
	defer quicksettings.Hide(ctx, tconn)

	ui := uiauto.New(tconn)
	if err := ui.LeftClick(quicksettings.FeatureTileBluetoothToggle)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the Bluetooth feature tile toggle")
	}
	if err := bt.PollForAdapterState(ctx, exp); err != nil {
		return errors.Wrap(err, "failed to toggle Bluetooth state")
	}
	return nil
}
