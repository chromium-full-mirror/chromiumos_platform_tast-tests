// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/capturemode"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/cryptohome"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/screenshot"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisableScreenshotsCaptureMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Behavior of the DisableScreenshots policy, check whether screenshot can be taken from capture mode in quick settings",
		Contacts: []string{
			"chromeos-wms@google.com",
			"afakhry@google.com",
		},
		BugComponent: "b:1253115",
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:hw_agnostic"},
		Fixture:      fixture.ChromePolicyLoggedIn,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DisableScreenshots{}, pci.VerifiedFunctionalityUI),
		},
	})
}

func DisableScreenshotsCaptureMode(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to get test API connection: ", err)
	}

	downloadsPath, err := cryptohome.DownloadsPath(ctx, cr.NormalizedUser())
	if err != nil {
		s.Fatal("Failed to retrieve user's Downloads path: ", err)
	}
	defer func() {
		if err := screenshot.RemoveScreenshots(downloadsPath); err != nil {
			s.Error("Failed to remove screenshots after all tests: ", err)
		}
	}()

	for _, tc := range []struct {
		name             string
		value            []policy.Policy
		wantAllowed      bool
		wantNotification string
	}{
		{
			name:             "true",
			value:            []policy.Policy{&policy.DisableScreenshots{Val: true}},
			wantAllowed:      false,
			wantNotification: "Can't capture content",
		},
		{
			name:             "false",
			value:            []policy.Policy{&policy.DisableScreenshots{Val: false}},
			wantAllowed:      true,
			wantNotification: "Screenshot taken",
		},
		{
			name:             "unset",
			value:            []policy.Policy{},
			wantAllowed:      true,
			wantNotification: "Screenshot taken",
		},
	} {
		s.Run(ctx, tc.name, func(ctx context.Context, s *testing.State) {
			defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "ui_tree_"+tc.name)

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			if err := ash.CloseNotifications(ctx, tconn); err != nil {
				s.Fatal("Failed to close notifications: ", err)
			}

			if err := screenshot.RemoveScreenshots(downloadsPath); err != nil {
				s.Fatal("Failed to remove screenshots: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, tc.value); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			if err := capturemode.TakeAreaScreenshot(ctx, tconn); err != nil {
				// Don't fail if screenshots are not allowed by policy and capture mode was not found.
				if errors.Is(err, capturemode.ErrCaptureModeNotFound) && tc.wantAllowed {
					s.Fatal("Capture mode is not shown, but allowed by policy")
				} else if !errors.Is(err, capturemode.ErrCaptureModeNotFound) {
					s.Fatal("Failed to take screenshot: ", err)
				}
			}

			if _, err := ash.WaitForNotification(ctx, tconn, 15*time.Second, ash.WaitIDContains("capture_mode_notification"), ash.WaitTitle(tc.wantNotification)); err != nil {
				s.Fatalf("Failed to wait notification with title %q: %v", tc.wantNotification, err)
			}

			has, err := screenshot.HasScreenshots(downloadsPath)
			if err != nil {
				s.Fatal("Failed to check whether screenshot is present: ", err)
			}
			if has != tc.wantAllowed {
				s.Errorf("Unexpected screenshot allowed: get %t; want %t", has, tc.wantAllowed)
			}
		})
	}
}
