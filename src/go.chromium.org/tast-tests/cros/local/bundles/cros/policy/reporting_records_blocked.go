// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"time"

	"go.chromium.org/chromiumos/reporting"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"

	"go.chromium.org/tast-tests/cros/common/fixture"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/fakedms"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/lockscreen"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/quicksettings"
	"go.chromium.org/tast-tests/cros/local/erpserver"
	"go.chromium.org/tast-tests/cros/local/input"
	"go.chromium.org/tast-tests/cros/local/policyutil/fixtures"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingRecordsBlocked,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify that the configuration file is blocking the destinations as intended",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:enterprise-reporting", "group:hw_agnostic"},
		SoftwareDeps: []string{"chrome"},
		Fixture:      fixture.FakeDMSEnrolled,
		Timeout:      5 * time.Minute,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceLoginLogout{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

func triggerLockUnlockEvent(ctx context.Context, tconn *chrome.TestConn, keyboard *input.KeyboardEventWriter, username, password string) error {
	// Lock the screen
	if err := quicksettings.LockScreen(ctx, tconn); err != nil {
		return errors.Wrap(err, "fail to lock")
	}

	// Unlock the screen.
	if err := lockscreen.WaitForPasswordField(ctx, tconn, username, 10*time.Second); err != nil {
		return errors.Wrap(err, "failed to wait for the password field")
	}

	if err := lockscreen.EnterPassword(ctx, tconn, username, password, keyboard); err != nil {
		return errors.Wrap(err, "failed to enter password")
	}

	return nil
}

func ReportingRecordsBlocked(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Update affiliation and policies.
	policies := []policy.Policy{
		&policy.ReportDeviceLoginLogout{Val: true}}
	pb := policy.NewBlob()
	affiliationID := []string{"affiliation_id"}
	pb.DeviceAffiliationIds = affiliationID
	pb.UserAffiliationIds = affiliationID
	pb.AddPolicies(policies)
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

	// Setting filter to only return lock unlock and login logout events.
	server.SetFilter(func(wr *reporting.WrappedRecord) bool {
		return *wr.Record.Destination == reporting.Destination_LOCK_UNLOCK_EVENTS || *wr.Record.Destination == reporting.Destination_LOGIN_LOGOUT_EVENTS || *wr.Record.Destination == reporting.Destination_PERIPHERAL_EVENTS
	})

	// Start the server.
	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Start chrome with the tests features enabled.
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures(erpserver.MissiveDefaultFeature, "ShouldRequestConfigurationFile, ReportingConfigurationFileTestSignature, CrOSLateBootMissiveConfigurationFile:blocking_destinations_enabled/true"),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Failed to create Chrome instance: ", err)
	}

	// Create test connection and virtual keyboard.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		s.Fatal("Failed to create test API connection: ", err)
	}

	keyboard, err := input.VirtualKeyboard(ctx)
	if err != nil {
		s.Fatal("Failed to get virtual keyboard: ", err)
	}
	defer keyboard.Close(ctx)

	// Trigger lock and unlock events.
	testing.ContextLog(ctx, "Triggering lock/unlock events")
	if err := triggerLockUnlockEvent(ctx, tconn, keyboard, fixtures.Username, fixtures.Password); err != nil {
		s.Fatal("Failed to trigger lock unlock event and USB event: ", err)
	}

	testing.ContextLog(ctx, "Checking the events")
	ch := make(chan *reporting.WrappedRecord)
	lockUnlockEvents := 0
	loginLogoutEvents := 0
	// We are supposed to get 3 events, 1 login record and 2 lock/unlock records.
	for i := 0; i < 3; i++ {
		go func() {
			record := server.NextRecord()
			ch <- record
		}()
	}
	for i := 0; i < 3; i++ {
		record := <-ch
		if record == nil {
			s.Errorf("Record %d is nil", i)
		}
		if *record.Record.Destination == reporting.Destination_LOCK_UNLOCK_EVENTS {
			lockUnlockEvents++
		} else if *record.Record.Destination == reporting.Destination_LOGIN_LOGOUT_EVENTS {
			loginLogoutEvents++
		}
		testing.ContextLog(ctx, "current event: ", record)
	}

	if lockUnlockEvents != 2 && loginLogoutEvents != 1 {
		s.Errorf("Expected 2 lock unlock events and 1 peripheral event, got %d lock unlock events and %d login logout events", lockUnlockEvents, loginLogoutEvents)
	}

	// Set the fake config file on the go to block the lock/unlock destination.
	testing.ContextLog(ctx, "Setting the fake config file")
	server.SetFakeConfigFile(&erpserver.ResponseConfigFile{
		Version:             111111,
		ConfigFileSignature: "XjZLPbyAJuhpG0khSHNU7uexbuyLil9ceZcEE7zjRchZytV0ybMoRO59JjbVHHeTNpPslw7rpyg6px3J+so2DA==",
		BlockedEventConfigs: []erpserver.EventConfig{erpserver.EventConfig{
			Destination: "LOCK_UNLOCK_EVENTS",
		}}})

	testing.ContextLog(ctx, "Sleeping for 2 mins")
	// GoBigSleepLint: Let 2 minutes pass to make sure that the configuration gets populated.
	if err := testing.Sleep(ctx, 2*time.Minute); err != nil {
		s.Fatal("Failed to sleep for 2 minutes: ", err)
	}

	// Trigger lock and unlock events. Both of this events shouldn't be reported since we blocked the destination.
	testing.ContextLog(ctx, "Triggering lock/unlock and logout events")
	if err := triggerLockUnlockEvent(ctx, tconn, keyboard, fixtures.Username, fixtures.Password); err != nil {
		s.Fatal("Failed to trigger lock unlock event and USB event: ", err)
	}

	// Trigger a logout event. Only this one should be reported.
	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		s.Fatal("Failed to logout: ", err)
	}

	// Verify that the logout event got reported correctly.
	record := server.NextRecord()
	if record == nil {
		s.Fatal("Record is nil")
	}
	if *record.Record.Destination != reporting.Destination_LOGIN_LOGOUT_EVENTS {
		s.Fatal("Record is not login/logout destination, got destination = ", *record.Record.Destination)
	}
}
