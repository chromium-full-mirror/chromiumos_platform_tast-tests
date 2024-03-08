// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/godbus/dbus/v5"
	"google.golang.org/protobuf/proto"

	rep "go.chromium.org/chromiumos/reporting"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

type bootReportingParameters struct {
	reportingEnabled bool // test should expect reporting enabled
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         BootReportingDbus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the boot reporting telemetry is being populated and sent correctly",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome", "vpd"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      5 * time.Minute,
		Params: []testing.Param{
			{
				Name: "enabled",
				Val: bootReportingParameters{
					reportingEnabled: true,
				},
			}, {
				Name: "disabled",
				Val: bootReportingParameters{
					reportingEnabled: false,
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceNetworkStatus{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceNetworkConfiguration{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceBootMode{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func BootReportingDbus(ctx context.Context, s *testing.State) {
	param := s.Param().(bootReportingParameters)
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Start monitoring dbus calls.
	m := []dbusutil.MatchSpec{{Type: "method_call",
		Path:      dbus.ObjectPath("/org/chromium/Missived"),
		Interface: "org.chromium.Missived",
		Member:    "EnqueueRecord"}}

	eventMonitor, err := dbusutil.DbusEventMonitor(ctx, m)
	if err != nil {
		s.Fatal("Connection to missive dbus monitoring error: ", err)
	}

	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Failed to create chrome instance: ", err)
	}

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create Test API connection: ", err)
	}

	policies := []policy.Policy{
		// Set policy depending on the parameter of the test.
		&policy.ReportDeviceBootMode{Val: param.reportingEnabled},
		// Setting this to false so they don't report the events corresponding to the policies.
		&policy.ReportDeviceNetworkStatus{Val: false},
		&policy.ReportDeviceNetworkConfiguration{Val: false},
	}

	pb := policy.NewBlob()
	pb.AddPolicies(policies)

	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	if err := policyutil.Verify(ctx, tconn, policies); err != nil {
		s.Fatal("Failed to verify ArcEnabled: ", err)
	}

	testing.ContextLog(ctx,
		"Sleeping for 60 secs")
	// GoBigSleepLint: Sleep for 60 seconds to make sure that the telemetry is reported.
	// TODO(b/278252387): Convert this to poll when tast's
	// dbusutil.DbusEventMonitor supports it.
	if err := testing.Sleep(ctx, time.Minute); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	enqueuedEvents, err := eventMonitor()
	if err != nil {
		s.Fatal("Failed to capture dbus calls to missive: ", err)
	}

	// Verify that nothing was sent if the policy is disabled.
	if !param.reportingEnabled && len(enqueuedEvents) > 0 {
		s.Fatal("Events found when policy disabled")
	}

	// Verify that the information was sent and that it is not malformed.
	if param.reportingEnabled {
		if len(enqueuedEvents) == 0 {
			s.Fatal("No events found when policy enabled")
		}
		if len(enqueuedEvents) > 1 {
			s.Fatal("More than one event reported when policy enabled")
		}

		if len(enqueuedEvents[0].Arguments) == 0 {
			s.Fatal("Event has no arguments")
		}
		arg, ok := enqueuedEvents[0].Arguments[0].([]byte)
		if !ok {
			s.Fatal("Failed to cast arguments")
		}
		enq := &rep.EnqueueRecordRequest{}
		if err := proto.Unmarshal(arg, enq); err != nil {
			s.Fatal("Failed to unmarshal record request")
		}
		if enq.GetRecord().GetDestination() != rep.Destination_TELEMETRY_METRIC {
			s.Fatal("Destination mismatch, got ", enq.GetRecord().GetDestination(), " wanted TELEMETRY_METRIC")
		}

		md := &rep.MetricData{}
		if err := proto.Unmarshal(enq.GetRecord().GetData(), md); err != nil {
			s.Fatal("Failed to unmarshal data for the event")
		}

		if md.GetTelemetryData() == nil {
			s.Fatal("No telemetry data found on the event")
		}
		if md.GetTelemetryData().GetBootPerformanceTelemetry() == nil {
			s.Fatal("No boot performance telemetry data found on the event")
		}
		bpt := md.GetTelemetryData().GetBootPerformanceTelemetry()
		if bpt.GetBootUpSeconds() < 0 || bpt.GetBootUpSeconds() > 3600 {
			s.Fatal("Boot up seconds field data corrupt, got: ", bpt.GetBootUpSeconds())
		}

		if bpt.GetBootUpTimestampSeconds() > md.GetTimestampMs() {
			s.Fatal("Boot up timestamp seconds data corrupt, got: ", bpt.GetBootUpTimestampSeconds())
		}
	}
}
