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
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"

	"chromiumos/reporting"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingChromeRestart,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies that missive and enqueue work after chrome shuts down and restarts, kill chrome, check missive, start chrome, check missive, enqueue",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		VarDeps:      []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
		Fixture:      fixture.FakeDMSEnrolled,
	})
}

func ReportingChromeRestart(ctx context.Context, s *testing.State) {
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

	chromeOptions := []chrome.Option{chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures("CrOSLateBootMissiveStorage:signature_verification_dev_enabled/true/compression_enabled/false"),
		chrome.KeepEnrollment()}

	// Start a Chrome instance for the first time.
	cr, err := chrome.New(ctx, chromeOptions...)
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	// Make sure that missive is running.
	if err := upstart.EnsureJobRunning(ctx, "missived"); err != nil {
		s.Fatal("Missive not running before killing chrome")
	}

	// Close the chrome instance and make sure that missive is still running.
	cr.Close(ctx)
	if err := upstart.EnsureJobRunning(ctx, "missived"); err != nil {
		s.Fatal("Missive not running after killing chrome")
	}

	// Start chrome instance again with heartbeat events enabled.
	chromeOptionsWithFlag := append(chromeOptions, chrome.EnableFeatures("EncryptedReportingManualTestHeartbeatEvent"))
	_, err = chrome.New(ctx, chromeOptionsWithFlag...)
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	// Verify that missive is running.
	if err := upstart.EnsureJobRunning(ctx, "missived"); err != nil {
		s.Fatal("Missive not running after starting chrome again")
	}

	// Make sure that all 10 records are formed correctly.
	// Create a channel to communicate between the goroutines.
	ch := make(chan *reporting.WrappedRecord)

	// Start 10 goroutines to fetch records.
	for i := 0; i < 10; i++ {
		go func() {
			record, err := server.NextRecordAsync(2*time.Minute, true /*expectEvents*/)
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
