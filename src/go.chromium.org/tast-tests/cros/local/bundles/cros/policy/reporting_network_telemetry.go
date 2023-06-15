// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"strconv"
	"strings"
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
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"chromiumos/reporting"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingNetworkTelemetry,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify network telemetry is reported when ReportDeviceNetworkStatus is enabled",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"anasr@google.com", // Test author
		},
		Timeout:      10 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:mainline", "informational", "group:dmserver-enrollment-daily", "group:enterprise-reporting"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceNetworkStatus{}, pci.VerifiedFunctionalityOS),
			pci.SearchFlag(&policy.ReportDeviceNetworkTelemetryCollectionRateMs{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

type networkData struct {
	Type string `json:"type"`
	GUID string `json:"guid"`
}

type networkResult struct {
	Networks []networkData `json:"networks"`
}

func verifyNetwork(reportedNetwork *reporting.NetworkTelemetry, actualNetworks networkResult) error {
	if reportedNetwork.GetGuid() == "" {
		return errors.New("Guid is empty")
	}
	if reportedNetwork.GetType() == reporting.NetworkType_NETWORK_TYPE_UNSPECIFIED {
		return errors.New("Type is unspecified")
	}
	if reportedNetwork.GetConnectionState() == reporting.NetworkConnectionState_NETWORK_CONNECTION_STATE_UNSPECIFIED {
		return errors.New("State is unspecified")
	}
	for _, network := range actualNetworks.Networks {
		if network.GUID != *reportedNetwork.Guid {
			continue
		}
		actualType := strings.ToUpper(network.Type)
		reportedType := reportedNetwork.Type.String()
		// TODO(b/288145052): Check other fields if possible.
		if reportedType != actualType {
			return errors.Errorf("Reported network type %s does not match actual type %s", reportedType, actualType)
		}
		return nil
	}
	return errors.Errorf("Reported network GUID %s was not found in the list of actual networks", *reportedNetwork.Guid)
}

func ReportingNetworkTelemetry(ctx context.Context, s *testing.State) {
	missiveFeature := "CrOSLateBootMissiveStorage:signature_verification_dev_enabled/true/compression_enabled/false"
	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}
	// Set buffer size to 2 since only 2 records are expected in this test.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 2)
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
			m.GetTelemetryData().NetworksTelemetry != nil &&
			len(m.GetTelemetryData().NetworksTelemetry.NetworkTelemetry) > 0
	})
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()

	// Update affiliation and policies.
	policies := []policy.Policy{
		&policy.ReportDeviceNetworkStatus{Val: true},
		&policy.ReportDeviceNetworkTelemetryCollectionRateMs{Val: 120000},
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

	// Ensure cros_healthd is running as it is used to fetch device networks
	// for reported networks validation.
	if err := upstart.EnsureJobRunning(ctx, "cros_healthd"); err != nil {
		s.Fatal("Could not start cros_healthd")
	}

	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryNetwork}
	networksRaw, err := croshealthd.RunTelem(ctx, params, s.OutDir())
	if err != nil {
		s.Fatal("Failed to fetch networks data from cros_healthd: ", err)
	}
	var networkResult networkResult
	if err := json.Unmarshal(networksRaw, &networkResult); err != nil {
		s.Fatal("Failed to parse networks data fetched from cros_healthd: ", err)
	}

	// Initial collection happens 1min after login, and initial upload
	// happens 4mins after login. Collection rate is set to 2mins which
	// will trigger exactly 1 more collection after the initial one
	// resulting 2 network telemetry records in the initial upload.
	// TODO(b/288882710): Make collection and upload rates configurable to
	// reduce them for testing and update the test accordingly.
	for i := 0; i < 2; i++ {
		record := server.NextRecord()
		if record == nil {
			s.Errorf("Record %d is nil", i)
		}
		var m reporting.MetricData
		if err := proto.Unmarshal(record.Record.Data, &m); err != nil {
			s.Errorf("Could not parse telemetry metric record %d: %s", i, err)
		}
		networks := m.GetTelemetryData().NetworksTelemetry.NetworkTelemetry
		for j, n := range networks {
			if err := verifyNetwork(n, networkResult); err != nil {
				s.Errorf("Verification failed for network %d of record %d: %s", j, i, err)
			}
		}
	}
}
