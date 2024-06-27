// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package bluetooth

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/bluetooth"
	"go.chromium.org/tast-tests/cros/local/bluetooth/bluez"
	"go.chromium.org/tast-tests/cros/local/bluetooth/floss"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/userutil"
	"go.chromium.org/tast/core/ctxutil"
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
		LifeCycleStage: testing.LifeCycleInDevelopment,
		Desc:           "Checks that the Bluetooth adapter state preference is preserved for the device and users",
		Contacts: []string{
			"alfredyu@cienet.com",
			"chromeos-connectivity-cienet-external@google.com",
		},
		BugComponent: "b:1578688", // ChromeOS > External > Cienet > Manual Test Automation > Test stabilization
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
				ExtraAttr: []string{"bluetooth_sa"},
			}, {
				Name: "floss_enabled",
				Val: enableDisableBluetoothWithDifferentUsersParams{
					btImpl:          &floss.Floss{},
					enableFeatures:  []string{"Floss"},
					disableFeatures: []string{"FlossIsAvailabilityCheckNeeded"},
				},
				ExtraAttr:         []string{"bluetooth_floss_flaky"},
				ExtraSoftwareDeps: []string{"bluetooth_floss"},
			},
		},
		Timeout: 5 * time.Minute,
	})
}

// EnableDisableBluetoothWithDifferentUsers tests that the device's and users' last Bluetooth adapter states are preserved and restored between user sessions.
func EnableDisableBluetoothWithDifferentUsers(ctx context.Context, s *testing.State) {
	params := s.Param().(enableDisableBluetoothWithDifferentUsersParams)

	enableFeatures := chrome.EnableFeatures(params.enableFeatures...)
	disableFeatures := chrome.DisableFeatures(params.disableFeatures...)

	// Create a device owner.
	userA := chrome.Creds{User: "test_owner@gmail.com", Pass: "test0000"}
	if err := userutil.CreateDeviceOwner(ctx, userA.User, userA.Pass, enableFeatures, disableFeatures); err != nil {
		s.Fatal("Failed to create device owner, userA: ", err)
	}

	// Create a second user.
	userB := chrome.Creds{User: "test_user2@gmail.com", Pass: "test0000"}
	if err := userutil.CreateUser(ctx, userB.User, userB.Pass, chrome.KeepState(), enableFeatures, disableFeatures); err != nil {
		s.Fatal("Failed to create userB: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	bt := params.btImpl
	// Ensure Bluetooth starts enabled.
	if err := bt.Enable(ctx); err != nil {
		s.Fatal("Failed to enable Bluetooth: ", err)
	}
	// Ensure the adapter state is cleaned up at the end of the test.
	defer bt.Enable(cleanupCtx)

	signInProfileTestExtension := s.RequiredVar("ui.signinProfileTestExtensionManifestKey")

	res := enableDisableBluetoothWithDifferentUsersHelper{
		bt:     bt,
		outdir: s.OutDir(),
	}

	for _, stage := range []struct {
		name     string
		loggedIn bool
		user     chrome.Creds
		actions  []uiauto.Action
	}{
		{
			name:     "Verify Bluetooth is enabled on sign in screen",
			loggedIn: false,
			actions: []uiauto.Action{
				res.verifyBluetoothState(true),
			},
		}, {
			name:     "Sign in as UserA. Verify Bluetooth is enabled, then manually disable Bluetooth",
			loggedIn: true,
			user:     userA,
			actions: []uiauto.Action{
				res.verifyBluetoothState(true),
				res.toggleBluetooth(false),
			},
		}, {
			name:     "Verify Bluetooth is enabled on sign in screen, then manually disable Bluetooth",
			loggedIn: false,
			actions: []uiauto.Action{
				res.verifyBluetoothState(true),
				res.toggleBluetooth(false),
			},
		}, {
			name:     "Sign in as UserB. Verify Bluetooth is enabled, then manually disable Bluetooth",
			loggedIn: true,
			user:     userB,
			actions: []uiauto.Action{
				res.verifyBluetoothState(true),
				res.toggleBluetooth(false),
			},
		}, {
			name:     "Verify Bluetooth is disabled on sign in screen",
			loggedIn: false,
			actions: []uiauto.Action{
				res.verifyBluetoothState(false),
			},
		}, {
			name:     "Verify Bluetooth is disabled from UserA, then manually enable Bluetooth",
			loggedIn: true,
			user:     userA,
			actions: []uiauto.Action{
				res.verifyBluetoothState(false),
				res.toggleBluetooth(true),
			},
		}, {
			name:     "Verify Bluetooth is disabled on sign in screen",
			loggedIn: false,
			actions: []uiauto.Action{
				res.verifyBluetoothState(false),
			},
		}, {
			name:     "Verify Bluetooth is disabled from UserB",
			loggedIn: true,
			user:     userB,
			actions: []uiauto.Action{
				res.verifyBluetoothState(false),
			},
		},
	} {
		opts := []chrome.Option{
			chrome.KeepState(),
			enableFeatures,
			disableFeatures,
		}
		if !stage.loggedIn {
			opts = append(opts, chrome.NoLogin())
			opts = append(opts, chrome.LoadSigninProfileExtension(signInProfileTestExtension))
		} else {
			opts = append(opts, chrome.FakeLogin(stage.user))
		}

		res.loggedIn = stage.loggedIn
		if err := res.startNewChromeSessionAndVerify(ctx, opts, stage.actions); err != nil {
			s.Fatalf("Failed to verify on %s stage: %v", stage.name, err)
		}
	}
}

type enableDisableBluetoothWithDifferentUsersHelper struct {
	cr       *chrome.Chrome
	tconn    *chrome.TestConn
	loggedIn bool
	bt       bluetooth.Bluetooth
	outdir   string
}

func (h *enableDisableBluetoothWithDifferentUsersHelper) startNewChromeSessionAndVerify(ctx context.Context, opts []chrome.Option, actions []uiauto.Action) error {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	var err error
	h.cr, err = chrome.New(ctx, opts...)
	if err != nil {
		return err
	}
	defer func(ctx context.Context) {
		h.cr.Close(ctx)
		h.cr = nil
	}(cleanupCtx)

	fetchTconn := h.cr.TestAPIConn
	if !h.loggedIn {
		fetchTconn = h.cr.SigninProfileTestAPIConn
	}

	h.tconn, err = fetchTconn(ctx)
	if err != nil {
		return err
	}
	defer func() { h.tconn = nil }()

	for _, action := range actions {
		if err := action(ctx); err != nil {
			return err
		}
	}
	return nil
}

func (h *enableDisableBluetoothWithDifferentUsersHelper) toggleBluetooth(expected bool) uiauto.Action {
	return func(ctx context.Context) (retErr error) {
		cleanupCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
		defer cancel()

		if err := quicksettings.ShowWithRetry(ctx, h.tconn, 30*time.Second); err != nil {
			return errors.Wrap(err, "failed to show the Quick Settings")
		}
		defer quicksettings.Hide(cleanupCtx, h.tconn)
		defer faillog.DumpUITreeWithScreenshotWithTestAPIOnError(cleanupCtx, h.outdir, func() bool { return retErr != nil }, h.tconn, "quick_setting_ui_dump")

		ui := uiauto.New(h.tconn)
		if err := ui.LeftClick(quicksettings.FeatureTileBluetoothToggle)(ctx); err != nil {
			return errors.Wrap(err, "failed to click the Bluetooth feature tile toggle")
		}

		return h.verifyBluetoothState(expected)(ctx)
	}
}

func (h *enableDisableBluetoothWithDifferentUsersHelper) verifyBluetoothState(expected bool) uiauto.Action {
	return func(ctx context.Context) error {
		msg := map[bool]string{false: "off", true: "on"}

		if err := h.bt.PollForAdapterState(ctx, expected); err != nil {
			return errors.Wrapf(err, "adapter state is not %q", msg[expected])
		}

		enabled, err := quicksettings.BluetoothEnabled(ctx, h.tconn)
		if err != nil {
			return errors.Wrap(err, "failed to fetch the Bluetooth state from quick settings")
		}
		if enabled != expected {
			return errors.Errorf("bluetooth state from quick settings is not %q", msg[expected])
		}

		return nil
	}
}
