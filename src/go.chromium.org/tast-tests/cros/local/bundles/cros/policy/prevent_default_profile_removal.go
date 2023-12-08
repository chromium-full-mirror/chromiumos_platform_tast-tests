// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/lacros"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PreventDefaultProfileRemoval,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Attempts to remove the default user profile in Lacros",
		Contacts: []string{
			"cros-engprod-muc@google.com",
			"eariassoto@google.com", // Test author
		},
		BugComponent: "b:1263917",
		// TODO(b/229003599): This test is failing on tablets and Chromeboxes.
		// We are disabling until the root cause is fixed.
		// Attr:         []string{"group:golden_tier"},
		SoftwareDeps: []string{"chrome", "lacros"},
		Fixture:      fixture.LacrosPolicyLoggedIn,
		Attr:         []string{"group:hw_agnostic"},
	})
}

func PreventDefaultProfileRemoval(ctx context.Context, s *testing.State) {
	// Reserve ten seconds for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Second)
	defer cancel()

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to connect to test API: ", err)
	}

	// Open an empty Lacros window.
	l, err := lacros.Launch(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to open lacros: ", err)
	}
	defer l.Close(ctx)

	// Dump the UI tree before we close lacros.
	defer faillog.DumpUITreeWithScreenshotOnError(cleanupCtx, s.OutDir(), s.HasError, cr, "ui_tree")

	// Start interacting with the UI
	ui := uiauto.New(tconn)
	buttonWith := nodewith.Role(role.Button).Focusable()

	if err := uiauto.Combine("open profile settings",
		ui.LeftClick(buttonWith.ClassName("AvatarToolbarButton")),
		ui.LeftClick(buttonWith.Name("Manage profiles")),
		ui.LeftClick(buttonWith.Name("More actions")),
		ui.LeftClick(nodewith.Role(role.MenuItem).Focusable().ClassName("dropdown-item").Name("Delete")),
	)(ctx); err != nil {
		s.Fatal("Failed to manipulate ui: ", err)
	}

	if err := ui.WaitUntilExists(nodewith.Role(role.Dialog).Name("Can't delete this profile"))(ctx); err != nil {
		s.Fatal("Expected error dialog that the profile cannot be deleted: ", err)
	}
}
