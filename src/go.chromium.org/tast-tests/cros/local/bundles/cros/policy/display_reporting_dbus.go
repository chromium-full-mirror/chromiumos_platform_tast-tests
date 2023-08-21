// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

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
	policyEnabled bool
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
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      2 * time.Minute,
		HardwareDeps: hwdep.D(hwdep.InternalDisplay(), hwdep.TouchScreen()),
		Params: []testing.Param{
			{
				Name: "enabled",
				Val: displayTestParam{
					policyEnabled: true,
				},
			},
			{
				Name: "disabled",
				Val: displayTestParam{
					policyEnabled: false,
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
	policyEnabled := s.Param().(displayTestParam).policyEnabled
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Start monitoring dbus events.
	ew, err := dbusutil.NewEventWatcher(ctx, []dbusutil.MatchSpec{
		{
			Interface: "org.chromium.Missived",
			Member:    "EnqueueRecord",
		},
	})
	if err != nil {
		s.Fatal("Cannot create EventWatcher: ", err)
	}
	defer ew.Close()

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

	findEventTimeout := 60 * time.Second

	testing.ContextLog(ctx, "Waiting for events")
	if policyEnabled {
		// Only expecting two events:
		// - Touch screen data
		// - Normal display data
		for i := 0; i < 2; i++ {
			select {
			case ev, ok := <-ew.Events():
				if !ok {
					s.Fatal("Got error while trying to query events")
				}
				arg, ok := ev.Arguments[0].([]byte)
				if !ok {
					s.Fatal("Failed to cast arguments")
				}
				enq := &rep.EnqueueRecordRequest{}
				if err := proto.Unmarshal(arg, enq); err != nil {
					s.Fatal("Failed to unmarshal an EnqueueRecordRequest with error: ", err)
				}
				if enq.GetRecord().GetDestination() != rep.Destination_INFO_METRIC {
					s.Fatal("Destination mismatch, got ", enq.GetRecord().GetDestination(), " wanted INFO_METRIC")
				}

				metricData := &rep.MetricData{}
				if err := proto.Unmarshal(enq.GetRecord().GetData(), metricData); err != nil {
					s.Fatal("Failed to unmarshal data with error: ", err)
				}
				if metricData.GetInfoData() == nil {
					s.Fatal("No info data found on the event: ", metricData)
				}
				if metricData.GetInfoData().GetDisplayInfo() == nil && metricData.GetInfoData().GetTouchScreenInfo() == nil {
					s.Fatal("No display info or touch screen info found on the info data, event: ", metricData)
				}
			case <-time.After(findEventTimeout):
				s.Fatal("findEventTimeout passed and no event found with policy enabled")
			}
		}
	} else {
		// Not expecting any events.
		select {
		case ev, _ := <-ew.Events():
			s.Fatal("Found event with policy disabled, event: ", ev)
		case <-time.After(findEventTimeout):
			testing.ContextLog(ctx, "findEventTimeout passed and no event found")
		}
	}
}
