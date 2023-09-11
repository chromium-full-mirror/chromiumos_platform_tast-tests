// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package quicksettings

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks that settings can be opened from Quick Settings",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-sw-engprod@google.com",
			"amehfooz@chromium.org",
		},
		BugComponent: "b:1246070", // ChromeOS > Software > System UI Surfaces > Status Area
		Attr:         []string{"group:mainline", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-39d4666a-39dc-40ed-a918-75acf57335a0",
		}},
		Params: []testing.Param{
			{
				Name:    "qs_revamp_enabled",
				Fixture: "chromeLoggedInQsRevampEnabled",
				Val:     true,
			},
			{
				Name:    "qs_revamp_disabled",
				Fixture: "chromeLoggedIn",
				Val:     false,
			},
		},
	})
}

// OpenSettings tests that we can open the settings app from Quick Settings.
func OpenSettings(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(*chrome.Chrome)
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	// TODO(crbug/1099502): replace this with quicksettings.Show when retry is no longer needed.
	if err := quicksettings.ShowWithRetry(ctx, tconn, 10*time.Second); err != nil {
		s.Fatal("Failed to open Quick Settings: ", err)
	}
	defer quicksettings.Hide(ctx, tconn)

	if err := quicksettings.OpenSettingsApp(ctx, tconn); err != nil {
		s.Fatal("Failed to open the Settings App from Quick Settings: ", err)
	}

	// Confirm that the Settings app is open by checking for the search box.
	if err := uiauto.New(tconn).WaitUntilExists(ossettings.SearchBoxFinder)(ctx); err != nil {
		s.Fatal("Waiting for Settings app search box failed: ", err)
	}
}
