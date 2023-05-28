// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         MultipleSignInDisabled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that multiple sign-in is disabled for Unicorn users. Geller users should behave similarly",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"zork@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		VarDeps:      []string{"family.unicornEmail"},
		Fixture:      "familyLinkUnicornLoginNonOwner",
	})
}

func MultipleSignInDisabled(ctx context.Context, s *testing.State) {
	// After the fixture runs, the DUT has two users on the
	// device: a regular owner and a Unicorn secondary user. The
	// Unicorn user is logged in.
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)
	ui := uiauto.New(tconn)

	s.Log("Opening the system status tray")
	if err := quicksettings.Show(ctx, tconn); err != nil {
		s.Fatal("Failed to open Quick Settings: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)

	s.Log("Attempting to add multiple profiles")
	userEmail := s.RequiredVar("family.unicornEmail")
	s.Logf("Looking for user email %q", userEmail)
	userProfileIcon := nodewith.NameContaining(userEmail).Role(role.Button)
	if err := ui.WaitUntilExists(userProfileIcon)(ctx); err != nil {
		s.Fatal("Failed to find the user profile icon: ", err)
	}

	// Family Link users are never allowed to use multi-user sign-
	// in, so the the profile icon button should be disabled.
	if err := ui.WaitUntilExists(userProfileIcon.Focusable())(ctx); err == nil {
		s.Fatal("User profile button should be disabled for Family Link users: ", err)
	}
}
