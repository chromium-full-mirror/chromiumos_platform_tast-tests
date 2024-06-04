// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"encoding/json"
	"os"
	"strconv"
	"strings"
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

type reportingMemoryInfoParams struct {
	vProDevice bool // test should prepare vPro specific logic
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingMemoryInfo,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify memory info is reported when ReportDeviceMemoryInfo is enabled",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		Timeout:      3 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome", "vpd"},
		Attr:         []string{"group:criticalstaging", "group:mainline", "informational", "group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:enterprise-reporting"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceMemoryInfo{}, pci.VerifiedFunctionalityOS),
		},
		Params: []testing.Param{
			{
				Name:              "vpro_memory_reporting_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.Model("brya", "redrix")),
				Val: reportingMemoryInfoParams{
					vProDevice: true,
				},
			}, {
				Name:              "nonvpro_memory_reporting_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("brya", "redrix")),
				Val: reportingMemoryInfoParams{
					vProDevice: false,
				},
			},
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

type memoryEncryptionInfo struct {
	EncryptionState string `json:"encryption_state"`
	KeyLength       int64  `json:"key_length"`
	MaxKeyNumber    int64  `json:"max_key_number"`
}

// vProSupported checks if vPro features are on the device. Even on vPro supported models, vPro
// is not always supported on the specific device. Therefore some manual checking is required.
func vProSupported() (bool, error) {
	out, err := os.ReadFile("/proc/cpuinfo")
	if err != nil {
		return false, errors.Wrap(err, "failed to read /proc/cpuinfo file")
	}
	// Checking whether the system supports vPro feature or not.
	if strings.Contains(string(out), "tme") {
		return true, nil
	}
	return false, nil
}

func ReportingMemoryInfo(ctx context.Context, s *testing.State) {
	vProDevice := s.Param().(reportingMemoryInfoParams).vProDevice
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

	// Filtering so the server only stores memory info events.
	server.SetFilter(func(wr *reporting.WrappedRecord) bool {
		if *wr.Record.Destination != reporting.Destination_INFO_METRIC {
			return false
		}
		var m reporting.MetricData
		if err := proto.Unmarshal(wr.Record.Data, &m); err != nil {
			s.Error("Could not parse info metric record: ", err)
		}
		return m.GetInfoData() != nil &&
			m.GetInfoData().MemoryInfo != nil
	})
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Update affiliation and policies.
	policies := []policy.Policy{
		&policy.ReportDeviceMemoryInfo{Val: true},
	}
	pb := policy.NewBlob()
	affiliationID := []string{"affiliation_id"}
	pb.DeviceAffiliationIds = affiliationID
	pb.UserAffiliationIds = affiliationID
	pb.AddPolicies(policies)
	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Could not apply policy: ", err)
	}

	// Check if vPro is supported on the device.
	if vProDevice {
		if su, err := vProSupported(); err != nil {
			s.Fatalf("Failed to verify if vPro is supported: %v: ", err)
		} else if !su {
			// vPro is not actually enabled on this device, skip test for now.
			testing.ContextLog(ctx, "not testing vPro reporting on non vpro device")
			return
		}
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
	params := croshealthd.TelemParams{Category: croshealthd.TelemCategoryMemory}
	inputRaw, err := croshealthd.RunTelem(ctx, params, s.OutDir())
	if err != nil {
		s.Fatal("Failed to fetch input data from cros_healthd: ", err)
	}

	var inputResult memoryEncryptionInfo
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

	reportedInfo := m.GetInfoData().GetMemoryInfo().GetTmeInfo()
	if reportedInfo == nil {
		s.Fatal("No touch screen info reported")
	}

	// If there is no encyrption the field should be marked with the disabled enum.
	if inputResult.EncryptionState == "" && *reportedInfo.EncryptionState != reporting.MemoryEncryptionState_MEMORY_ENCRYPTION_STATE_DISABLED {
		s.Fatal("Encryption state is not disabled when it should be. Full TME event: ", reportedInfo)
	}

	// If there is an encryption state then the fields KeyLength
	// and MaxKeys should match the data gotten from croshealthd.
	if inputResult.EncryptionState != "" {
		if *reportedInfo.KeyLength != inputResult.KeyLength {
			s.Fatal("Key length mismatch. Full TME event: ", reportedInfo)
		}
		if *reportedInfo.MaxKeys != inputResult.MaxKeyNumber {
			s.Fatal("Max keys mismatch. Full TME event: ", reportedInfo)
		}
	}
}
