// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package familylink is used for writing Family Link tests.
package familylink

import (
	"context"
	"fmt"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome/familylink"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         IncognitoModeDisabled,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests that incognito mode is disabled for Unicorn users",
		Contacts: []string{
			"cros-families-eng+test@google.com",
			"chromeos-consumer-engprod@google.com",
		},
		// ChromeOS > Software > Family > Parental controls
		BugComponent: "b:1090157",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "gaia"},
		Timeout:      time.Minute,
		Fixture:      "familyLinkUnicornLogin",
	})
}

func IncognitoModeDisabled(ctx context.Context, s *testing.State) {
	tconn := s.FixtValue().(familylink.HasTestConn).TestConn()

	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	ui := uiauto.New(tconn)

	// Get the primary browser.
	chromeApp, err := apps.ChromeOrChromium(ctx, tconn)
	if err != nil {
		s.Fatal("Failed to find Chrome or Chromium app: ", err)
	}

	// Chrome app name doesn't exactly match the chrome shelf name so modify it here for simpler code later.
	if chromeApp.Name == apps.Chrome.Name {
		chromeApp.Name = "Google Chrome"
	}

	s.Logf("Right clicking the %s shelf app button", chromeApp.Name)
	if err := uiauto.Combine(fmt.Sprintf("Right clicking the %s shelf app button", chromeApp.Name),
		ui.RightClick(nodewith.Name(chromeApp.Name).Role(role.Button)),
		ui.WaitUntilExists(nodewith.Role(role.MenuItem).First()))(ctx); err != nil {
		s.Fatal(fmt.Sprintf("Failed to right click the %s shelf app button: ", chromeApp.Name), err)
	}

	s.Log("Verifying the New Incognito window menu item does not exist")
	if err := ui.Exists(nodewith.Name("New Incognito window").Role(role.MenuItem))(ctx); err == nil {
		s.Fatal("Incognito mode detected and enabled: ", err)
	}
}
