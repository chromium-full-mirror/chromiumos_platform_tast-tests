// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"time"

	"github.com/godbus/dbus/v5"
	"go.chromium.org/tast/core/testing"
	"google.golang.org/protobuf/proto"

	rep "go.chromium.org/chromiumos/reporting"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/versionutil"
	"go.chromium.org/tast-tests/cros/local/dbusutil"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingConfigFileDbus,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the configuration file is delivered correctly to missive",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:enterprise-reporting"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      2 * time.Minute,
		VarDeps:      []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

func ReportingConfigFileDbus(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}

	// Set buffer size to 1 since we are not expecting events.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 1)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}

	// Get the current version to use it on the config file.
	chromeVersion, err := versionutil.AshVersion(ctx)
	if err != nil {
		s.Fatal("Failed to get Chrome version: ", err)
	}
	chromeMajorVersion := int32(chromeVersion.Major())

	// Set the fake config file for the fake server to return
	// This fake configuration file tests the three different
	// options that are available for blocked events:
	// no versions set, only min version and both versions set.
	server.SetFakeConfigFile(&erpserver.ResponseConfigFile{
		Version:             111111,
		ConfigFileSignature: "XjZLPbyAJuhpG0khSHNU7uexbuyLil9ceZcEE7zjRchZytV0ybMoRO59JjbVHHeTNpPslw7rpyg6px3J+so2DA==",
		BlockedEventConfigs: []erpserver.EventConfig{erpserver.EventConfig{
			Destination: "HEARTBEAT_EVENTS",
		},
			erpserver.EventConfig{
				Destination:           "LOGIN_LOGOUT_EVENTS",
				MinimumReleaseVersion: chromeMajorVersion - 3,
			},
			erpserver.EventConfig{
				Destination:           "OS_EVENTS",
				MinimumReleaseVersion: chromeMajorVersion,
				MaximumReleaseVersion: chromeMajorVersion + 1,
			}}})

	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Start monitoring dbus for member UpdateConfigInMissive.
	m := []dbusutil.MatchSpec{{Type: "method_call",
		Path:      dbus.ObjectPath("/org/chromium/Missived"),
		Interface: "org.chromium.Missived",
		Member:    "UpdateConfigInMissive"}}

	mon, err := dbusutil.DbusEventMonitor(ctx, m)
	if err != nil {
		s.Fatal("Connection to missive dbus monitoring error: ", err)
	}

	// Start chrome with the two tests features enabled.
	_, err = chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures("ShouldRequestConfigurationFile, ReportingConfigurationFileTestSignature"),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Failed to create Chrome instance: ", err)
	}

	missiveCalls, err := mon()
	if err != nil {
		s.Fatal("Failed to capture dbus calls to missive: ", err)
	}

	if len(missiveCalls) > 1 {
		s.Fatal("More than one call to missive, full trace: ", missiveCalls)
	}

	if len(missiveCalls) == 0 {
		s.Fatal("Didn't call missive")
	}

	// Parse the call to a proto.
	if len(missiveCalls[0].Arguments) == 0 {
		s.Fatal("Event has no arguments")
	}
	arg, ok := missiveCalls[0].Arguments[0].([]byte)
	if !ok {
		s.Fatal("Failed to cast arguments")
	}
	listFromCall := &rep.ListOfBlockedDestinations{}
	if err := proto.Unmarshal(arg, listFromCall); err != nil {
		s.Fatal("Failed to unmarshal record request")
	}

	// Create a proto containing the expected list.
	listToCompare := &rep.ListOfBlockedDestinations{
		Destinations: []rep.Destination{8, rep.Destination_HEARTBEAT_EVENTS, 8, rep.Destination_LOGIN_LOGOUT_EVENTS, 8, rep.Destination_OS_EVENTS},
	}

	// Compare both protos.
	if !proto.Equal(listFromCall, listToCompare) {
		s.Fatal("Lists are not equal, got: ", listFromCall, " - wanted: ", listToCompare)
	}
}
