// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/family"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/state"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MultipleSignInDisabled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that multiple sign-in is disabled for Unicorn users. Geller users should behave similarly",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      5 * time.Minute,
		VarDeps:      []string{family.UnicornAccountVarName},
		Fixture:      "familyLinkUnicornLoginNonOwner",
	})
}

func MultipleSignInDisabled(ctx context.Context, s *testing.State) {
	// After the fixture runs, the DUT has two users on the
	// device: a regular owner and a Unicorn secondary user. The
	// Unicorn user is logged in.
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	ui := uiauto.New(tconn)

	s.Log("Opening the system status tray")
	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to open Quick Settings: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)

	// On failure, dump the UI tree while quicksettings is still open.
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	s.Log("Opening power menu")
	if err := ui.LeftClick(quicksettings.PowerMenuButton)(ctx); err != nil {
		s.Fatal("Failed to find and click power menu button: ", err)
	}
	// Close the menu when done.
	defer ui.LeftClick(quicksettings.PowerMenuButton)(ctx)

	s.Log("Attempting to add multiple profiles")
	userEmail, _, err := dma.UserPassFromPool(family.UnicornAccountVarName)
	if err != nil {
		s.Fatal("Failed to get child user: ", err)
	}
	s.Logf("Looking for user email menu item %q", userEmail)
	emailMenuItem := nodewith.NameContaining(userEmail).Role(role.MenuItem)
	if err := ui.WaitUntilExists(emailMenuItem)(ctx); err != nil {
		s.Fatal("Failed to find the user email menu item: ", err)
	}

	// Family Link users are never allowed to use multi-user sign-
	// in, so the the menu item should be disabled.
	info, err := ui.Info(ctx, emailMenuItem)
	if err != nil {
		s.Fatal("Failed to get info for email menu item: ", err)
	}
	if info.State[state.Focusable] {
		s.Fatal("User profile button should be disabled for Family Link users")
	}
}
