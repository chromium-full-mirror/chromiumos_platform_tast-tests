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
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/croshealthd"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/upstart"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingInputInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify network telemetry is reported when ReportDeviceNetworkStatus is enabled",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test author
		},
		Timeout:      3 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome", "vpd"},
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		Fixture:      fixture.FakeDMSEnrolled,
		HardwareDeps: hwdep.D(hwdep.TouchScreen()),
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceGraphicsStatus{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

type inputInfo struct {
	TouchpadLibraryName string              `json:"touchpad_library_name"`
	TouchscreenDevices  []touchscreenDevice `json:"touchscreen_devices"`
}

type touchscreenDevice struct {
	TouchPoints int `json:"touch_points"`
}

func verifyInput(reportedInfo *reporting.TouchScreenInfo, localInfo inputInfo) error {
	if reportedInfo.GetLibraryName() != localInfo.TouchpadLibraryName {
		return errors.New("Library name doesn't match")
	}
	if len(reportedInfo.GetTouchScreenDevices()) != len(localInfo.TouchscreenDevices) {
		return errors.New("Touch screen devices size mismatch")
	}
	return nil
}

func ReportingInputInfo(ctx context.Context, s *testing.State) {
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
			s.Error("Could not parse info metric record: ", err)
		}
		return m.GetInfoData() != nil &&
			m.GetInfoData().TouchScreenInfo != nil
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
		chrome.EnableFeatures(erpserver.MissiveDeaultFeature),
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
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryInput}
	inputRaw, err := croshealthd.RunTelem(ctx, params, s.OutDir())
	if err != nil {
		s.Fatal("Failed to fetch input data from cros_healthd: ", err)
	}

	var inputResult inputInfo
	if err := json.Unmarshal(inputRaw, &inputResult); err != nil {
		s.Fatal("Failed to parse input data fetched from cros_healthd: ", err)
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

	reportedInfo := m.GetInfoData().GetTouchScreenInfo()
	if reportedInfo == nil {
		s.Fatal("No touch screen info reported")
	}
	if err := verifyInput(reportedInfo, inputResult); err != nil {
		s.Errorf("Verification failed for network %s of record: %s", reportedInfo, err)
	}
}
