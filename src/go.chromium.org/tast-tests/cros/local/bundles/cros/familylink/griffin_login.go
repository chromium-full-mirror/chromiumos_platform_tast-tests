// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for testing parental controls and experiences of Family Link users.
package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GriffinLogin,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks if login is working for Family Link Griffin account",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"cros-oac@google.com",
			"agawronska@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      2 * time.Minute,
		Params: []testing.Param{{
			Fixture: "familyLinkGriffinLogin",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           "familyLinkGriffinLoginWithLacros",
			Val:               browser.TypeLacros,
		}},
		VarDeps: []string{
			"family.griffinEmail",
		},
	})
}

func GriffinLogin(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	// GoBigSleepLint - Wait for login.
	// TODO(b/254891227): Remove this when chrome.New() doesn't have a race condition.
	if err := testing.Sleep(ctx, 5*time.Second); err != nil {
		s.Fatal("Failed to wait for Login to complete: ", err)
	}

	if cr == nil {
		s.Fatal("Failed to start Chrome")
	}
	if tconn == nil {
		s.Fatal("Failed to create test API connection")
	}
	if err := familylink.VerifyUserSignedIntoBrowserAsChild(ctx, cr, tconn, s.Param().(browser.Type), s.RequiredVar("family.griffinEmail"), s.OutDir()); err != nil {
		s.Fatal("Failed to verify user signed into browser: ", err)
	}
}
