// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"time"

	"github.com/golang/protobuf/proto"
	"golang.org/x/sys/unix"

	"go.chromium.org/chromiumos/reporting"
	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/common/policy/reportingutil"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/crash"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingUploadedCrashEvents,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify uploaded fatal crash events are reported when policy ReportDeviceCrashReportInfo is on",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test maintainer
		},
		Timeout:      6 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:enterprise-reporting", "group:hw_agnostic"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceCrashReportInfo{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

// setUpUploadedCrash sets up crash utilities and generates the uploaded crash.
func setUpUploadedCrash(ctx context.Context, s *testing.State) {
	testing.ContextLog(ctx, "Setting up the uploaded crash")
	// Reserve 5 seconds for cleaning up crash tests.
	ctxForCleanUpCrashSetup := ctx
	ctx, cancelForCleanUpCrashSetup := ctxutil.Shorten(ctx, 5*time.Second)
	defer cancelForCleanUpCrashSetup()

	// This will ensure no pre-existing crashes are in /var/spool/crash etc.
	// It will ensure that no other instance of crash_sender is running. It
	// will also help set up consent.
	//
	// It will also kill existing crash_sender processes. This isn't a
	// problem for the test because there is no subscriber at the beginning
	// of this test and healthD doesn't invoke crash_sender --dry_run in
	// this case.
	if err := crash.SetUpCrashTest(ctx, crash.WithMockConsent()); err != nil {
		s.Fatal("Could not set up crash test: ", err)
	}
	defer func() {
		if err := crash.TearDownCrashTest(ctxForCleanUpCrashSetup); err != nil {
			testing.ContextLog(ctx, "Failed to tear down crash test: ", err)
		}
	}()
	// Don't actually upload crashes.
	if err := crash.EnableMockSending(true); err != nil {
		s.Fatal("Could not enable mock sending: ", err)
	}

	// Trigger unuploaded crash event: Run the sleep command and crash it.
	sleepCmd := testexec.CommandContext(ctx, "sleep", "100")
	if err := sleepCmd.Start(); err != nil {
		s.Fatal("Failed to start the sleep command: ", err)
	}
	if err := sleepCmd.Signal(unix.SIGSEGV); err != nil {
		s.Fatal("Failed to crash the sleep command: ", err)
	}
	err := sleepCmd.Wait()
	waitStatus, ok := testexec.GetWaitStatus(err)
	if !ok {
		s.Fatal("Failed to get sleep's wait status: ", err)
	}
	if !waitStatus.Signaled() || waitStatus.Signal() != unix.SIGSEGV {
		s.Fatal("Failed to crash sleep: ", err)
	}
	// When a crash occurs, crash_reporter is responsible for writing crash
	// meta files. However, occasionally `cros-health-tool` already has
	// started subscribing while the *.meta file hasn't been created yet.
	// Wait for crash files to be ready.
	crashDirs := crash.GetAllCrashDirs(ctx)
	if crashFiles, err := crash.WaitForCrashFiles(ctx, crashDirs, []string{`coreutils\.[\d\.]+\.meta`}); err != nil {
		s.Fatalf("Failed to wait for crash files from %v: %s", crashDirs, err)
	} else {
		for _, files := range crashFiles {
			for _, file := range files {
				testing.ContextLog(ctx, "Crash meta file corresponding to the test unuploaded crash: ", file)
				testing.ContextLog(ctx, "Replacing its crash type with kernel")
				if err := reportingutil.ReplaceCrashType(file); err != nil {
					s.Fatal("Failed to replace crash type in ", file, err)
				}
			}
		}
	}

	// Convert the unuploaded crash event to uploaded crash event.
	testing.ContextLog(ctx, "Starting crash_sender")
	if _, err := crash.RunSender(ctx); err != nil {
		s.Fatal("Failed to run crash_sender: ", err)
	}
}

func ReportingUploadedCrashEvents(ctx context.Context, s *testing.State) {
	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}
	// Set buffer size to 100 to leave room for additional crash events that
	// are not expected by this test.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 100)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}
	currentTimestampUs := time.Now().UnixNano() / 1000
	server.SetFilter(func(wr *reporting.WrappedRecord) bool {
		if *wr.Record.Destination != reporting.Destination_CRASH_EVENTS {
			return false
		}
		var m reporting.MetricData
		if err := proto.Unmarshal(wr.Record.Data, &m); err != nil {
			s.Error("Could not parse record: ", err)
		}

		if m.GetTelemetryData() == nil {
			return false
		}

		fatalCrashTelemetry := m.GetTelemetryData().FatalCrashTelemetry
		if fatalCrashTelemetry == nil {
			return false
		}

		// The crash event must be sufficiently new and have a crash
		// report ID, i.e., uploaded.
		return fatalCrashTelemetry.GetTimestampUs() > currentTimestampUs && len(fatalCrashTelemetry.GetCrashReportId()) > 0

	})
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Update affiliation and policies.
	policies := []policy.Policy{
		&policy.ReportDeviceCrashReportInfo{Val: true},
	}
	pb := policy.NewBlob()
	affiliationID := []string{"affiliation_id"}
	pb.DeviceAffiliationIds = affiliationID
	pb.UserAffiliationIds = affiliationID
	pb.AddPolicies(policies)
	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Could not apply policy: ", err)
	}

	// Start a Chrome instance that will fetch policies from the FakeDMS.
	_, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures(erpserver.MissiveDefaultFeature),
		chrome.EnableFeatures("EnableFatalCrashEventsObserver"),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	setUpUploadedCrash(ctx, s)

	// Restart healthd so that it will immediately retrieve and emit the
	// newly generated crash.
	if err := upstart.RestartJob(ctx, "cros_healthd"); err != nil {
		s.Fatal("Could not restart cros_healthd")
	}

	testing.ContextLog(ctx, "Waiting for the crash record")
	record := server.NextRecord()
	if record == nil {
		s.Fatal("Record is nil")
	}

	// Succeed, because a record that passes the filter is found.
}
