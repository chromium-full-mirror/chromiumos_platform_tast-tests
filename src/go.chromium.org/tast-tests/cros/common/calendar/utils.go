// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package calendar

import "go.chromium.org/tast/core/testing"

// GoogleCalendarAccountPoolVarName is the calendar google calendar account pool name.
const GoogleCalendarAccountPoolVarName = "calendar.googleCalendarAccountPool"

// UpcomingEventsAccountVarName is the calendar upcoming events account pool name.
const UpcomingEventsAccountVarName = "calendar.upcomingEventsAccountPool"

const googleCalendarDMAAccountPoolVarName = "calendar.googleCalendarDMAAccountPool"

const upcomingEventsDMAAccountVarName = "calendar.upcomingEventsDMAAccountPool"

var googleCalendarAccountPoolVar = testing.RegisterVarString(
	GoogleCalendarAccountPoolVarName,
	"",
	"It contains creds in calendar.googleCalendarAccountPool",
)

var googleCalendarDMAAccountPoolVar = testing.RegisterVarString(
	googleCalendarDMAAccountPoolVarName,
	"",
	"It contains creds in calendar.googleCalendarDMAAccountPool",
)

var upcomingEventsAccountVar = testing.RegisterVarString(
	UpcomingEventsAccountVarName,
	"",
	"It contains creds in calendar.upcomingEventsAccountPool",
)

var upcomingEventsDMAAccountVar = testing.RegisterVarString(
	upcomingEventsDMAAccountVarName,
	"",
	"It contains creds in calendar.upcomingEventsDMAAccountPool",
)

// GoogleCalendarAccountPoolValue returns credentials from calendar.googleCalendarAccountPool.
func GoogleCalendarAccountPoolValue() string {
	return googleCalendarAccountPoolVar.Value()
}

// GoogleCalendarDMAAccountPoolValue returns credentials from calendar.googleCalendarDMAAccountPool.
func GoogleCalendarDMAAccountPoolValue() string {
	return googleCalendarDMAAccountPoolVar.Value()
}

// UpcomingEventsAccountValue returns credentials from calendar.upcomingEventsAccountPool.
func UpcomingEventsAccountValue() string {
	return upcomingEventsAccountVar.Value()
}

// UpcomingEventsDMAAccountValue returns credentials from calendar.upcomingEventsDMAAccountPool.
func UpcomingEventsDMAAccountValue() string {
	return upcomingEventsDMAAccountVar.Value()
}
