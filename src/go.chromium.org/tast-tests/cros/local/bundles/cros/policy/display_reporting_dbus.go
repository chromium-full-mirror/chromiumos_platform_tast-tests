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
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/policyutil"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing/hwdep"
)

type displayTestParam struct {
	internalDisplay bool
	externalDisplay bool
	privacyScreen   bool
	policyEnabled   bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         DisplayReportingDbus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the display information is being reported as expected",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      2 * time.Minute,
		Params: []testing.Param{
			{
				Name:              "external_display_disabled",
				ExtraHardwareDeps: hwdep.D(hwdep.ExternalDisplay(), hwdep.NoInternalDisplay()),
				Val: displayTestParam{
					internalDisplay: false,
					externalDisplay: true,
					privacyScreen:   false,
					policyEnabled:   false,
				},
			},
			{
				Name:              "external_display_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.ExternalDisplay(), hwdep.NoInternalDisplay()),
				Val: displayTestParam{
					internalDisplay: false,
					externalDisplay: true,
					privacyScreen:   false,
					policyEnabled:   true,
				},
			},
			{
				Name:              "internal_display_disabled",
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.NoPrivacyScreen()),
				Val: displayTestParam{
					internalDisplay: true,
					externalDisplay: false,
					privacyScreen:   false,
					policyEnabled:   false,
				},
			},
			{
				Name:              "internal_display_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.NoPrivacyScreen()),
				Val: displayTestParam{
					internalDisplay: true,
					externalDisplay: false,
					privacyScreen:   false,
					policyEnabled:   true,
				},
			},
			{
				Name:              "internal_display_privacy_screen_disabled",
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.PrivacyScreen()),
				Val: displayTestParam{
					internalDisplay: true,
					externalDisplay: false,
					privacyScreen:   true,
					policyEnabled:   false,
				},
			},
			{
				Name:              "internal_display_privacy_screen_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.PrivacyScreen()),
				Val: displayTestParam{
					internalDisplay: true,
					externalDisplay: false,
					privacyScreen:   true,
					policyEnabled:   true,
				},
			},
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceNetworkStatus{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceNetworkConfiguration{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceBootMode{}, pci.Served),
			pci.SearchFlag(&policy.ReportDeviceGraphicsStatus{}, pci.VerifiedFunctionalityOS),
		},
	})
}

func DisplayReportingDbus(ctx context.Context, s *testing.State) {
	internalDisplay := s.Param().(displayTestParam).internalDisplay
	externalDisplay := s.Param().(displayTestParam).externalDisplay
	privacyScreen := s.Param().(displayTestParam).privacyScreen
	policyEnabled := s.Param().(displayTestParam).policyEnabled
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
		&policy.ReportDeviceGraphicsStatus{Val: policyEnabled},
		// Setting this to false so they don't report the events corresponding to the policies.
		&policy.ReportDeviceNetworkStatus{Val: false},
		&policy.ReportDeviceNetworkConfiguration{Val: false},
		&policy.ReportDeviceBootMode{Val: false},
	}

	pb := policy.NewBlob()
	pb.AddPolicies(policies)

	if err := policyutil.ServeBlobAndRefresh(ctx, fdms, cr, pb); err != nil {
		s.Fatal("Failed to serve policies: ", err)
	}

	if err := policyutil.Verify(ctx, tconn, policies); err != nil {
		s.Fatal("Failed to verify policies: ", err)
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

	for _, method := range enqueuedEvents {
		arg, ok := method.Arguments[0].([]byte)
		if !ok {
			s.Fatal("Failed to cast arguments")
		}
		enq := &rep.EnqueueRecordRequest{}
		if err := proto.Unmarshal(arg, enq); err != nil {
			s.Fatal("Failed to unmarshal an EnqueueRecordRequest")
		}

		metricData := &rep.MetricData{}
		if err := proto.Unmarshal(enq.GetRecord().GetData(), metricData); err != nil {
			s.Fatal("Failed to unmarshal data for a CROS_SECURITY_AGENT record")
		}
	}
	// Verify that nothing was sent if the policy is disabled.
	if !policyEnabled && len(enqueuedEvents) > 0 {
		s.Fatal("Events found when policy disabled")
	}

	// Verify that the information was sent and that it is not malformed.
	if policyEnabled {
		if len(enqueuedEvents) == 0 {
			s.Fatal("No events found when policy enabled")
		}
		if len(enqueuedEvents) > 1 {
			s.Fatal("More than one event reported when policy enabled. Full data: ", enqueuedEvents)
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
		if enq.GetRecord().GetDestination() != rep.Destination_INFO_METRIC {
			s.Fatal("Destination mismatch, got ", enq.GetRecord().GetDestination(), " wanted INFO_METRIC")
		}

		metricData := &rep.MetricData{}
		if err := proto.Unmarshal(enq.GetRecord().GetData(), metricData); err != nil {
			s.Fatal("Failed to unmarshal data for the event")
		}

		if metricData.GetInfoData() == nil {
			s.Fatal("No info data found on the event")
		}
		// Privacy screen info.
		if metricData.GetInfoData().GetPrivacyScreenInfo() == nil {
			s.Fatal("No privacy screen info found on the info data")
		}
		privacyInfo := metricData.GetInfoData().GetPrivacyScreenInfo()
		if privacyInfo.GetSupported() != privacyScreen {
			s.Fatal("Privacy screen mismatch")
		}

		// Display info.
		if metricData.GetInfoData().GetDisplayInfo() == nil {
			s.Fatal("No display info found on the info data")
		}
		if metricData.GetInfoData().GetDisplayInfo().GetDisplayDevice() == nil {
			s.Fatal("No display device(s) found on the info data")
		}
		displayDevices := metricData.GetInfoData().GetDisplayInfo().GetDisplayDevice()

		if internalDisplay && externalDisplay {
			if len(displayDevices) < 2 {
				s.Fatal("Was expecting 2 display devices reported and got ", len(displayDevices), ". Full data: ", displayDevices)
			}
		} else if internalDisplay || externalDisplay {
			if len(displayDevices) > 1 {
				s.Fatal("Was expecting one display device reported and got ", len(displayDevices), ". Full data: ", displayDevices)
			}
			if internalDisplay && !displayDevices[0].GetIsInternal() {
				s.Fatal("Display info reporting internal display as external display")
			}
			if externalDisplay && displayDevices[0].GetIsInternal() {
				s.Fatal("Display info reporting external display as internal display")
			}
		}
	}
}
