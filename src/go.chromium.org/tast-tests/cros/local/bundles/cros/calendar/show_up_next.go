// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package calendar

import (
	"context"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/role"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ShowUpNext,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks the up next view shows correctly on the sys ui calendar view",
		Contacts: []string{
			"cros-status-area-eng@google.com",
			"samcackett@google.com",
		},
		BugComponent: "b:1246126", // ChromeOS > Software > System UI Surfaces > Status Area > Calendar
		Attr:         []string{"group:mainline", "informational"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      "chromeLoggedInWithUpcomingCalendarEvents",
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

	// Opens calendar view.
	dateTray := nodewith.HasClass("DateTray")
	calendarView := nodewith.HasClass("CalendarView")
	if err := uiauto.Combine("Open calendar view",
		ui.DoDefault(dateTray),
		ui.WaitForLocation(calendarView),
	)(ctx); err != nil {
		s.Fatal("Failed to open Calendar View: ", err)
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

	// Closes the calendar view.
	calendarViewBounds, err := ui.Location(ctx, calendarView)
	if err != nil {
		s.Fatal("Failed to find calendar view bounds: ", err)
	}
	outsideCalendarPt := coords.NewPoint(calendarViewBounds.Right()+2, calendarViewBounds.Top-5)
	if err := mouse.Click(tconn, outsideCalendarPt, mouse.LeftButton)(ctx); err != nil {
		s.Fatal("Failed to click outside of the calendar view: ", err)
	}
}
