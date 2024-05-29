// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"strconv"
	"time"

	rep "go.chromium.org/chromiumos/reporting"

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
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         ReportingLoginLogoutPolicy,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify user added/removed, device lock/unlock, and user login/logout events are reported correctly",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test author
		},
		Timeout:      3 * time.Minute,
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		SoftwareDeps: []string{"chrome", "vpd"},
		Attr:         []string{"group:golden_tier", "group:medium_low_tier", "group:hardware", "group:complementary", "group:enterprise-reporting", "group:hw_agnostic"},
		Fixture:      fixture.FakeDMSEnrolled,
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceLoginLogout{}, pci.VerifiedFunctionalityOS),
		},
		VarDeps: []string{"erpserver.key_id", "erpserver.public_key", "erpserver.private_key", "erpserver.signature"},
	})
}

func ReportingLoginLogoutPolicy(ctx context.Context, s *testing.State) {
	fdms := s.FixtValue().(fakedms.HasFakeDMS).FakeDMS()
	// Prepare fake ERP server.
	publicKey := s.RequiredVar("erpserver.public_key")
	privateKey := s.RequiredVar("erpserver.private_key")
	signature := s.RequiredVar("erpserver.signature")
	keyID, err := strconv.Atoi(s.RequiredVar("erpserver.key_id"))
	if err != nil {
		s.Fatal("Could not convert key ID to int: ", err)
	}
	// Set buffer size to 5.
	server, err := erpserver.New(publicKey, privateKey, signature, keyID, 5)
	if err != nil {
		s.Fatal("Reporting server creation failed: ", err)
	}

	// Setting filter to only accept login/logout events, added/removed events, and lock/unlock events.
	server.SetFilter(func(wr *rep.WrappedRecord) bool {
		return *wr.Record.Destination == rep.Destination_LOCK_UNLOCK_EVENTS || *wr.Record.Destination == rep.Destination_LOGIN_LOGOUT_EVENTS || *wr.Record.Destination == rep.Destination_ADDED_REMOVED_EVENTS
	})

	if err = server.Start(ctx); err != nil {
		s.Fatal("Reporting server failed to start: ", err)
	}
	defer server.Close()

	// Update affiliation and policies.
	policies := []policy.Policy{
		&policy.ReportDeviceLoginLogout{Val: true},
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
	cr, err := chrome.New(ctx,
		chrome.FakeLogin(chrome.Creds{User: fixtures.Username, Pass: fixtures.Password}),
		chrome.DMSPolicy(fdms.URL),
		chrome.EncryptedReportingAddr(server.URL()),
		chrome.EnableFeatures(erpserver.MissiveDefaultFeature),
		chrome.KeepEnrollment())
	if err != nil {
		s.Fatal("Chrome start failed: ", err)
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

	// Lock the screen
	if err := quicksettings.LockScreen(ctx, tconn); err != nil {
		s.Fatal("Failed to lock the screen: ", err)
	}

	// Unlock the screen.
	if err := lockscreen.WaitForPasswordField(ctx, tconn, fixtures.Username, 10*time.Second); err != nil {
		s.Fatal("Failed to wait for the password field: ", err)
	}

	if err := lockscreen.EnterPassword(ctx, tconn, fixtures.Username, fixtures.Password, keyboard); err != nil {
		s.Fatal("Failed to enter password: ", err)
	}

	// Trigger a logout event.
	if err := quicksettings.SignOut(ctx, tconn); err != nil {
		s.Fatal("Failed to logout: ", err)
	}

	// We are supposed to get 5 events:
	// 1 added user event
	// 2 lock unlock events
	// 2 login logout event
	ch := make(chan *rep.WrappedRecord)
	lockUnlockEvents := 0
	loginLogoutEvents := 0
	addedRemovedEvents := 0
	for i := 0; i < 5; i++ {
		go func() {
			record := server.NextRecord()
			testing.ContextLog(ctx, "record: ", record)
			ch <- record
		}()
	}
	for i := 0; i < 5; i++ {
		record := <-ch
		if record == nil {
			s.Errorf("Record %d is nil", i)
		}
		if *record.Record.Destination == rep.Destination_LOCK_UNLOCK_EVENTS {
			lockUnlockEvents++
		} else if *record.Record.Destination == rep.Destination_LOGIN_LOGOUT_EVENTS {
			loginLogoutEvents++
		} else if *record.Record.Destination == rep.Destination_ADDED_REMOVED_EVENTS {
			addedRemovedEvents++
		}
		testing.ContextLog(ctx, "Current event: ", record)
	}

	// Check that we got the expected number of events of each type.
	if lockUnlockEvents != 2 && loginLogoutEvents != 2 && addedRemovedEvents != 1 {
		s.Fatal("Expected 2 lock unlock events, 2 login logout events and 1 added/removed event, got: ", lockUnlockEvents, " lock unlock events, ", loginLogoutEvents, " login logout events and ", addedRemovedEvents, " added/removed events")
	}
}
