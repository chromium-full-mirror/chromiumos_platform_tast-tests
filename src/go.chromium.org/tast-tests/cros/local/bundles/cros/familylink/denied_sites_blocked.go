// Copyright 2021 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         DeniedSitesBlocked,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that parent-blocked sites are blocked for Unicorn users",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      5 * time.Minute,
		Vars:         []string{"unicorn.blockedSite"},
		Params: []testing.Param{{
			Fixture: "familyLinkUnicornLogin",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "familyLinkUnicornLoginWithLacros",
			Val:               browser.TypeLacros,
		}},
	})
}

func DeniedSitesBlocked(ctx context.Context, s *testing.State) {
	// Reserve time for cleanup.
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	blockedSite := s.RequiredVar("unicorn.blockedSite")

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
	if err != nil {
		s.Fatal("Failed to set up browser: ", err)
	}
	defer closeBrowser(cleanupCtx)

	// The allow/block list can take a while to sync so loop checking
	// for the website to be blocked.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		conn, err := br.NewConn(ctx, blockedSite)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to open browser to website"))
		}
		defer conn.Close()

		if err := ui.WaitUntilExists(nodewith.Name("Ask your parent").Role(role.StaticText))(ctx); err != nil {
			return errors.Wrap(err, "failed to detect blocked site interstitial")
		}
		return nil
	}, &testing.PollOptions{Timeout: 5 * time.Minute}); err != nil {
		s.Fatal("Parent-blocked website is not blocked for Unicorn user: ", err)
	}
}
