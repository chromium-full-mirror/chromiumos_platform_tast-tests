// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros/lacrosproc"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/kioskmode"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         FloatingAccessibilityMenuEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Start Kiosk application with FloatingAccessibilityMenuEnabled applied to the account",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"kamilszarek@google.com", // Test author - Ash.
			"anqing@google.com",      // Test author - Lacros.
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		VarDeps:      []string{"ui.signinProfileTestExtensionManifestKey"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      kioskmode.SetupDuration + kioskmode.LaunchDuration + kioskmode.CleanupDuration + 30*time.Second,
		Params: []testing.Param{
			{
				Name: "ash",
				Val:  kioskmode.TestData{IsLacros: false},
			},
			{
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Val: kioskmode.TestData{
					IsLacros: true,
					Policies: []policy.Policy{
						&policy.LacrosAvailability{Val: "lacros_only"},
					},
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LacrosAvailability{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.FloatingAccessibilityMenuEnabled{}, pci.VerifiedFunctionalityUI),
			{
				Key: "feature_id",
				// Enable accessibility menu in kiosk.
				Value: "screenplay-fff0f806-5b29-44a8-b48a-075c3d0e19f0",
			},
		},
	})
}

func FloatingAccessibilityMenuEnabled(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	param := s.Param().(kioskmode.TestData)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, kioskmode.CleanupDuration)
	defer cancel()

	kiosk, cr, err := kioskmode.New(
		ctx,
		fdms,
		s.RequiredVar("ui.signinProfileTestExtensionManifestKey"),
		kioskmode.PublicAccountPolicies(
			kioskmode.WebKioskAccountID,
			append(param.Policies, &policy.FloatingAccessibilityMenuEnabled{Val: true})...,
		),
		kioskmode.AutoLaunch(kioskmode.WebKioskAccountID),
	)
	if err != nil {
		s.Fatal("Failed to start Chrome in Kiosk mode: ", err)
	}
	defer func(ctx context.Context) {
		if err := kiosk.Close(ctx); err != nil {
			s.Error("Failed to close kiosk: ", err)
		}
	}(cleanupCtx)

	if err := kiosk.WaitLaunchLogs(ctx); err != nil {
		s.Fatal("Failed to launch Kiosk: ", err)
	}

	testConn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeWithScreenshotOnError(ctx, s.OutDir(), s.HasError, cr, "kiosk_with_FloatingAccessibilityMenuEnabled")

	if param.IsLacros {
		testing.ContextLog(ctx, "Checking if Kiosk started in Lacros mode")
		if _, err = lacrosproc.Root(ctx, testConn); err != nil {
			s.Fatal("Failed to get lacros proc: ", err)
		}
	}

	ui := uiauto.New(testConn)
	if err := ui.WaitUntilExists(nodewith.Name("Floating accessibility menu").HasClass("Widget"))(ctx); err != nil {
		s.Error("Failed to find floating accessibility menu: ", err)
	}
}
