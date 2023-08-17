// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package calendarintegration contains helpers to verify the calendar
// integration.
package calendarintegration

import (
	"context"
	"net/http/httptest"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/policy/networkrequestmonitor"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/mouse"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/nodewith"
	"go.chromium.org/tast-tests/cros/local/coords"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// AnnotationHashCode is the hashcode of network annotation
// tag calendar_get_events.
const AnnotationHashCode = "86429515"

// TestCase defines test expectations based on the policy value.
type TestCase struct {
	Name                    string
	ShouldFindEventListView bool
	ShouldFindManagedIcon   bool
	ShouldFindAnnotation    bool
	Policy                  *policy.CalendarIntegrationEnabled
}

// TestCases returns the map of policy setting enum to TestCase
// object on which CalendarIntegrationEnabled policy is tested.
func TestCases() map[networkrequestmonitor.PolicySetting]TestCase {
	return map[networkrequestmonitor.PolicySetting]TestCase{
		networkrequestmonitor.PolicyDisabled: {
			Name:                    "disabled",
			ShouldFindEventListView: false,
			ShouldFindManagedIcon:   true,
			ShouldFindAnnotation:    false,
			Policy:                  &policy.CalendarIntegrationEnabled{Val: false},
		},
		networkrequestmonitor.PolicyEnabled: {
			Name:                    "enabled",
			ShouldFindEventListView: true,
			ShouldFindManagedIcon:   false,
			ShouldFindAnnotation:    true,
			Policy:                  &policy.CalendarIntegrationEnabled{Val: true},
		},
		networkrequestmonitor.PolicyUnset: {
			Name:                    "unset",
			ShouldFindEventListView: true,
			ShouldFindManagedIcon:   false,
			ShouldFindAnnotation:    true,
			Policy:                  &policy.CalendarIntegrationEnabled{Stat: policy.StatusUnset},
		},
	}
}

// TriggerCalendarIntegration verifies calendar integration process works as
// expected. i.e. the existence of the managed icon and event list view
// depending on the policy value.
func TriggerCalendarIntegration(ctx context.Context, _ *chrome.Chrome, br *browser.Browser, _ *httptest.Server, tconn *chrome.TestConn, policySetting networkrequestmonitor.PolicySetting) (err error) {
	param := TestCases()[policySetting]

	ui := uiauto.New(tconn)
	dateTray := nodewith.HasClass("DateTray")

	// Comparing the time before and after opening the calendar view just in case
	// this test is run at the very end of a year, e.g. Dec 31 23:59:59.
	beforeOpeningCalendarYear := time.Now().Year()

	if err := ui.DoDefault(dateTray)(ctx); err != nil {
		return errors.Wrap(err, "failed to click the date tray")
	}

	calendarView := nodewith.HasClass("CalendarView")
	mainHeaderTriView := nodewith.HasClass("TriView").Ancestor(calendarView).Nth(0)
	mainHeaderContainer := nodewith.HasClass("View").Ancestor(mainHeaderTriView).Nth(1)
	mainHeader := nodewith.Name("Calendar").HasClass("Label").Ancestor(mainHeaderContainer)

	if err := ui.WaitUntilExists(mainHeader)(ctx); err != nil {
		return errors.Wrap(err, "failed to find calendar main label after opening calendar view")
	}

	// For some corner cases, if it cannot find the year label with the time
	// before clicking on the date tray, it should find the year label with the
	// time after the calendar view is open. E.g. before opening it's
	//Dec 31 23:59:59 2022, and after openting it's Jan 1 00:00 2023.
	yearInt := beforeOpeningCalendarYear
	beforeOpeningCalendarYearLabel := nodewith.Name(strconv.Itoa(beforeOpeningCalendarYear)).HasClass("Label").Onscreen()
	if found, err := ui.IsNodeFound(ctx, beforeOpeningCalendarYearLabel); err != nil {
		return errors.Wrap(err, "failed to check beforeOpeningCalendarYearLabel after clicking on the date tray")
	} else if found != true {
		yearInt = time.Now().Year()
	}

	// Opening the calendar view should show today's year label.
	year := strconv.Itoa(yearInt)
	todayYearLabel := nodewith.Name(year).HasClass("Label").Onscreen()
	if err := ui.WaitUntilExists(todayYearLabel)(ctx); err != nil {
		return errors.Wrap(err, "failed to find year label after opening calendar view")
	}

	// Clicks on a Monday's cell to show the event list view.
	scrollView := nodewith.HasClass("ScrollView").Ancestor(calendarView).Nth(0)
	scrollViewport := nodewith.HasClass("ScrollView::Viewport").Ancestor(scrollView).Nth(0)
	contentView := nodewith.HasClass("View").Ancestor(scrollViewport).Nth(0)
	currentMonthView := nodewith.HasClass("View").Ancestor(contentView).Nth(3)
	firstMondayDateCell := nodewith.HasClass("CalendarDateCellView").Ancestor(currentMonthView).Nth(1)
	scrollViewBounds, err := ui.Location(ctx, scrollView)
	if err != nil {
		return errors.Wrap(err, "failed to find calendar scroll view bounds")
	}
	firstMondayDateCellBounds, err := ui.Location(ctx, firstMondayDateCell)
	if err != nil {
		return errors.Wrap(err, "failed to find calendar first monday cell bounds")
	}

	// TODO(b/234673735): Should click on the finder directly after this bug is
	// fixed.
	// Currently the vertical location of the cells fetched from |ui.Location| are
	// not correct. It returns the same number for the |Top| of all the cells,
	// which is the same number as the |Top| of the scroll view. This might
	// because of the scroll view is nested in some other views. Here a small
	// amount (5) of pixel is added to the top of the scroll view each time in the
	// loop to find the first available Monday cell with events.
	const findCellTimes = 20
	didFindEventListView := false
	cellPositionY := 0
	eventListView := nodewith.HasClass("CalendarEventListView").Ancestor(calendarView)
	eventCloseButtonViewContainer := nodewith.HasClass("View").Ancestor(eventListView).Nth(0)
	eventCloseButtonView := nodewith.HasClass("IconButton").Ancestor(eventCloseButtonViewContainer).Nth(0)
	for i := 0; i < findCellTimes; i++ {
		testing.ContextLogf(ctx, "Moving towards the first Monday cell (iteration %d of %d)", i+1, findCellTimes)
		cellPositionY += 5
		firstMondayDateCellPt := coords.NewPoint(firstMondayDateCellBounds.CenterX(), scrollViewBounds.Top+cellPositionY)
		if err := mouse.Click(tconn, firstMondayDateCellPt, mouse.LeftButton)(ctx); err != nil {
			return errors.Wrap(err, "failed to click the first monday date cell")
		}
		if found, err := ui.IsNodeFound(ctx, eventCloseButtonView); err != nil {
			return errors.Wrap(err, "failed to check event list view close button while finding the first monday cell")
		} else if found == true {
			didFindEventListView = true
			break
		}
	}

	didFindManagedIcon := false
	rightHeaderContainer := nodewith.HasClass("View").Ancestor(mainHeaderTriView).Nth(2)
	managedIcon := nodewith.Name("Disabled by admin").HasClass("IconButton").Ancestor(rightHeaderContainer)
	if found, err := ui.IsNodeFound(ctx, managedIcon); err != nil {
		return errors.Wrap(err, "failed to check for managed icon in calendar tray")
	} else if found == true {
		didFindManagedIcon = true
	}

	// Check for enterprise management icon.
	if param.ShouldFindManagedIcon && didFindManagedIcon == false {
		return errors.New("did not find expected disabled by admin icon")
	}

	if !param.ShouldFindManagedIcon && didFindManagedIcon == true {
		return errors.New("found unexpected disabled by admin icon")
	}

	// Check for event list.
	if param.ShouldFindEventListView && didFindEventListView == false {
		return errors.New("did not find expected event list view")
	}

	if !param.ShouldFindEventListView && didFindEventListView == true {
		return errors.New("found unexpected event list view")
	}

	// Click it again to close the calendar view.
	if err := ui.DoDefault(dateTray)(ctx); err != nil {
		return errors.New("failed to click the date tray")
	}

	return nil
}
