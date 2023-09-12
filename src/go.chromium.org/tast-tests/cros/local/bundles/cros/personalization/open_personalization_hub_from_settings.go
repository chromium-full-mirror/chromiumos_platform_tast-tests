// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package personalization

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/personalization"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenPersonalizationHubFromSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test opening personalization hub app from Settings app",
		Contacts: []string{
			"assistive-eng@google.com",
			"pzliu@google.com",
			"chromeos-sw-engprod@google.com",
		},
		// ChromeOS > Software > Personalization
		BugComponent: "b:1006527",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-e92e2d70-5969-4405-9cdd-c3ecee573f81",
		}},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedIn",
	})
}

func OpenPersonalizationHubFromSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(cleanupCtx, s.OutDir(), s.HasError, tconn)

	// The test has a dependency of network speed, so we give uiauto.Context ample
	// time to wait for nodes to load.
	ui := uiauto.New(tconn).WithTimeout(30 * time.Second)
	kb, err := input.Keyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get keyboard: ", err)
	}
	defer kb.Close(ctx)

	if err := uiauto.Combine("open personalization hub from settings",
		personalization.SearchForAppInLauncher(personalization.SettingsSearchTerm, personalization.SettingsAppName, kb, ui),
		ui.LeftClick(nodewith.Role(role.Link).NameContaining(personalization.Personalization).HasClass("item")),
		ui.LeftClick(nodewith.Role(role.Link).NameContaining(personalization.SettingsSetWallpaper).First()),
		ui.WaitUntilExists(personalization.PersonalizationHubWindow),
	)(ctx); err != nil {
		s.Fatal("Failed to open personalization hub from settings: ", err)
	}

	if err := uiauto.Combine("open personalization hub by searching in settings",
		personalization.SearchForAppInLauncher(personalization.SettingsSearchTerm, personalization.SettingsAppName, kb, ui),
		ui.WaitUntilExists(nodewith.Role(role.TextField).HasClass("Textfield")),
		kb.TypeAction(personalization.PersonalizationSearchTerm),
		kb.AccelAction("Enter"),
		ui.WaitUntilExists(personalization.PersonalizationHubWindow),
	)(ctx); err != nil {
		s.Fatal("Failed to open personalization hub by searching in settings: ", err)
	}
}
