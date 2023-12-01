// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package apps

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/pre"
	"go.chromium.org/tast-tests/cros/local/chrome/apps/helpapp"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchHelpAppFromShortcut,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Help app can be launched using shortcut Search+H",
		Contacts: []string{
			"showoff-eng@google.com",
		},
		BugComponent: "b:690873",
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Params: []testing.Param{
			{
				Name:              "stable",
				Fixture:           fixture.LoggedIn,
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				ExtraAttr:         []string{"group:mainline"},
			}, {
				Name:    "unstable",
				Fixture: fixture.LoggedIn,
				// b:238260020 - disable aged (>1y) unpromoted informational tests
				// ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraHardwareDeps: hwdep.D(pre.AppsUnstableModels),
			},
			{
				Name:              "stable_guest",
				Fixture:           fixture.LoggedInGuest,
				ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
				ExtraAttr:         []string{"group:mainline"},
			}, {
				Name:    "unstable_guest",
				Fixture: fixture.LoggedInGuest,
				// b:238260020 - disable aged (>1y) unpromoted informational tests
				// ExtraAttr:         []string{"group:mainline", "informational"},
				ExtraHardwareDeps: hwdep.D(pre.AppsUnstableModels),
			},
		},
	})
}

// LaunchHelpAppFromShortcut verifies launching Help app from Search+H.
func LaunchHelpAppFromShortcut(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(fixture.FixtData).Chrome
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	kw, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard handle: ", err)
	}
	defer kw.Close(ctx)

	// On some low-end devices and guest mode sometimes Chrome is still
	// initializing when the shortcut keys are emitted. Check that the
	// app is showing up as installed before emitting the shortcut keys.
	if err := ash.WaitForChromeAppInstalled(ctx, tconn, apps.Help.ID, 30*time.Second); err != nil {
		s.Fatal("Failed to wait for Explore to be installed: ", err)
	}

	helpCtx := helpapp.NewContext(cr, tconn)

	shortcut := "Search+H"
	testName := "launch_help_app_shortcut"
	s.Run(ctx, testName, func(ctx context.Context, s *testing.State) {
		defer func() {
			outDir := filepath.Join(s.OutDir(), testName)
			faillog.DumpUITreeWithScreenshotOnError(ctx, outDir, s.HasError, cr, "ui_tree_"+testName)

			if err := helpCtx.Close()(ctx); err != nil {
				s.Log("Failed to close the app, may not have been opened: ", err)
			}
		}()

		ui := uiauto.New(tconn).WithTimeout(time.Minute)
		if err := ui.Retry(5, func(ctx context.Context) error {
			if err := kw.Accel(ctx, shortcut); err != nil {
				return errors.Wrapf(err, "failed to press %q keys", shortcut)
			}
			return helpapp.NewContext(cr, tconn).WaitForApp()(ctx)
		})(ctx); err != nil {
			s.Fatalf("Failed to launch or render Help app by shortcut %q: %v", shortcut, err)
		}
	})
}
