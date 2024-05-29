// Copyright 2024 The ChromiumOS Authors
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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type reportingPrivacyScreenInfoParameters struct {
	privacyScreenDevice bool // test being ran on a device with a privacy screen
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingPrivacyScreenInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify network telemetry is reported when ReportDeviceNetworkStatus is enabled",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		Timeout:      4 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:enterprise-reporting"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceGraphicsStatus{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name:              "non_privacy_screen_device",
				ExtraHardwareDeps: hwdep.D(hwdep.NoPrivacyScreen()),
				Val: reportingPrivacyScreenInfoParameters{
					privacyScreenDevice: false,
				},
			}, {
				Name:              "privacy_screen_device",
				ExtraHardwareDeps: hwdep.D(hwdep.PrivacyScreen()),
				Val: reportingPrivacyScreenInfoParameters{
					privacyScreenDevice: true,
				},
			},
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

type displayInfo struct {
	EmbeddedDisplay embedddedDisplay `json:"embedded_display"`
}

type embedddedDisplay struct {
	PrivacyScreenSupported bool `json:"privacy_screen_supported"`
}

func ReportingPrivacyScreenInfo(ctx context.Context, s *testing.State) {
	privacyScreenDevice := s.Param().(reportingPrivacyScreenInfoParameters).privacyScreenDevice
	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}

	// Set buffer size to 2 since we are only interested in the first record.
	// This just sets the queue size.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID /*buffer_size=*/, 2)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}
	server.SetFilter(func(wr *reporting.WrappedRecord) bool {
		if *wr.Record.Destination != reporting.Destination_INFO_METRIC {
			return false
		}
		var m reporting.MetricData
		if err := proto.Unmarshal(wr.Record.Data, &m); err != nil {
			s.Error("Could not parse info metric record: ", err)
		}
		return m.GetInfoData() != nil &&
			m.GetInfoData().PrivacyScreenInfo != nil
	})
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Update affiliation and policies.
	policies := []policy.Policy{
		&policy.ReportDeviceGraphicsStatus{Val: true},
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
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
	}

	// Ensure cros_healthd is running as it is used to fetch the information
	// to validate the reported event.
	if err := upstart.EnsureJobRunning(ctx, "cros_healthd"); err != nil {
		s.Fatal("Could not start cros_healthd")
	}

	// Collect the local info and transform it into a go struct.
	// This only queries the data from cros_healthd, doesn't trigger any
	// of our events.
	// The privacy screen telemetry is collected on startup.
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryDisplay}
	inputRaw, err := croshealthd.RunTelem(ctx, params, s.OutDir())
	if err != nil {
		s.Fatal("Failed to fetch input data from cros_healthd: ", err)
	}
	var inputResult displayInfo

	if err := json.Unmarshal(inputRaw, &inputResult); err != nil {
		s.Fatal("Failed to parse display data fetched from cros_healthd: ", err)
	}

	if inputResult.EmbeddedDisplay.PrivacyScreenSupported != privacyScreenDevice {
		s.Fatal("HW dependency mismatch error, have: ", inputResult.EmbeddedDisplay.PrivacyScreenSupported, " want: ", privacyScreenDevice)
	}

	// Wait for the record then compare it to the one obtained from
	// croshealthd above.
	record := server.NextRecord()
	if record == nil {
		s.Fatal("Record is nil")
	}
	var m reporting.MetricData
	if err := proto.Unmarshal(record.Record.Data, &m); err != nil {
		s.Fatal("Could not parse info metric record: ", err)
	}

	reportedInfo := m.GetInfoData().GetPrivacyScreenInfo()
	if reportedInfo == nil {
		s.Fatal("No privacy screen info reported")
	}
	if reportedInfo.GetSupported() != inputResult.EmbeddedDisplay.PrivacyScreenSupported {
		s.Fatal("Privacy screen supported info mismatch")
	}
}
