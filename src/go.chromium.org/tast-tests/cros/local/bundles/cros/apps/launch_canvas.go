// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apps contains functionality and test cases for ChromeOS essential Apps.
package apps

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/fixture"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/apps/pre"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LaunchCanvas,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Launches Chrome Canvas APP through the launcher after user login",
		Attr:         []string{"group:mainline" /* TODO(b/266507106): fix the test for L1 betty. "group:hw_agnostic" */},
		Contacts: []string{
			"blick-swe@google.com",
			"xiuwen@google.com",
		},
		BugComponent: "b:385726",
		Timeout:      5 * time.Minute,
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Params: []testing.Param{{
			Name:              "stable",
			Fixture:           fixture.LoggedIn,
			ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
		}, {
			Name:              "unstable",
			Fixture:           fixture.LoggedIn,
			ExtraHardwareDeps: hwdep.D(pre.AppsUnstableModels),
			ExtraAttr:         []string{"group:hw_agnostic"},
			// b:238260020 - disable aged (>1y) unpromoted informational tests
			// ExtraAttr:         []string{"group:mainline", "informational"},
		}, {
			Name:              "lacros",
			Fixture:           fixture.LacrosLoggedIn,
			ExtraSoftwareDeps: []string{"lacros"},
			ExtraAttr:         []string{"informational"},
			ExtraHardwareDeps: hwdep.D(pre.AppsStableModels),
		}},
	})
}

// LaunchCanvas verifies launching Canvas after OOBE
func LaunchCanvas(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(fixture.FixtData).TestAPIConn

	cleanupCtx := ctx
	// Use a shortened context for test operations to reserve time for cleanup.
	ctx, shortCancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer shortCancel()

	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	if err := ash.WaitForChromeAppInstalled(ctx, tconn, apps.Canvas.ID, 2*time.Minute); err != nil {
		s.Fatal("Failed to wait for installed app: ", err)
	}

	if err := apps.Launch(ctx, tconn, apps.Canvas.ID); err != nil {
		s.Fatal("Failed to launch Canvas: ", err)
	}
	defer apps.Close(cleanupCtx, tconn, apps.Canvas.ID)

	if err := ash.WaitForApp(ctx, tconn, apps.Canvas.ID, time.Minute); err != nil {
		s.Fatalf("Fail to wait for %s by app id %s: %v", apps.Canvas.Name, apps.Canvas.ID, err)
	}

	// Use welcome page to verify page rendering
	ui := uiauto.New(tconn).WithTimeout(60 * time.Second)
	canvasFinder := nodewith.Role(role.Heading).Name("Welcome to Canvas!")

	if err := ui.WaitUntilExists(canvasFinder)(ctx); err != nil {
		s.Fatal("Failed to render Canvas: ", err)
	}
}
