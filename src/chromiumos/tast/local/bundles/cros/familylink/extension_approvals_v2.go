// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for writing Family Link tests.
package familylink

import (
	"context"
	"time"

	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/familylink"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ExtensionApprovalsV2,
		LacrosStatus: testing.LacrosVariantNeeded,
		Desc:         "Checks if Unicorn user can add extension with parent permission in the V2 UI",
		Contacts:     []string{"chromeos-sw-engprod@google.com", "courtneywong@chromium.org", "cros-families-eng+test@google.com"},
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
		Fixture: "familyLinkUnicornLoginWithExtensionApprovalsV2",
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

	// TODO(b/279664134): Remove passing only Ash in here when Lacros is enabled for Extension Approvals V2.
	if err := familylink.WaitForBoolPrefValueFromAshOrLacros(ctx, tconn, browser.TypeAsh, "profile.managed.extensions_may_request_permissions", true, 4*time.Minute); err != nil {
		s.Fatal("Failed to wait for pref: ", err)
	}

	// Set up browser.
	// TODO(b/279664134): Remove passing only Ash in here when Lacros is enabled for Extension Approvals V2.
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, browser.TypeAsh)
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
