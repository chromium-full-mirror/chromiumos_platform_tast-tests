// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast/core/testing"

	"chromiumos/reporting"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingHeartbeatLocal,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify heartbeat events with flag enabled using the fake reporting service",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test author
		},
		Timeout:      3 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "group:criticalstaging", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		Fixture:      fixture.FakeDMSEnrolled,
		VarDeps:      []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

func ReportingHeartbeatLocal(ctx context.Context, s *testing.State) {
	missiveFeature := "CrOSLateBootMissiveStorage:signature_verification_dev_enabled/true/compression_enabled/false"
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}

	testStartTime := time.Now()

	// Set buffer size to 10 since only 10 records are expected in this test.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 10)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}

	// Setting filter to only return heartbeat events.
	server.SetFilter(func(wr *reporting.WrappedRecord) bool {
		return *wr.Record.Destination == reporting.Destination_HEARTBEAT_EVENTS
	})
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Start a Chrome instance with the Fake DM server.
	_, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures(missiveFeature, "EncryptedReportingManualTestHeartbeatEvent"),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	// Create a channel to communicate between the goroutines.
	ch := make(chan *reporting.WrappedRecord)

	// Start 10 goroutines to fetch records.
	for i := 0; i < 10; i++ {
		go func() {
			record, err := server.NextRecordAsync(1 * time.Minute)
			if err != nil {
				s.Fatal("Failed to wait for the record: ", err)
			}
			ch <- record
		}()
	}

	// Wait for all of the records to be fetched.
	for i := 0; i < 10; i++ {
		record := <-ch

		if record == nil {
			s.Errorf("Record %d is nil", i)
		}

		ts := *record.Record.TimestampUs
		if time.UnixMicro(ts).Before(testStartTime) {
			s.Errorf("Invalid timestamp, test start time: %s , record  %d timestamp: %d", testStartTime.String(), i, ts)
		}
	}
}
