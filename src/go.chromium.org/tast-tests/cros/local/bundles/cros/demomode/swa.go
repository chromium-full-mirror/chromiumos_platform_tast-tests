// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package demomode

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/demomode"
	"go.chromium.org/tast-tests/cros/local/network"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// demoModeSWATestCase struct encapsulates parameters for each SWA test.
type demoModeSWATestCase struct {
	dmServerURL     string
	shouldRunOnline bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         SWA,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the Demo Mode System Web App launches in fullscreen and goes to windowed mode after user interaction",
		Contacts:     []string{"cros-demo-mode-eng@google.com", "jacksontadie@google.com"},
		BugComponent: "b:812312",
		Attr:         []string{"group:mainline", "informational"},
		// Demo Mode uses Zero Touch Enrollment for enterprise enrollment, which
		// requires a real TPM.
		// We require "arc" and "chrome_internal" because the ARC TOS screen
		// is only shown for chrome-branded builds when the device is ARC-capable.
		// Demo Mode doesn't support VMs, use "crossystem" to exclude VMs.
		SoftwareDeps: []string{"chrome", "chrome_internal", "arc", "tpm2", "crossystem"},
		Params: []testing.Param{{
			Name: "online_alpha",
			Val: demoModeSWATestCase{
				dmServerURL:     policy.DMServerAlphaURL,
				shouldRunOnline: true,
			},
			Fixture: fixture.PostDemoModeOOBEAlpha,
			// TODO (b/346725308): Refactor to use utility and known dependency list.
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "external_dependency", Value: "DMServerAlpha",
			}},
		}, {
			Name: "online_prod",
			Val: demoModeSWATestCase{
				dmServerURL:     policy.DMServerProdURL,
				shouldRunOnline: true,
			},
			Fixture: fixture.PostDemoModeOOBEProd,
			// TODO (b/346725308): Refactor to use utility and known dependency list.
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "external_dependency", Value: "DMServerProd",
			}},
		}, {
			// DMServer URL is irrelevant for offline test case, so we don't have two separate cases
			Name: "offline",
			Val: demoModeSWATestCase{
				dmServerURL:     policy.DMServerAlphaURL,
				shouldRunOnline: false,
			},
			Fixture: fixture.PostDemoModeOOBEAlpha,
			// TODO (b/346725308): Refactor to use utility and known dependency list.
			ExtraSearchFlags: []*testing.StringPair{{
				Key: "external_dependency", Value: "DMServerAlpha",
			}},
		}},
	})
}

func SWA(ctx context.Context, s *testing.State) {
	tc := s.Param().(demoModeSWATestCase)

	// Behavior that will be tested both online and offline.
	restartChromeAndVerifySWA := func(ctx context.Context) error {
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
			chrome.ExtraArgs("--force-devtools-available", "--component-updater=test-request"),
			chrome.DMSPolicy(tc.dmServerURL))
		if err != nil {
			return errors.Wrap(err, "failed to restart Chrome")
		}

		clearUpCtx := ctx
		ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
		defer cancel()
		defer cr.Close(clearUpCtx)

		tconn, err := cr.TestAPIConn(ctx)
		if err != nil {
			return errors.Wrap(err, "failed to create the test API connection")
		}
		defer faillog.DumpUITreeOnError(clearUpCtx, s.OutDir(), s.HasError, tconn)

		// Element in 2024 C1 refresh.
		highlightsMainNav := nodewith.Role(role.Button).Name("Chromebook logo")
		attractLoopNode := nodewith.Role(role.Video).ClassName("attract-loop")

		return demomode.VerifySWAFunctionality(ctx, tconn, highlightsMainNav, attractLoopNode)
	}

	if tc.shouldRunOnline {
		if err := restartChromeAndVerifySWA(ctx); err != nil {
			s.Fatal("Failed to verify SWA functionality online: ", err)
		}
	} else {
		if err := network.ExecFuncOnChromeOffline(ctx, restartChromeAndVerifySWA); err != nil {
			s.Fatal("Failed to verify SWA functionality offline: ", err)
		}
	}
}
