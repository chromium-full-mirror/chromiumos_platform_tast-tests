// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for writing Family Link tests.
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
		Func:         UnicornLogin,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks if Unicorn login is working",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-sw-engprod@google.com",
			"cros-oac@google.com",
			"zork@chromium.org",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Timeout:      time.Minute,
		VarDeps: []string{
			"family.parentEmail",
			"family.parentPassword",
			"family.unicornEmail",
			"family.unicornPassword",
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

func UnicornLogin(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	if cr == nil {
		s.Fatal("Failed to start Chrome")
	}
	if tconn == nil {
		s.Fatal("Failed to create test API connection")
	}
	if err := familylink.VerifyUserSignedIntoBrowserAsChild(ctx, cr, tconn, s.Param().(browser.Type), s.RequiredVar("family.unicornEmail"), s.OutDir()); err != nil {
		s.Fatal("Failed to verify user signed into browser: ", err)
	}
}
