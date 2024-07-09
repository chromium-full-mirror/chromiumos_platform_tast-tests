// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for writing Family Link tests.
package familylink

import (
	"context"
	"time"

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
		Func:         WebFilteringAllowlist,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that web filtering allowlist mode work correctly: websites on the list are allowed and websites outside of the list are blocked",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      5 * time.Minute,
		Vars:         []string{"unicorn.siteOnAllowlist", "unicorn.siteNotOnAllowlist", "unicorn.siteOnAllowlistNodeName"},
		Fixture:      "familyLinkUnicornWebAllowlistLogin",
	})
}

func WebFilteringAllowlist(ctx context.Context, s *testing.State) {
	// Reserve time for cleanup.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	s.Log("Opening a URL that is not on allowlist")
	notOnAllowlist := s.RequiredVar("unicorn.siteNotOnAllowlist")
	conn, err := cr.NewConn(ctx, notOnAllowlist)
	if err != nil {
		s.Fatal("Failed to navigate to site NOT on allowlist : ", err)
	}
	defer conn.Close()

	s.Log("Verifying that URL not on allowlist has been blocked")
	ui := uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Name("Site blocked").Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Site NOT on allowlist is NOT blocked for Unicorn user: ", err)
	}

	s.Log("Opening a URL that is on allowlist")
	onAllowlist := s.RequiredVar("unicorn.siteOnAllowlist")
	conn, err = cr.NewConn(ctx, onAllowlist)
	if err != nil {
		s.Fatal("Failed to navigate to site on allowlist : ", err)
	}
	defer conn.Close()

	s.Log("Verifying that URL on allowlist has been loaded")
	onAllowlistNode := s.RequiredVar("unicorn.siteOnAllowlistNodeName")
	ui = uiauto.New(tconn)
	if err := ui.WaitUntilExists(nodewith.Name(onAllowlistNode).Role(role.StaticText))(ctx); err != nil {
		s.Fatal("Site on allowlist failed to load for Unicorn user: ", err)
	}
}
