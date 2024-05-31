// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package assistant

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/assistant"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/ossettings"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OpenSettings,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Tests opening the Settings app using an Assistant query",
		Contacts:     []string{"assistive-eng@google.com"},
		BugComponent: "b:905229", // ChromeOS > Software > Assistive
		Attr: []string{
			"group:mainline",
			"informational",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "chrome_internal", "gaia"},
		Fixture:      "assistantWithGaia",
	})
}

// OpenSettings tests that the Settings app can be opened by the Assistant
func OpenSettings(ctx context.Context, s *testing.State) {
	fixtData := s.FixtValue().(*assistant.FixtData)
	cr := fixtData.Chrome

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Creating test API connection failed: ", err)
	}

	// Run query to open the Settings window.
	// assistant.SendTextQuery returns an error even when Settings launches successfully,
	// so check for that here instead of processing the returned error.
	// todo (crbug/1080366): process the error from assistantSendTextQuery.
	s.Log("Launching Settings app with Assistant query and waiting for it to open")
	_, assistErr := assistant.SendTextQuery(ctx, tconn, "open settings")
	if err := ash.WaitForApp(ctx, tconn, apps.Settings.ID, time.Minute); err != nil {
		s.Fatalf("Settings app did not appear in the shelf: %v. Last assistant.SendTextQuery error: %v", err, assistErr)
	}

	// Confirm that the Settings app is open by checking for the search box.
	if err := uiauto.New(tconn).WaitUntilExists(ossettings.SearchBoxFinder)(ctx); err != nil {
		s.Fatal("Waiting for Settings app search box failed: ", err)
	}
}
