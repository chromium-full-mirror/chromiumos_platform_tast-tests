// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for writing Family Link tests.
package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         UnicornExtensions,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks if Unicorn user can add extension with parent permission",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"cros-oac@google.com",
			"galenemco@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		// This test has a long timeout because syncing settings can occasionally
		// take a long time.
		Timeout: 5 * time.Minute,
		VarDeps: []string{
			"family.parentEmail",
			"family.parentPassword",
		},
		Params: []testing.Param{{
			Val:     browser.TypeAsh,
			Fixture: "familyLinkUnicornLogin",
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
			Fixture:           "familyLinkUnicornLoginWithLacros",
		}},
	})
}

func UnicornExtensions(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	if cr == nil {
		s.Fatal("Failed to start Chrome")
	}
	if tconn == nil {
		s.Fatal("Failed to create test API connection")
	}

	if err := familylink.WaitForBoolPrefValueFromAshOrLacros(ctx, tconn, s.Param().(browser.Type), "profile.managed.extensions_may_request_permissions", true, 4*time.Minute); err != nil {
		s.Fatal("Failed to wait for pref: ", err)
	}
	// Set up browser.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	if err := familylink.AddExtension(ctx, cr, tconn, br); err != nil {
		s.Fatal("Failed to add extension: ", err)
	}

	if err := familylink.NavigateExtensionPermissionV1Dialog(ctx, tconn, s.RequiredVar("family.parentEmail"), s.RequiredVar("family.parentPassword")); err != nil {
		s.Fatal("Failed to navigate permission dialog: ", err)
	}
}
