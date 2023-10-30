// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"path/filepath"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/optin"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/launcher"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

type playStoreSearchAndLaunchTestParams struct {
	MaxOptinAttempts int
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PlayStoreSearchAndLaunch,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "A functional test of the Play Store that installs, search for, and launches Google Calculator",
		Contacts: []string{
			"arc-core@google.com",
			"cros-system-ui-eng@google.com",
			"yulunwu@google.com",
			"tbarzic@chromium.org",
		},
		// ChromeOS > Software > ARC++ > Core > Play Store Setup
		BugComponent: "b:1131344",
		Attr:         []string{"group:mainline", "informational", "group:arc-functional"},
		Params: []testing.Param{{
			Val: playStoreSearchAndLaunchTestParams{
				MaxOptinAttempts: 2,
			},
			ExtraSoftwareDeps: []string{"android_container", "chrome"},
		}, {
			Name: "vm",
			Val: playStoreSearchAndLaunchTestParams{
				MaxOptinAttempts: 2,
			},
			ExtraAttr:         []string{"group:hw_agnostic"},
			ExtraSoftwareDeps: []string{"android_vm", "chrome"},
		}},
		Timeout: 10 * time.Minute,
		VarDeps: []string{"ui.gaiaPoolDefault"},
	})
}

func PlayStoreSearchAndLaunch(ctx context.Context, s *testing.State) {
	const (
		pkgName = "com.google.android.calculator"
		appName = "Calculator"
	)
	// Give cleanup actions a minute to run.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, time.Minute)
	defer cancel()

	// Setup Chrome.
	cr, err := chrome.New(ctx,
		chrome.GAIALoginPool(s.RequiredVar("ui.gaiaPoolDefault")),
		chrome.ARCSupported(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	// Optin to Play Store.
	s.Log("Opting into Play Store")
	maxAttempts := s.Param().(playStoreSearchAndLaunchTestParams).MaxOptinAttempts

	if err := optin.PerformWithRetry(ctx, cr, maxAttempts); err != nil {
		s.Fatal("Failed to optin to Play Store: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	// Setup ARC.
	a, err := arc.New(ctx, s.OutDir(), cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to start ARC: ", err)
	}
	defer a.Close(cleanupCtx)
	defer func() {
		if s.HasError() {
			if err := a.Command(cleanupCtx, "uiautomator", "dump").Run(testexec.DumpLogOnError); err != nil {
				s.Error("Failed to dump UIAutomator: ", err)
			}
			if err := a.PullFile(cleanupCtx, "/sdcard/window_dump.xml", filepath.Join(s.OutDir(), "uiautomator_dump.xml")); err != nil {
				s.Error("Failed to pull UIAutomator dump: ", err)
			}
		}
	}()

	d, err := a.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(cleanupCtx)

	// Install app.
	s.Log("Installing app")
	if err := playstore.InstallApp(ctx, a, d, pkgName, &playstore.Options{TryLimit: -1}); err != nil {
		s.Fatal("Failed to install app: ", err)
	}

	// Setup keyboard.
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to find keyboard: ", err)
	}
	defer kb.Close(ctx)

	ui := uiauto.New(tconn)

	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	//Search for and launch the app.
	if err := uiauto.Combine("search for calculator in launcher",
		launcher.SearchAndLaunchWithQuery(tconn, kb, appName, appName),
		ui.WaitUntilExists(nodewith.Name(appName).ClassName("Widget")),
	)(ctx); err != nil {
		s.Fatal("Failed to search and launch: ", err)
	}
}
