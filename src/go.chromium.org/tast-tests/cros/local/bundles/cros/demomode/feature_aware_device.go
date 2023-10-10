// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"os"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FeatureAwareDevice,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that feature aware devices download the right app component for demomode",
		Contacts:     []string{"cros-demo-mode-eng@google.com", "xiqiruan@google.com"},
		// Chrome OS Server Projects > Enterprise Management > Demo Mode
		BugComponent: "b:812312",
		Fixture:      fixture.PostDemoModeOOBEProd,
		Attr:         []string{"group:cbx", "cbx_feature_enabled", "cbx_unstable"},
		// Demo Mode uses Zero Touch Enrollment for enterprise enrollment, which
		// requires a real TPM.
		// We require "arc" and "chrome_internal" because the ARC TOS screen
		// is only shown for chrome-branded builds when the device is ARC-capable.
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc", "tpm2", "tpm_clear_allowed"},
	})
}

// FeatureAwareDevice test to verify that feature aware devices download the
// right app component for demo mode.
func FeatureAwareDevice(ctx context.Context, s *testing.State) {
	clearUpCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr, err := chrome.New(ctx,
		chrome.NoLogin(),
		chrome.ARCSupported(),
		chrome.KeepEnrollment(),
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

	defer cr.Close(clearUpCtx)

	tconn, err := cr.TestAPIConn(ctx)
	defer faillog.DumpUITreeOnError(clearUpCtx, s.OutDir(), s.HasError, tconn)

	const appPath = "/run/imageloader/demo-mode-app"
	entries, err := os.ReadDir(appPath)
	if err != nil {
		s.Fatal("Failed to find the mounted app component directory: ", err)
	}

	versionRegexp, _ := regexp.Compile("1\\.[0-9]+\\.2\\.[0-9]+")
	hasMatch := false
	for _, e := range entries {
		if versionRegexp.MatchString(e.Name()) {
			hasMatch = true
			break
		}
	}

	if !hasMatch {
		s.Fatal("Cannot find versioned app component")
	}
}
