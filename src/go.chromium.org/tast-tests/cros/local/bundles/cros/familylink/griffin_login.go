// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for testing parental controls and experiences of Family Link users.
package familylink

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/family"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GriffinLogin,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks if login is working for Family Link Griffin account",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      2 * time.Minute,
		Fixture:      "familyLinkGriffinLogin",
		VarDeps: []string{
			family.GriffinAccountVarName,
		},
	})
}

func GriffinLogin(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	if cr == nil {
		s.Fatal("Failed to start Chrome")
	}
	if tconn == nil {
		s.Fatal("Failed to create test API connection")
	}

	user, _, err := dma.UserPassFromPool(family.GriffinAccountVarName)
	if err != nil {
		s.Fatal("Failed to get child user: ", err)
	}

	if err := familylink.VerifyUserSignedIntoBrowserAsChild(ctx, cr, tconn, user, s.OutDir()); err != nil {
		s.Fatal("Failed to verify user signed into browser: ", err)
	}
}
