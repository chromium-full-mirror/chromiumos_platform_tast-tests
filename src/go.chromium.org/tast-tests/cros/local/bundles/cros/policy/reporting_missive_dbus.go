// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"github.com/golang/protobuf/proto"
	rep "go.chromium.org/chromiumos/reporting"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingMissiveDbus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that events from the missive_testing_tool are being sent to missive via dbus",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:criticalstaging", "group:mainline", "informational", "group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:enterprise-reporting", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMS,
		Timeout:      2 * time.Minute,
	})
}

func ReportingMissiveDbus(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Start monitoring dbus.
	m := []dbusutil.MatchSpec{{Type: "method_call",
		Path:      dbus.ObjectPath("/org/chromium/Missived"),
		Interface: "org.chromium.Missived",
		Member:    "EnqueueRecord"}}

	mon, err := dbusutil.DbusEventMonitor(ctx, m)
	if err != nil {
		s.Fatal("Connection to missive dbus monitoring error: ", err)
	}

	_, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EnableFeatures(""),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Failed to create chrome instance: ", err)
	}

	// Run action to enqueue an event.
	if err := testexec.CommandContext(ctx, "sudo", "-u", "chronos", "/usr/local/bin/missive_testing_tool", "--enqueue='Sample data'", "--priority=SECURITY", "--destination=SUSPICIOUS_EVENTS").Run(); err != nil {
		s.Fatal("Failed to run missive_testing_tool: ", err)
	}

	enqueuedEvents, err := mon()
	if err != nil {
		s.Fatal("Failed to capture dbus calls to missive: ", err)
	}

	if len(enqueuedEvents) == 0 {
		s.Fatal("No events found")
	}

	// Verify that the events are not malformed and that data is correct.
	for _, event := range enqueuedEvents {
		if len(event.Arguments) == 0 {
			s.Fatal("Event has no arguments")
		}
		arg, ok := event.Arguments[0].([]byte)
		if !ok {
			s.Fatal("Failed to cast arguments")
		}
		enq := &rep.EnqueueRecordRequest{}
		if err := proto.Unmarshal(arg, enq); err != nil {
			s.Fatal("Failed to unmarshal record request")
		}

		if string(enq.GetRecord().GetData()) == "'Sample data'" {
			testing.ContextLog(ctx,
				"Reporting: Matching record found: ", enq)
			return
		}
	}
	// If no matching event is found, report an error.
	s.Fatal("Reporting: No matching event found, all events: ", enqueuedEvents)
}
