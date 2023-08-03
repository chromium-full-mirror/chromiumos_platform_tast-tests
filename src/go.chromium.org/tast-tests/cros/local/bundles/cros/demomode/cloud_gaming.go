// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/demomode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

// Models that are cloud gaming devices. Ideally instead of specifying models directly,
// we should specify devices with the cloud_gaming_device brand_config, but currently
// this is unsupported by software deps.
var cloudGamingModels = []string{
	"taniks",
	"osiris",
	"delbing",
	"mithrax",
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         CloudGaming,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the Demo Mode System Web App launches in fullscreen and goes to windowed mode after user interaction",
		Contacts:     []string{"cros-demo-mode-eng@google.com", "jacksontadie@google.com"},
		// Chrome OS Server Projects > Enterprise Management > Demo Mode
		BugComponent: "b:812312",
		Fixture:      fixture.PostDemoModeOOBECloudGaming,
		Attr:         []string{"group:mainline", "informational"},
		// Demo Mode uses Zero Touch Enrollment for enterprise enrollment, which
		// requires a real TPM.
		// We require "arc" and "chrome_internal" because the ARC TOS screen
		// is only shown for chrome-branded builds when the device is ARC-capable.
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc", "tpm2"},
		HardwareDeps: hwdep.D(hwdep.Model(cloudGamingModels...)),
	})
}

func CloudGaming(ctx context.Context, s *testing.State) {
	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ARCSupported(),
		chrome.KeepEnrollment(),
		chrome.EnableFeatures("CloudGamingDevice"),
		// --force-devtools-available forces devtools on regardless of policy (devtools is
		// disabled in Demo Mode policy) to support connecting to the test API extension.
		//
		// --component-updater=test-request adds a "test-request" parameter to Omaha
		// update requests, causing the fetched Demo Mode App component to come from a
		// test cohort.
		chrome.ExtraArgs("--force-devtools-available", "--component-updater=test-request"))
	if err != nil {
		s.Fatal("Failed to restart Chrome: ", err)
	}

	clearUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()
	defer cr.Close(clearUpCtx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create the test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(clearUpCtx, s.OutDir(), s.HasError, tconn)

	gamingDisplaySectionButton := nodewith.Role(role.Button).Name("Discover Chromebook Display")

	if err := demomode.VerifySWAFunctionality(ctx, tconn, gamingDisplaySectionButton); err != nil {
		s.Fatal("Failed to verify SWA functionality: ", err)
	}
}
