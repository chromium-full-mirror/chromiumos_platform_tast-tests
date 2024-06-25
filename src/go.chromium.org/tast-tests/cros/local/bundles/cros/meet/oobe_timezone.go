// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meet

import (
	"context"
	"fmt"
	"strings"
	"time"

	_ "time/tzdata"
	_ "unsafe"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OobeTimezone,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test that verifies OOBE timezones accuracy, accounting for accurate offsets and unique timezone entries",
		Contacts: []string{
			"core-devices@google.com",
			"torikauffman@google.com", // Test author
			"egwuekwe@google.com",     // Test author
		},
		BugComponent: "b:543707", // Communications > Video (Meet) > Platforms > Rooms > Core Devices (OS & Hardware)
		Attr:         []string{"group:meet", "group:mainline", "informational", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "meet_device"},
		Timeout:      chrome.LoginTimeout + 45*time.Second,
	})
}

const secPerHour = 3600
const minPerHour = 60
const secPerMin = 60

// loadFromEmbeddedTZData comes from time/tzdata
//go:linkname loadFromEmbeddedTZData time/tzdata.loadFromEmbeddedTZData
func loadFromEmbeddedTZData(name string) (string, error)

func OobeTimezone(ctx context.Context, s *testing.State) {
	tags := []string{
		"login_display_host*=4",
		"oobe_ui=4",
	}

	opts := append([]chrome.Option{
		chrome.ExtraArgs("--enable-logging", "--vmodule="+strings.Join(tags, ","))},
		chrome.NoLogin())
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)

	conn, err := cr.WaitForOOBEConnection(ctx)
	if err != nil {
		s.Fatal("Failed to wait for OOBE connection: ", err)
	}
	defer conn.Close()

	if err := conn.WaitForExprFailOnErr(ctx, "OobeAPI.screens.WelcomeScreen.isVisible()"); err != nil {
		s.Fatal("Failed to wait for the Welcome screen to be visible: ", err)
	}

	var tzMap map[string]string
	var tzArr []string
	const f = `Array.from(
			document.querySelector("#connect")
			.shadowRoot.querySelector("#timezoneSelect")
			.shadowRoot.querySelector("#select").options)
		.map((e) => e.value + " || " + e.text)`

	if err := conn.Eval(ctx, f, &tzArr); err != nil {
		s.Fatal("Failed to get timezone options: ", err)
	}

	tzMap = make(map[string]string)
	t := time.Now()

	var errLog []string

	for _, tz := range tzArr {
		tzSlice := strings.Split(tz, " || ")
		loc := tzSlice[0]
		iOffset := tzSlice[1]
		if _, dup := tzMap[loc]; dup {
			errMsg := fmt.Sprintf("Duplicate time zone: %s.", loc)
			errLog = append(errLog, errMsg)
		}

		iOffset = strings.Split(strings.Split(iOffset, "UTC")[1], ")")[0]
		tzMap[loc] = iOffset

		zoneData, err := loadFromEmbeddedTZData(loc)
		if err != nil {
			s.Fatalf("Failed to get embedded data for location %s: %v", loc, err)
		}
		dbLoc, err := time.LoadLocationFromTZData(loc, []byte(zoneData))
		if err != nil {
			s.Fatalf("Failed to get time zone data for location %s: %v", loc, err)
		}

		_, dbOffsetSecs := t.In(dbLoc).Zone()

		dbOffsetHrs := dbOffsetSecs / secPerHour
		dbOffsetMins := (dbOffsetSecs % secPerHour) / secPerMin
		if dbOffsetMins < 0 {
			dbOffsetMins *= -1
		}

		dbOffset := fmt.Sprintf("%+d:%02d", dbOffsetHrs, dbOffsetMins)

		if iOffset != dbOffset {
			errMsg := fmt.Sprintf("Invalid offset: %s; expected UTC%s; found UTC%s.", loc, dbOffset, iOffset)
			errLog = append(errLog, errMsg)
		}
	}

	if len(errLog) > 0 {
		errMsg := strings.Join(errLog, "\n")
		s.Fatal("Test encountered the following errors: ", errMsg)
	}
}
