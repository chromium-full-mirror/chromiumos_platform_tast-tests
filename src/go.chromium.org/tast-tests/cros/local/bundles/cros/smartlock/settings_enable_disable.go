// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package smartlock contains tests for the Smart Lock feature in ChromeOS.
package smartlock

import (
	"context"
	"time"

	cdcommon "go.chromium.org/tast-tests/cros/common/cros/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice"
	"go.chromium.org/tast-tests/cros/local/chrome/crossdevice/smartlock"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SettingsEnableDisable,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests ability to enable/disable Smart Lock with Settings",
		Contacts: []string{
			"chromeos-cross-device-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"hansberry@google.com",
		},
		BugComponent: "b:1131772",
		Attr:         []string{"group:cross-device", "cross-device_smartlock"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      8 * time.Minute,
		Params: []testing.Param{
			{
				Fixture:           "crossdeviceOnboardedNoLock",
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel(cdcommon.UnstableModels...)),
			},
			{
				Name:              "unstable",
				Fixture:           "crossdeviceOnboardedNoLock",
				ExtraHardwareDeps: hwdep.D(hwdep.Model(cdcommon.UnstableModels...)),
			},
			{
				Name:      "floss",
				Fixture:   "crossdeviceOnboardedNoLockFloss",
				ExtraAttr: []string{"cross-device_floss"},
			},
		},
	})
}

// SettingsEnableDisable tests enabling/disabling Smart Lock in settings.
func SettingsEnableDisable(ctx context.Context, s *testing.State) {
	var err error

	cr := s.FixtValue().(*crossdevice.FixtData).Chrome
	tconn := s.FixtValue().(*crossdevice.FixtData).TestConn
	username := s.FixtValue().(*crossdevice.FixtData).Username
	password := s.FixtValue().(*crossdevice.FixtData).Password

	// This test does a few login/logout cycles. We want all the chrome sessions to have the same setup as the fixture.
	opts := s.FixtValue().(*crossdevice.FixtData).ChromeOptions

	// Ensure all logins do not clear existing users.
	opts = append(opts, chrome.KeepState())

	// Explicitly copy opts. Otherwise loginOpts could be overridden by append() when constructing noLoginOpts.
	loginOpts := append(
		append([]chrome.Option(nil), opts...),
		chrome.GAIALogin(chrome.Creds{User: username, Pass: password}),
	)
	noLoginOpts := append(
		append([]chrome.Option(nil), opts...),
		chrome.NoLogin(),
		chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")),
	)

	defer func() {
		faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)
		cr.Close(ctx)
	}()

	// Check that Smart Lock is not shown when disabled.
	s.Log("Disabling Smart Lock")
	if err := smartlock.ToggleSmartLockEnabled(ctx, false /*enable*/, tconn, cr, password); err != nil {
		s.Fatal("Failed to toggle off Smart Lock: ", err)
	}
	if err = smartlock.CheckSmartLockVisibilityOnLockScreen(ctx, false /*expectVisible*/, tconn, username, password); err != nil {
		s.Fatal("CheckSmartLockVisibilityOnLockScreen(expectVisible=false) failed: ", err)
	}
	if cr, tconn, err = smartlock.CheckSmartLockVisibilityOnSigninScreen(ctx, false /*expectVisible*/, cr, tconn, loginOpts, noLoginOpts); err != nil {
		s.Fatal("CheckSmartLockVisibilityOnSigninScreen(expectVisible=false) failed: ", err)
	}

	// Check that Smart Lock is shown when enabled.
	s.Log("Enabling Smart Lock")
	if err := smartlock.ToggleSmartLockEnabled(ctx, true /*enable*/, tconn, cr, password); err != nil {
		s.Fatal("Failed to toggle on Smart Lock: ", err)
	}
	if err = smartlock.CheckSmartLockVisibilityOnLockScreen(ctx, true /*expectVisible*/, tconn, username, password); err != nil {
		s.Fatal("CheckSmartLockVisibilityOnLockScreen(expectVisible=true) failed: ", err)
	}
	if cr, tconn, err = smartlock.CheckSmartLockVisibilityOnSigninScreen(ctx, false /*expectVisible*/, cr, tconn, loginOpts, noLoginOpts); err != nil {
		s.Fatal("CheckSmartLockVisibilityOnSigninScreen(expectVisible=false) failed: ", err)
	}
}
