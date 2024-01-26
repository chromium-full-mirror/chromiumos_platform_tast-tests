// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"

	"go.chromium.org/chromiumos/reporting"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingUsbEvents,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that USB added events get sent to the reporting server",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test author
		},
		Timeout:      3 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome", "vpd"},
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDevicePeripherals{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

func ReportingUsbEvents(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}

	// Set buffer size to 5 since we expect at most 5 records.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 5)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}

	// Setting filter to only return events from the metrics reporting.
	server.SetFilter(func(wr *reporting.WrappedRecord) bool {
		if *wr.Record.Destination != reporting.Destination_PERIPHERAL_EVENTS {
			return false
		}
		var m reporting.MetricData
		if err := proto.Unmarshal(wr.Record.Data, &m); err != nil {
			s.Error("Could not parse telemetry metric record: ", err)
		}
		return m.GetEventData() != nil
	})
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Set policies and affiliattion.
	policies := []policy.Policy{
		&policy.ReportDevicePeripherals{Val: true},
	}
	pb := policy.NewBlob()
	affiliationID := []string{"affiliation_id"}
	pb.DeviceAffiliationIds = affiliationID
	pb.UserAffiliationIds = affiliationID
	pb.AddPolicies(policies)
	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Could not apply policy: ", err)
	}

	// Start a Chrome instance with the Fake DM server.
	_, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures(erpserver.MissiveDeaultFeature),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	// Trigger USB event.
	if err := testexec.CommandContext(ctx, "udevadm", "trigger", "-s", "usb", "-c", "add").Run(); err != nil {
		s.Fatal("Failed to trigger usb add event: ", err)
	}

	// Check that the events are USB added events as expected.
	// We only verify 2 events since the other ones are optional
	// and might not be reported on every board.
	// Create a channel to communicate between the goroutines.
	ch := make(chan *reporting.WrappedRecord)

	// Start 2 goroutines to fetch records.
	for i := 0; i < 2; i++ {
		go func() {
			record, err := server.NextRecordAsync(2*time.Minute, true /*expectEvents*/)
			if err != nil {
				s.Fatal("Failed to wait for the record: ", err)
			}
			ch <- record
		}()
	}

	// Wait for all of the records to be fetched.
	for i := 0; i < 2; i++ {
		record := <-ch
		if record == nil {
			s.Errorf("Record %d is nil", i)
		}
		var m reporting.MetricData
		if err := proto.Unmarshal(record.Record.Data, &m); err != nil {
			s.Errorf("Could not parse telemetry metric record %d: %s", i, err)
		}
		if m.GetEventData().GetType() != reporting.MetricEventType_USB_ADDED {
			s.Errorf("Event type not matched, got %s but wanted MetricEventType_USB_ADDED", m.GetEventData().GetType())
		}
	}
}
