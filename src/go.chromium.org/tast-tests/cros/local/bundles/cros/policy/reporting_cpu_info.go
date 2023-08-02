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

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"

	"chromiumos/reporting"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingCPUInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify CPU info is reported when ReportDeviceCpuInfo is enabled",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test author
		},
		Timeout:      3 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceCpuInfo{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

type cpuInfo struct {
	KeylockerInfo *keylockerinfo `json:"keylocker_info"`
}

type keylockerinfo struct {
	KeylockerConfigured bool `json:"keylocker_configured"`
}

func ReportingCPUInfo(ctx context.Context, s *testing.State) {
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
	// Set buffer size to 2.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 2)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}
	server.SetFilter(func(wr *reporting.WrappedRecord) bool {
		if *wr.Record.Destination != reporting.Destination_INFO_METRIC {
			return false
		}
		var m reporting.MetricData
		if err := proto.Unmarshal(wr.Record.Data, &m); err != nil {
			s.Error("Could not parse telemetry metric record: ", err)
		}
		return m.GetInfoData() != nil &&
			m.GetInfoData().CpuInfo != nil
	})
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Update affiliation and policies.
	policies := []policy.Policy{
		&policy.ReportDeviceCpuInfo{Val: true},
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
		chrome.EnableFeatures(missiveFeature),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	// Ensure cros_healthd is running.
	if err := upstart.EnsureJobRunning(ctx, "cros_healthd"); err != nil {
		s.Fatal("Could not start cros_healthd")
	}

	// Fetch CPU data from croshealthd.
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryCPU}
	audioRaw, err := croshealthd.RunTelem(ctx, params, s.OutDir())
	if err != nil {
		s.Fatal("Failed to fetch audio data from cros_healthd: ", err)
	}

	var cpuResult cpuInfo
	if err := json.Unmarshal(audioRaw, &cpuResult); err != nil {
		s.Fatal("Failed to parse CPU data fetched from cros_healthd: ", err)
	}

	// Wait for the record.
	record := server.NextRecord()
	if record == nil {
		s.Fatal("Record is nil")
	}
	var m reporting.MetricData
	if err := proto.Unmarshal(record.Record.Data, &m); err != nil {
		s.Fatal("Could not parse telemetry metric record: ", err)
	}

	keyLockerInfo := m.GetInfoData().GetCpuInfo().GetKeylockerInfo()
	if keyLockerInfo == nil {
		s.Fatal("Key locker info is null")
	}

	// Verify that the record matches what we got from croshealthd,
	// if the key locker is not supported we expect both fields to be false
	// and if its supported we expect the configured field to match the one
	// obtained from the croshealthd.
	isSupported := cpuResult.KeylockerInfo != nil
	if isSupported {
		if !*keyLockerInfo.Supported {
			s.Fatal("Event is not supported on a supported device")
		}
		if *keyLockerInfo.Configured != cpuResult.KeylockerInfo.KeylockerConfigured {
			s.Fatal("Event configured doesn't match")
		}
	} else {
		if *keyLockerInfo.Supported {
			s.Fatal("Event is supported on a non supported device")
		}
		if *keyLockerInfo.Configured {
			s.Fatal("Event is configured on a non supported device")
		}
	}
}
