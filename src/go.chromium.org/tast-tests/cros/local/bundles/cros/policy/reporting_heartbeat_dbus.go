// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"

	rep "chromiumos/reporting"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"

	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingHeartbeatDbus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that heartbeat events are being sent to missive when the flag is enabled",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMS,
		Timeout:      2 * time.Minute,
	})
}

func ReportingHeartbeatDbus(ctx context.Context, s *testing.State) {
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
		chrome.EnableFeatures("EncryptedReportingManualTestHeartbeatEvent"),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Failed to create chrome instance: ", err)
	}

	testing.ContextLog(ctx,
		"Sleeping for 15 secs")
	// GoBigSleepLint: Sleep for 15 seconds to allow time for events to be
	// generated and reported.
	// TODO(b/278252387): Convert this to poll when tast's
	// dbusutil.DbusEventMonitor supports it.
	if err := testing.Sleep(ctx, 15*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	enqueuedEvents, err := mon()
	if err != nil {
		s.Fatal("Failed to capture dbus calls to missive: ", err)
	}

	if len(enqueuedEvents) == 0 {
		s.Fatal("No events found")
	}

	// Verify that the events are not malformed and that the destination is correct.
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
		if enq.GetRecord().GetDestination() != rep.Destination_HEARTBEAT_EVENTS {
			s.Fatal("Destination mismatch, got ", enq.GetRecord().GetDestination(), " wanted HEARTBEAT_EVENTS")
		}
	}
}
