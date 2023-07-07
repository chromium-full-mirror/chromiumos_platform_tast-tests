// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/arc/playstore"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/demomode/fixture"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DemoModeOfflineProvision,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Enter Demo Mode from OOBE via offline mode, verify Play Store can be opened",
		Contacts:     []string{"arc-commercial@google.com", "yaohuali@google.com"},
		// ChromeOS > Software > ARC++ > Commercial
		BugComponent: "b:157100",
		Fixture:      fixture.PostDemoModeOOBE,
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		// Demo Mode uses Zero Touch Enrollment for enterprise enrollment, which
		// requires a real TPM.
		// We require "arc" and "chrome_internal" because the ARC TOS screen
		// is only shown for chrome-branded builds when the device is ARC-capable.
		SoftwareDeps: []string{"chrome", "arc", "tpm", "play_store"},
		Timeout:      15 * time.Minute,
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

func DemoModeOfflineProvision(ctx context.Context, s *testing.State) {
	const (
		bootTimeout            = 6 * time.Minute
		provisioningTimeout    = 5 * time.Minute
		playStoreWindowTimeout = 10 * time.Second
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ARCSupported(),
		chrome.UnRestrictARCCPU(),
		chrome.KeepEnrollment(),
		chrome.ExtraArgs(arc.DisableSyncFlags()...),
		// Force ARC++ provisioning to be done in offline mode.
		chrome.ExtraArgs("--demo-mode-force-arc-offline-provision"),
		// Force devtools on regardless of policy (devtools is disabled in
		// Demo Mode policy) to support connecting to the test API extension.
		chrome.ExtraArgs("--force-devtools-available"))
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(cleanupCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// Wait for ARC to start and ADB to be setup, which would take a bit long.
	a, err := arc.NewWithTimeout(ctx, s.OutDir(), bootTimeout)
	if err != nil {
		s.Fatal("Failed to get ARC: ", err)
	}
	defer a.Close(cleanupCtx)

	if err := a.WaitForProvisioning(ctx, provisioningTimeout); err != nil {
		s.Fatal("Failed to wait for ARC provisioning: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.PlayStore.ID); err != nil {
		s.Fatal("Failed to launch Play Store: ", err)
	}

	if err := playstore.VerifyPlayStoreWindowPresent(ctx, tconn, playStoreWindowTimeout); err != nil {
		s.Fatal("Failed to ensure Play Store window is present: ", err)
	}
}
