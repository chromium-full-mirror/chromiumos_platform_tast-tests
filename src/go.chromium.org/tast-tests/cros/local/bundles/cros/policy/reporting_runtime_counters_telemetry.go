// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
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
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingRuntimeCountersTelemetry,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify runtime counters telemetry is reported when DeviceReportRuntimeCounters is on",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"xuhong@chromium.org", // Test author
		},
		Timeout:      6 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceReportRuntimeCounters{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.DeviceReportRuntimeCountersCheckingRateMs{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

type runtimeCountersResult struct {
	IsSupported   bool   `json:"is_supported"`
	LogState      string `json:"log_state"`
	UptimeSeconds string `json:"uptime_seconds"`
	S3Counter     string `json:"s3_counter"`
	S4Counter     string `json:"s4_counter"`
	S5Counter     string `json:"s5_counter"`
}

type systemResult struct {
	PsrInfo runtimeCountersResult `json:"psr_info"`
}

// verifyRuntimeCounters verifies that the runtime counters record meets the
// expectation.
func verifyRuntimeCounters(reportedRuntimeCounters *reporting.RuntimeCountersTelemetry, actualRuntimeCounters *runtimeCountersResult) error {
	uptimeSeconds, err := strconv.ParseUint(actualRuntimeCounters.UptimeSeconds, 10, 32)
	if err != nil {
		return errors.Wrap(err, "UptimeRuntimeSeconds is not an integer")
	}
	// Reporting happens a bit later than getting the result from healthd.
	// Allow some leeway for UptimeRuntimeSeconds.
	if reportedRuntimeCounters.GetUptimeRuntimeSeconds() < int64(uptimeSeconds) || reportedRuntimeCounters.GetUptimeRuntimeSeconds() > int64(uptimeSeconds)+300 {
		return errors.New("UptimeRuntimeSeconds does not match")
	}

	CounterEnterSleep, err := strconv.ParseUint(actualRuntimeCounters.S3Counter, 10, 32)
	if err != nil {
		return errors.Wrap(err, "CounterEnterSleep is not an integer")
	}
	if reportedRuntimeCounters.GetCounterEnterSleep() != int64(CounterEnterSleep) {
		return errors.New("CounterEnterSleep does not match")
	}

	CounterEnterHibernation, err := strconv.ParseUint(actualRuntimeCounters.S4Counter, 10, 32)
	if err != nil {
		return errors.Wrap(err, "CounterEnterHibernation is not an integer")
	}
	if reportedRuntimeCounters.GetCounterEnterHibernation() != int64(CounterEnterHibernation) {
		return errors.New("CounterEnterHibernation does not match")
	}

	CounterEnterPoweroff, err := strconv.ParseUint(actualRuntimeCounters.S5Counter, 10, 32)
	if err != nil {
		return errors.Wrap(err, "CounterEnterPoweroff is not an integer")
	}
	if reportedRuntimeCounters.GetCounterEnterPoweroff() != int64(CounterEnterPoweroff) {
		return errors.New("CounterEnterPoweroff does not match")
	}
	return nil
}

// verifyNoRuntimeCounters verifies that there's no runtime counters record
// within a given time limit.
func verifyNoRuntimeCounters(server *erpserver.ErpServer) error {
	record, err := server.NextRecordAsync(3*time.Minute, false /*expectEvents*/)
	if err != nil {
		return errors.Wrap(err, "failed to get next ERP server record async")
	}
	if record != nil {
		return errors.Errorf("Expect no record, but received %+v", record)
	}
	return nil
}

func ReportingRuntimeCountersTelemetry(ctx context.Context, s *testing.State) {
	// Need to enable PSR if the device is proto. Ignore any error because
	// intel-psrtool would not exist in other environment and the command
	// would fail if PSR is already enabled on a proto device.
	psrtoolCmd := testexec.CommandContext(ctx, "intel-psrtool", "-a")
	if err := psrtoolCmd.Run(); err != nil {
		s.Log("Failed to run the intel-psrtool command: ", err)
	}

	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}
	// Set buffer size to 1.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 1)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}
	server.SetFilter(func(wr *reporting.WrappedRecord) bool {
		if *wr.Record.Destination != reporting.Destination_TELEMETRY_METRIC {
			return false
		}
		var m reporting.MetricData
		if err := proto.Unmarshal(wr.Record.Data, &m); err != nil {
			s.Error("Could not parse telemetry metric record: ", err)
		}
		return m.GetTelemetryData() != nil &&
			m.GetTelemetryData().RuntimeCountersTelemetry != nil
	})
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Update affiliation and policies.
	policies := []policy.Policy{
		&policy.DeviceReportRuntimeCounters{Val: true},
		&policy.DeviceReportRuntimeCountersCheckingRateMs{Val: 120_000_000},
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
		chrome.EnableFeatures(erpserver.MissiveDeaultFeature),
		chrome.EnableFeatures("EnableRuntimeCountersTelemetry"),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	// Ensure cros_healthd is running as it is used to fetch device runtime
	// counters for reported runtime counters validation.
	if err := upstart.EnsureJobRunning(ctx, "cros_healthd"); err != nil {
		s.Fatal("Could not start cros_healthd")
	}

	params := croshealthd.TelemParams{Category: croshealthd.TelemCategorySystem}
	runtimeCountersRaw, err := croshealthd.RunTelem(ctx, params, s.OutDir())
	if err != nil {
		s.Fatal("Failed to fetch PSR data from cros_healthd: ", err)
	}
	var expectedSystemResult systemResult
	if err := json.Unmarshal(runtimeCountersRaw, &expectedSystemResult); err != nil {
		s.Fatal("Failed to parse PSR data fetched from cros_healthd: ", err)
	}
	expectedRuntimeCountersResult := &expectedSystemResult.PsrInfo

	if !expectedRuntimeCountersResult.IsSupported {
		s.Log("PSR is unsupported. No record expected")
		if err := verifyNoRuntimeCounters(server); err != nil {
			s.Fatal("Failed to verify no runtime counters: ", err)
		}
		return
	}
	if expectedRuntimeCountersResult.LogState != "Started" {
		s.Log("PSR logging is not running. No record expected")
		if err := verifyNoRuntimeCounters(server); err != nil {
			s.Fatal("Failed to verify no runtime counters: ", err)
		}
		return
	}

	record := server.NextRecord()
	if record == nil {
		s.Fatal("Record is nil")
	}
	var m reporting.MetricData
	if err := proto.Unmarshal(record.Record.Data, &m); err != nil {
		s.Fatalf("Could not parse telemetry metric record: %s", err)
	}
	reportedRuntimeCountersResult := m.GetTelemetryData().RuntimeCountersTelemetry
	if reportedRuntimeCountersResult == nil {
		s.Fatal("No runtime counters telemetry reported")
	}

	if err := verifyRuntimeCounters(reportedRuntimeCountersResult, expectedRuntimeCountersResult); err != nil {
		testing.ContextLogf(ctx, "expected: %+v", expectedRuntimeCountersResult)
		testing.ContextLogf(ctx, "reported: %+v", reportedRuntimeCountersResult)
		s.Fatalf("Verification failed: %s", err)
	}
}
