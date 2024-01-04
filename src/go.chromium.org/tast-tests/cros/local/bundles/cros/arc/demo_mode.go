// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/android/ui"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DemoMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Enter Demo Mode from OOBE, open Play Store and verify the Install button is disabled",
		Contacts:     []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial > Tast Tests
		BugComponent: "b:1487630",
		Fixture:      fixture.PostDemoModeOOBEProd,
		Attr:         []string{"group:mainline"},
		// Demo Mode uses Zero Touch Enrollment for enterprise enrollment, which
		// requires a real TPM.
		// We require "arc" and "chrome_internal" because the ARC TOS screen
		// is only shown for chrome-branded builds when the device is ARC-capable.
		// Demo Mode doesn't support VMs, use "crossystem" to exclude VMs.
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc", "tpm", "play_store", "crossystem"},
		Timeout:      10 * time.Minute,
		Params: []testing.Param{
			{
				ExtraSoftwareDeps: []string{"android_container"},
				ExtraAttr:         []string{"informational"},
			},
			{
				Name:              "vm",
				ExtraSoftwareDeps: []string{"android_vm"},
				ExtraAttr:         []string{"informational"},
			}},
	})
}

func DemoMode(ctx context.Context, s *testing.State) {
	const (
		installButtonText  = "Install"
		inDemoModeText     = "In demo mode. Content and features may be limited."
		policyNotReadyText = "Your administrator has not given you access to this item."
		testPackage        = "com.google.android.calculator"
	)

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.KeepEnrollment(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
		// Force devtools on regardless of policy (devtools is disabled in
		// Demo Mode policy) to support connecting to the test API extension.
		chrome.ExtraArgs("--force-devtools-available"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	clearupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer cr.Close(clearupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(clearupCtx, s.OutDir(), s.HasError, tconn)

	uia := uiauto.New(tconn)

	// Wait for ARC to start and ADB to be setup, which would take a bit long.
	arc, err := arc.NewWithTimeout(ctx, s.OutDir(), 4*time.Minute, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to get ARC: ", err)
	}
	defer arc.Close(clearupCtx)

	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}

	// Verify that Play Store window shows up.
	classNameRegexp := regexp.MustCompile(`^ExoShellSurface(-\d+)?$`)
	playStoreUI := nodewith.Name("Play Store").Role(role.Window).ClassNameRegex(classNameRegexp)
	if err := uia.WithTimeout(5 * time.Minute).WaitUntilExists(playStoreUI)(ctx); err != nil {
		s.Fatal("Failed to see Play Store window: ", err)
	}

	d, err := arc.NewUIDevice(ctx)
	if err != nil {
		s.Fatal("Failed initializing UI Automator: ", err)
	}
	defer d.Close(clearupCtx)

	// Wait until Play Store received proper policy and shows "In demo mode...". Otherwise, the
	// (calculator) app page would not show a grayed out Install button, but rather a text "Your
	// administrator has not given you access to this item.".
	demoModeTextView := d.Object(ui.ClassName("android.widget.TextView"), ui.TextMatches(inDemoModeText))
	if err := demoModeTextView.WaitForExists(ctx, 90*time.Second); err != nil {
		s.Fatal("Failed to find \"In demo mode...\" in Play Store: ", err)
	}

	playstore.OpenAppPage(ctx, arc, testPackage)

	// Ensure that the "Install" button appears and is disabled.
	opButton := d.Object(ui.ClassName("android.widget.Button"), ui.TextMatches(installButtonText), ui.Enabled(false))
	if err := opButton.WaitForExists(ctx, 20*time.Second); err != nil {
		s.Fatal("Failed to find greyed Install button in Play Store: ", err)
	}
}
