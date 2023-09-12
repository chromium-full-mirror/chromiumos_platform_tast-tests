// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strings"
	"time"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/missiveutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/syslog"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingMissiveFeatureFlagsDbus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that missive executes a dBus call to the Chrome feature API without error",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"jrhilke@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMS,
		Timeout:      2 * time.Minute,
	})
}

func ReportingMissiveFeatureFlagsDbus(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	missiveDbusAddress, err := missiveutil.GetMissiveDbusAddress(ctx)
	if err != nil {
		s.Fatal("Failed to get missive's dbus address: ", err)
	}

	// Matcher that looks for dbus calls from missive to the GetFeatureParams() function of the Chrome feature API.
	m := []dbusutil.MatchSpec{
		{
			Type:      "method_call",
			Path:      dbus.ObjectPath("/org/chromium/ChromeFeaturesService"),
			Interface: "org.chromium.ChromeFeaturesServiceInterface",
			Member:    "GetFeatureParams",
			Sender:    missiveDbusAddress,
		},
	}

	// When mon() is called, the dbus event monitor will stop looking for events and determine whether it has seen any calls that match the matcher `m`.
	mon, err := dbusutil.DbusEventMonitor(ctx, m)
	if err != nil {
		s.Fatal("Connection to missive dbus monitoring error: ", err)
	}

	// Create a new log reader.
	logReader, err := syslog.NewReader(ctx, syslog.SourcePath(syslog.MessageFile))
	if err != nil {
		s.Fatal("Failed to create log reader: ", err)
	}
	_, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())

	if err != nil {
		s.Fatal("Failed to create chrome instance: ", err)
	}

	missiveDBusCallsToFeatureAPI, err := mon()
	if err != nil {
		s.Fatal("Failed to capture dbus calls from Missive to the feature API: ", err)
	}

	// Verify Missive called the feature API.
	if len(missiveDBusCallsToFeatureAPI) == 0 {
		s.Fatal("No events found")
	}

	// If an error occurred during the dBus call, it will appear in the logs.
	containsMessage := func(e *syslog.Entry) bool {
		// Verify:
		// 1. Missive is the function caller
		// 2. The function call failed
		// 2. Feature API is the function callee
		// 3. DBus error is directly related to retrieving features
		return strings.Contains(e.Program, "missived") &&
			strings.Contains(e.Content, "Failed to call method") &&
			strings.Contains(e.Content, "org.chromium.ChromeFeaturesServiceInterface.GetFeatureParams") &&
			strings.Contains(e.Content, "org.freedesktop.DBus.Error.InvalidArgs")
	}

	// Check the log for the relevant error message. Fail if we find it.
	if msg, err := logReader.Wait(ctx, 10*time.Second, containsMessage); err == nil {
		s.Fatalf("Found a missive dbus error message in %s, with error: %v", syslog.MessageFile, msg)
	}

}
