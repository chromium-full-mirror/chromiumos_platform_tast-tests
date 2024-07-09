// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for writing Family Link tests.
package familylink

import (
	"context"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/family"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         LocalWebApprovals,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that parent can approve blocked sites locally",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      5 * time.Minute,
		Vars:         []string{"unicorn.matureSite"},
		Fixture:      "familyLinkUnicornLogin",
	})
}

func LocalWebApprovals(ctx context.Context, s *testing.State) {
	// Reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	matureSite := s.RequiredVar("unicorn.matureSite")

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	conn, err := cr.NewConn(ctx, matureSite)
	if err != nil {
		s.Fatal("Failed to navigate to website: ", err)
	}
	defer conn.Close()

	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Name("Site blocked").Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Mature website is not blocked for Unicorn user: ", err)
	}

	testing.ContextLog(ctx, "Clicking ask in person")
	askInPerson := nodewith.Name("Ask in person").Role(role.Button)
	if err := ui.DoDefault(askInPerson)(ctx); err != nil {
		s.Fatal("Failed to load parent access widget: ", err)
	}

	parentEmail, parentPassword, err := dma.UserPassFromPool(family.ParentAccountVarName)
	if err != nil {
		s.Fatal("Failed to get parent user and pass: ", err)
	}
	if err := familylink.NavigateParentAccessDialogAuthentication(ctx, tconn, parentEmail, parentPassword); err != nil {
		s.Fatal("Failed to navigate parent access widget: ", err)
	}

	// Only test the deny scenario, as clicking approve would change the state of the blocked sites list for the account.
	denyButton := nodewith.Name("Deny").Role(role.Button)
	if err := ui.WaitUntilExists(denyButton)(ctx); err != nil {
		s.Fatal("Failed to render Deny button: ", err)
	}

	// Label for the site appears without the subdomain or prefix www.
	matureSite = strings.ReplaceAll(matureSite, "www.", "")

	if err := ui.Exists(nodewith.NameContaining(matureSite).Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Blocked website URL is not shown: ", err)
	}

	testing.ContextLog(ctx, "Clicking deny")
	parentAccess := nodewith.Name("Parent access").Role(role.RootWebArea)
	if err := ui.DoDefault(denyButton)(ctx); err != nil {
		s.Fatal("Failed to click deny button: ", err)
	}
	if err := ui.WaitUntilGone(parentAccess)(ctx); err != nil {
		s.Fatal("Parent access dialog is still open: ", err)
	}
}
