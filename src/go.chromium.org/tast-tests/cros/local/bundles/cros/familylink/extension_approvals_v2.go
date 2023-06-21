// Copyright 2023 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtensionApprovalsV2,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks if Unicorn user can add extension with parent permission in the V2 UI",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "cros-families-eng+test@google.com", "courtneywong@chromium.org"},
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
			Fixture: "familyLinkUnicornLoginWithExtensionApprovalsV2",
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
			Fixture:           "familyLinkUnicornLoginWithLacrosAndExtensionApprovalsV2",
		}},
	})
}

func ExtensionApprovalsV2(ctx context.Context, s *testing.State) {
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

	ui := uiauto.New(tconn).WithTimeout(20 * time.Second)

	// Navigate the screen before parent authentication.
	testing.ContextLog(ctx, "Finding ask in person")
	askInPerson := nodewith.Name("Ask in person").Role(role.Button)
	if err := ui.WaitUntilExists(askInPerson)(ctx); err != nil {
		s.Fatal("Failed to find ask in person button: ", err)
	}

	testing.ContextLog(ctx, "Clicking ask in person")
	if err := ui.LeftClick(askInPerson)(ctx); err != nil {
		s.Fatal("Failed to ask permission for extension: ", err)
	}

	parentEmail := s.RequiredVar("family.parentEmail")
	parentPassword := s.RequiredVar("family.parentPassword")
	if err := familylink.NavigateParentAccessDialogAuthentication(ctx, tconn, parentEmail, parentPassword); err != nil {
		s.Fatal("Failed to navigate parent access widget: ", err)
	}

	// Only test the deny scenario, as clicking approve would change the state of the blocked sites list for the account.
	denyButton := nodewith.Name("Deny").Role(role.Button)
	if err := ui.WaitUntilExists(denyButton)(ctx); err != nil {
		s.Fatal("Failed to render Deny button: ", err)
	}

	testing.ContextLog(ctx, "Clicking deny")
	parentAccess := nodewith.Name("Parent access").Role(role.RootWebArea)
	if err := ui.DoDefault(denyButton)(ctx); err != nil {
		s.Fatal("Failed to click Deny button: ", err)
	}
	if err := ui.WaitUntilGone(parentAccess)(ctx); err != nil {
		s.Fatal("Parent access dialog is still open: ", err)
	}
}
