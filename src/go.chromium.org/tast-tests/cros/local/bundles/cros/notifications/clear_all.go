// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package notifications

import (
	"context"
	"fmt"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/browser/browserfixt"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ClearAll,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks that the 'Clear all' button dismisses all notifications",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"amehfooz@chromium.org",
			"cros-system-ui-eng@google.com",
		},
		BugComponent: "b:1246021", // ChromeOS > Software > System UI Surfaces > Notifications
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-39d4666a-39dc-40ed-a918-75acf57335a0",
		}},
		Params: []testing.Param{{
			Fixture: "chromeLoggedIn",
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			Fixture:           "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Val:               browser.TypeLacros,
		}},
	})
}

// ClearAll tests that several notifications can be dismissed with the 'Clear all' button.
func ClearAll(ctx context.Context, s *testing.State) {
	const uiTimeout = 30 * time.Second

	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// Setup a browser.
	bt := s.Param().(browser.Type)
	br, closeBrowser, err := browserfixt.SetUp(ctx, cr, bt)
	if err != nil {
		s.Fatal("Failed to open the browser: ", err)
	}
	defer closeBrowser(ctx)
	bTconn, err := br.TestAPIConn(ctx)
	if err != nil {
		s.Fatalf("Failed to create Test API connection for %v browser: %v", bt, err)
	}

	cleanup, err := ash.EnsureTabletModeEnabled(ctx, tconn, false)
	if err != nil {
		s.Fatal("Failed to ensure DUT is not in tablet mode: ", err)
	}
	defer cleanup(ctx)

	const baseTitle = "TestNotification"
	const n = 10
	for i := 0; i < n; i++ {
		title := fmt.Sprintf("%s%d", baseTitle, i)
		if _, err := browser.CreateTestNotification(ctx, bTconn, browser.NotificationTypeBasic, title, "blahhh"); err != nil {
			s.Fatalf("Failed to create test notification %v: %v", i, err)
		}
		if _, err := ash.WaitForNotification(ctx, tconn, uiTimeout, ash.WaitTitle(title)); err != nil {
			s.Fatalf("Failed waiting for %v: %v", title, err)
		}
	}

	// Open notification center to ensure the 'Clear all' button is available.
	if err := quicksettings.ShowNotificationCenter(ctx, tconn); err != nil {
		s.Fatal("Failed to open notification center: ", err)
	}
	// Hiding all notifications ensures the notification center closes.
	defer ash.CloseNotifications(ctx, tconn)

	ui := uiauto.New(tconn)
	clearAll := nodewith.Name("Clear all").Role(role.StaticText)
	if err := ui.WithTimeout(uiTimeout).LeftClick(clearAll)(ctx); err != nil {
		s.Fatal("Failed to click 'Clear all' button: ", err)
	}

	// Wait until all notifications and the 'Clear all' button are gone.
	// The notification names change based on their title and content, so partially match the name attribute.
	r, err := regexp.Compile(baseTitle)
	if err != nil {
		s.Fatal("Failed to compile notification regex: ", err)
	}
	notification := nodewith.ClassName("MessageView").Attribute("name", r)
	if err := ui.WithTimeout(uiTimeout).WaitUntilGone(notification)(ctx); err != nil {
		s.Fatal("Failed waiting for notifications to be dismissed: ", err)
	}
	if err := ui.WithTimeout(uiTimeout).WaitUntilGone(clearAll)(ctx); err != nil {
		s.Fatal("Failed waiting for 'Clear all' button to be gone: ", err)
	}
}
