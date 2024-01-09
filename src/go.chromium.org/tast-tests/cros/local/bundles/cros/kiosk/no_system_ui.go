// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kiosk

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NoSystemUI,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that no system UI is shown in PWA Kiosk",
		Contacts: []string{
			"chromeos-kiosk-eng+TAST@google.com",
			"irfedorova@google.com", // Test author
		},
		BugComponent: "b:892153", // ChromeOS > Software > Commercial (Enterprise) > Kiosk
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
		},
		SoftwareDeps: []string{"reboot", "chrome"},
		Timeout:      1 * time.Minute,
		Params: []testing.Param{
			{
				Name:    "ash",
				Fixture: fixture.KioskLoggedInAsh,
			}, {
				Name:              "lacros",
				ExtraSoftwareDeps: []string{"lacros"},
				Fixture:           fixture.KioskLoggedInLacros,
			},
		},
		SearchFlags: []*testing.StringPair{
			{
				Key: "feature_id",
				// Test auto-launched kiosk.
				Value: "screenplay-90708eaa-cdde-4060-8ccb-eeb305485531",
			},
			{
				Key: "feature_id",
				// Test launch PWA kiosk.
				Value: "screenplay-d93db63f-372e-4c3b-a1ca-3f23587dbac4",
			}}})
}

func NoSystemUI(ctx context.Context, s *testing.State) {
	// Default timeout for UI interactions.
	const defaultUITimeout = 5 * time.Second

	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	testConn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "kiosk_NoSystemUI")

	if err := ash.WaitForShelf(ctx, testConn, defaultUITimeout); err == nil {
		s.Fatal("Shelf is shown in Kiosk")
	}

	ui := uiauto.New(testConn).WithTimeout(defaultUITimeout)

	for _, param := range []struct {
		// class name for nodewith.finder
		className string
		// element name that would be included to error message
		errorElementName string
	}{
		{
			className:        "HomeButton",
			errorElementName: "'Launcher' button",
		},
		{
			className:        "UnifiedSystemTray",
			errorElementName: "'Status'",
		},
		{
			className:        "ToolbarView",
			errorElementName: "'Toolbar'",
		},
		{
			className:        "OmniboxViewViews",
			errorElementName: "'Omnibox'",
		},
	} {
		s.Run(ctx, param.className, func(ctx context.Context, s *testing.State) {
			finder := nodewith.HasClass(param.className).First()
			if err := ui.WaitUntilExists(finder)(ctx); err == nil {
				s.Fatal(param.errorElementName, " is shown in Kiosk")
			} else if !nodewith.IsNodeNotFoundErr(err) {
				s.Fatal("Failed to wait for ", param.errorElementName, ": ", err)
			}
		})
	}
}
