// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package calendar

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShowUpNext,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Checks the up next view shows correctly on the sys ui calendar view",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"chromeos-consumer-engprod@google.com",
			"samcackett@google.com",
		},
		BugComponent: "b:1246126", // ChromeOS > Software > System UI Surfaces > Status Area > Calendar
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "gaia"},
		SearchFlags: []*testing.StringPair{{
			Key:   "feature_id",
			Value: "screenplay-fed70290-27e6-403a-8956-d3ba9671fc90",
		}},
		Fixture: "chromeLoggedInWithUpcomingCalendarEvents",
	})
}

// ShowUpNext verifies that the up next view is shown correctly for any upcoming meetings.
func ShowUpNext(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	ui := uiauto.New(tconn)

	// Click on System tray to reset calendar.
	if err := ui.LeftClick(nodewith.ClassName("UnifiedSystemTray"))(ctx); err != nil {
		s.Fatal("Failed to click the system tray: ", err)
	}

	// Opens calendar view.
	dateTray := nodewith.HasClass("DateTray")
	calendarView := nodewith.ClassName("CalendarView")
	mainHeaderTriView := nodewith.ClassName("TriView").Ancestor(calendarView)
	mainHeader := nodewith.ClassName("CalendarHeaderView").Ancestor(mainHeaderTriView)

	if err := uiauto.Combine("Open calendar view",
		ui.LeftClick(dateTray),
		ui.WaitUntilExists(mainHeader),
	)(ctx); err != nil {
		s.Fatal("Failed to find calendar main label after opening calendar view: ", err)
	}

	// User should have an upcoming meeting.
	upNextView := nodewith.HasClass("CalendarUpNextView").Ancestor(calendarView)
	eventListView := nodewith.HasClass("CalendarEventListView").Ancestor(calendarView)
	showTodaysEventsButton := nodewith.Name("See all events for today").Role(role.Button).Ancestor(upNextView)
	if err := uiauto.Combine("Wait for up next to appear and click show todays events button",
		ui.WaitForLocation(upNextView),
		// Clicking show todays events should open event list view.
		ui.LeftClick(showTodaysEventsButton),
		ui.WaitForLocation(eventListView),
	)(ctx); err != nil {
		s.Log(uiauto.RootDebugInfo(ctx, tconn))
		s.Fatal("Testing up next view failed: ", err)
	}
}
