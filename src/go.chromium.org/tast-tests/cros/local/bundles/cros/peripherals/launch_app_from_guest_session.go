// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package peripherals

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/local/apps"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/ash"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast/core/testing"
)

// guestModeTest contains all the data needed to run a single test iteration.
type guestModeTest struct {
	app apps.App
}

func init() {
	testing.AddTest(&testing.Test{
		Func: LaunchAppFromGuestSession,
		Desc: "Peripherals app can be found and launched from guest mode",
		Contacts: []string{
			"cros-device-enablement@google.com",
			"michaelcheco@google.com",
			"jimmyxgong@google.com",
			"ashleydp@google.com",
		},
		// ChromeOS > Software > System Services > Peripherals
		BugComponent: "b:1150827",
		Attr:         []string{"group:mainline", "group:hw_agnostic", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInGuest",
		Params: []testing.Param{
			{
				Name: "diagnostics",
				Val: guestModeTest{
					app: apps.Diagnostics,
				},
			},
		},
	})
}

// LaunchAppFromGuestSession verifies launching an app from guest mode.
func LaunchAppFromGuestSession(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	// Attempt to open the Test API connection.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API Connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	params := s.Param().(guestModeTest)

	err = apps.Launch(ctx, tconn, params.app.ID)
	if err != nil {
		s.Fatal("Failed to launch app: ", err)
	}

	err = ash.WaitForApp(ctx, tconn, params.app.ID, time.Minute)
	if err != nil {
		s.Fatal("Could not find app in shelf after launch: ", err)
	}
}
