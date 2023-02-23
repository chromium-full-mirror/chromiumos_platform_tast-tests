// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"time"

	"chromiumos/tast/common/fixture"
	"chromiumos/tast/common/policy"
	"chromiumos/tast/common/policy/fakedms"
	"chromiumos/tast/local/annotations"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/local/chrome/browser"
	"chromiumos/tast/local/chrome/browser/browserfixt"
	"chromiumos/tast/local/chrome/uiauto"
	"chromiumos/tast/local/chrome/uiauto/faillog"
	"chromiumos/tast/local/chrome/uiauto/mouse"
	"chromiumos/tast/local/chrome/uiauto/nodewith"
	"chromiumos/tast/local/coords"
	"chromiumos/tast/local/policyutil"
	"chromiumos/tast/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CalendarIntegrationEnabled,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Checks behavior of CalendarIntegrationEnabled policy, check if event list is shown based on value of the policy",
		BugComponent: "b:1129862",
		Contacts: []string{
			"dp-chromeos-eng@google.com",
			"crmullins@google.com",
		},
		Attr:         []string{"group:commercial_limited"},
		SoftwareDeps: []string{"chrome"},
		Params: []testing.Param{{
			Fixture: fixture.ChromePolicyLoggedIn,
			Val:     browser.TypeAsh,
		}, {
			Name:              "lacros",
			ExtraSoftwareDeps: []string{"lacros"},
			Fixture:           fixture.LacrosPolicyLoggedIn,
			Val:               browser.TypeLacros,
		}},
	})
}

func CalendarIntegrationEnabled(ctx context.Context, s *testing.State) {
	cr := s.FixtValue().(chrome.HasChrome).Chrome()
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}
	defer faillog.DumpUITreeOnError(ctx, s.OutDir(), s.HasError, tconn)

	for _, param := range []struct {
		name                    string
		shouldFindEventListView bool
		shouldFindManagedIcon   bool
		shouldFindAnnotation    bool
		policy                  *policy.CalendarIntegrationEnabled
	}{
		{
			name:                    "unset",
			shouldFindEventListView: true,
			shouldFindManagedIcon:   false,
			shouldFindAnnotation:    false, /* TODO(b/262281684): investigate why this is not found with unset policy */
			policy:                  &policy.CalendarIntegrationEnabled{Stat: policy.StatusUnset},
		},
		{
			name:                    "enabled",
			shouldFindEventListView: true,
			shouldFindManagedIcon:   false,
			shouldFindAnnotation:    false, /* TODO(b/262281684): investigate why this is not found with enabled policy */
			policy:                  &policy.CalendarIntegrationEnabled{Val: true},
		},
		{
			name:                    "disabled",
			shouldFindEventListView: false,
			shouldFindManagedIcon:   true,
			shouldFindAnnotation:    false,
			policy:                  &policy.CalendarIntegrationEnabled{Val: false},
		},
	} {
		s.Run(ctx, param.name, func(ctx context.Context, s *testing.State) {

			// Perform cleanup.
			if err := policyutil.ResetChrome(ctx, fdms, cr); err != nil {
				s.Fatal("Failed to clean up: ", err)
			}

			// Update policies.
			if err := policyutil.ServeAndVerify(ctx, fdms, cr, []policy.Policy{param.policy}); err != nil {
				s.Fatal("Failed to update policies: ", err)
			}

			// Setup browser based on the chrome type.
			// br, closeBrowser, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))
			br, _, err := browserfixt.SetUp(ctx, cr, s.Param().(browser.Type))

			ui := uiauto.New(tconn)
			s.Log("Start testing calendar view from date tray")
			dateTray := nodewith.HasClass("DateTray")

			// Comparing the time before and after opening the calendar view just in case this test is run at the very end of a year, e.g. Dec 31 23:59:59
			beforeOpeningCalendarYear := time.Now().Year()

			// Open the net-export page and start logging.
			if err := annotations.StartLogging(ctx, cr, br); err != nil {
				s.Fatal("Failed to start logging: ", err)
			}

			if err := ui.DoDefault(dateTray)(ctx); err != nil {
				s.Fatal("Failed to click the date tray: ", err)
			}

			calendarView := nodewith.HasClass("CalendarView")
			mainHeaderTriView := nodewith.HasClass("TriView").Ancestor(calendarView).Nth(0)
			mainHeaderContainer := nodewith.HasClass("View").Ancestor(mainHeaderTriView).Nth(1)
			mainHeader := nodewith.Name("Calendar").HasClass("Label").Ancestor(mainHeaderContainer)

			if err := ui.WaitUntilExists(mainHeader)(ctx); err != nil {
				s.Fatal("Failed to find calendar main label after opening calendar view: ", err)
			}

			// For some corner cases, if it cannot find the year label with the time before clicking on the date tray, it should find the year label with the time after the calendar view is open.
			// E.g. before opening it's Dec 31 23:59:59 2022, and after openting it's Jan 1 00:00 2023.
			yearInt := beforeOpeningCalendarYear
			beforeOpeningCalendarYearLabel := nodewith.Name(strconv.Itoa(beforeOpeningCalendarYear)).HasClass("Label").Onscreen()
			if found, err := ui.IsNodeFound(ctx, beforeOpeningCalendarYearLabel); err != nil {
				s.Fatal("Failed to check beforeOpeningCalendarYearLabel after clicking on the date tray: ", err)
			} else if found != true {
				yearInt = time.Now().Year()
			}

			// Opening the calendar view should show today's year label.
			year := strconv.Itoa(yearInt)
			todayYearLabel := nodewith.Name(year).HasClass("Label").Onscreen()
			if err := ui.WaitUntilExists(todayYearLabel)(ctx); err != nil {
				s.Fatal("Failed to find year label after opening calendar view: ", err)
			}

			// Clicks on a Monday's cell to show the event list view.
			scrollView := nodewith.HasClass("ScrollView").Ancestor(calendarView).Nth(0)
			scrollViewport := nodewith.HasClass("ScrollView::Viewport").Ancestor(scrollView).Nth(0)
			contentView := nodewith.HasClass("View").Ancestor(scrollViewport).Nth(0)
			currentMonthView := nodewith.HasClass("View").Ancestor(contentView).Nth(3)
			firstMondayDateCell := nodewith.HasClass("CalendarDateCellView").Ancestor(currentMonthView).Nth(1)
			scrollViewBounds, err := ui.Location(ctx, scrollView)
			if err != nil {
				s.Fatal("Failed to find calendar scroll view bounds: ", err)
			}
			firstMondayDateCellBounds, err := ui.Location(ctx, firstMondayDateCell)
			if err != nil {
				s.Fatal("Failed to find calendar first Monday cell bounds: ", err)
			}

			// TODO(b/234673735): Should click on the finder directly after this bug is fixed.
			// Currently the vertical location of the cells fetched from |ui.Location| are not correct.
			// It returns the same number for the |Top| of all the cells, which is the same number as the |Top| of the scroll view.
			// This might because of the scroll view is nested in some other views.
			// Here a small amount (5) of pixel is added to the top of the scroll view each time in the loop to find the first available Monday cell with events.
			const findCellTimes = 20
			didFindEventListView := false
			cellPositionY := 0
			eventListView := nodewith.HasClass("CalendarEventListView").Ancestor(calendarView)
			eventCloseButtonViewContainer := nodewith.HasClass("View").Ancestor(eventListView).Nth(0)
			eventCloseButtonView := nodewith.HasClass("IconButton").Ancestor(eventCloseButtonViewContainer).Nth(0)
			for i := 0; i < findCellTimes; i++ {
				s.Logf("Moving towards the first Monday cell (iteration %d of %d)", i+1, findCellTimes)
				cellPositionY += 5
				firstMondayDateCellPt := coords.NewPoint(firstMondayDateCellBounds.CenterX(), scrollViewBounds.Top+cellPositionY)
				if err := mouse.Click(tconn, firstMondayDateCellPt, mouse.LeftButton)(ctx); err != nil {
					s.Fatal("Failed to click the first Monday date cell: ", err)
				}
				if found, err := ui.IsNodeFound(ctx, eventCloseButtonView); err != nil {
					s.Fatal("Failed to check event list view close button while finding the first Monday cell: ", err)
				} else if found == true {
					didFindEventListView = true
					break
				}
			}

			didFindManagedIcon := false
			rightHeaderContainer := nodewith.HasClass("View").Ancestor(mainHeaderTriView).Nth(2)
			managedIcon := nodewith.Name("Disabled by admin").HasClass("IconButton").Ancestor(rightHeaderContainer)
			if found, err := ui.IsNodeFound(ctx, managedIcon); err != nil {
				s.Fatal("Failed to check for managed icon in calendar tray: ", err)
			} else if found == true {
				didFindManagedIcon = true
			}

			didFindAnnotation := false
			// Stop logging and check the logs for calendar_get_events NetworkTrafficAnnotationTag with hash 86429515
			if foundAnnotation, err := annotations.StopLoggingCheckLogs(ctx, cr, br, "86429515"); err != nil {
				s.Fatal("Failed to stop logging and check logs: ", err)
			} else if foundAnnotation == true {
				didFindAnnotation = true
			}

			// Check for enterprise management icon
			if param.shouldFindManagedIcon && didFindManagedIcon == false {
				s.Fatal("Did not find expected Disabled by Admin icon")
			}

			if !param.shouldFindManagedIcon && didFindManagedIcon == true {
				s.Fatal("Found unexpected Disabled by Admin icon")
			}

			// Check for event list
			if param.shouldFindEventListView && didFindEventListView == false {
				s.Fatal("Did not find expected event list view")
			}

			if !param.shouldFindEventListView && didFindEventListView == true {
				s.Fatal("Found unexpected event list view")
			}

			// Check for annotation
			if param.shouldFindAnnotation && didFindAnnotation == false {
				s.Fatal("Did not find expected NetworkTrafficAnnotationTag with id calendar_get_events")
			}

			if !param.shouldFindAnnotation && didFindAnnotation == true {
				s.Fatal("Found unexpected NetworkTrafficAnnotationTag with id calendar_get_events")
			}

			// Click it again to close the calendar view
			if err := ui.DoDefault(dateTray)(ctx); err != nil {
				s.Fatal("Failed to click the date tray: ", err)
			}

		})
	}
}
