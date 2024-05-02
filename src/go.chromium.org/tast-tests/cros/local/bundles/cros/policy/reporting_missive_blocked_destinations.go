// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/versionutil"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
	"go.chromium.org/tast-tests/cros/local/syslog"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingMissiveBlockedDestinations,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the configuration file is reaching missive and being updated correctly",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:enterprise-reporting"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      5 * time.Minute,
		VarDeps:      []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

func ReportingMissiveBlockedDestinations(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Update affiliation.
	pb := policy.NewBlob()
	affiliationID := []string{"affiliation_id"}
	pb.DeviceAffiliationIds = affiliationID
	pb.UserAffiliationIds = affiliationID
	if err := fdms.WritePolicyBlob(pb); err != nil {
		s.Fatal("Could not apply policy: ", err)
	}

	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}

	// Create the server.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 20)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}

	// Get the current version to use it on the config file.
	chromeVersion, err := versionutil.AshVersion(ctx)
	if err != nil {
		s.Fatal("Failed to get Chrome version: ", err)
	}
	chromeMajorVersion := int32(chromeVersion.Major())

	// Set the fake config file on the go to block the lock/unlock destination.
	server.SetFakeConfigFile(&erpserver.ResponseConfigFile{
		Version:             111111,
		ConfigFileSignature: "XjZLPbyAJuhpG0khSHNU7uexbuyLil9ceZcEE7zjRchZytV0ybMoRO59JjbVHHeTNpPslw7rpyg6px3J+so2DA==",
		BlockedEventConfigs: []erpserver.EventConfig{
			// This destination is valid and should appear in the logs.
			erpserver.EventConfig{
				Destination:           "LOGIN_LOGOUT_EVENTS",
				MinimumReleaseVersion: chromeMajorVersion - 3,
			},
			// This destination shouldn't be on the logs since it's invalid.
			erpserver.EventConfig{
				Destination:           "OS_EVENTS",
				MinimumReleaseVersion: chromeMajorVersion + 1,
				MaximumReleaseVersion: chromeMajorVersion + 5,
			}}})

	// Start the server.
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Create a new log reader.
	logReader, err := syslog.NewReader(ctx, syslog.SourcePath(syslog.MessageFile))
	if err != nil {
		s.Fatal("Failed to create log reader: ", err)
	}

	// Start chrome with the tests features enabled.
	_, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures(erpserver.MissiveDefaultFeature, "ShouldRequestConfigurationFile, ReportingConfigurationFileTestSignature, CrOSLateBootMissiveConfigurationFile:blocking_destinations_enabled/true"),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Failed to create Chrome instance: ", err)
	}

	// If the destination list got updated the logs should reflect that.
	containsMessage := func(e *syslog.Entry) bool {
		return strings.Contains(e.Program, "missived") &&
			strings.Contains(e.Content, "Destination") &&
			strings.Contains(e.Content, "LOGIN_LOGOUT_EVENTS") &&
			strings.Contains(e.Content, "switched to blocked")
	}

	// Check the log for the relevant message.
	msg, err := logReader.Wait(ctx, 30*time.Second, containsMessage)
	if err != nil {
		s.Fatal("Failed to found the message on the logs: ", err)
	}

	testing.ContextLog(ctx, "Message found in the logs: ", msg)
}
