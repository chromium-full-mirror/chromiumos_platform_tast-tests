// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/reportingutil"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const loginLogoutPolicyReportingTimeout = 7 * time.Minute

type loginLogoutPolicyReportingParameters struct {
	reportingEnabled bool // test should expect reporting enabled
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         LoginLogoutPolicyReporting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a device and verify device login/logout events, device lock/unlock and user added events",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test owner
		},
		BugComponent: "b:817866", // ChromeOS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service"},
		Timeout:      loginLogoutPolicyReportingTimeout,
		Params: []testing.Param{
			{
				Name: "enabled",
				Val: loginLogoutPolicyReportingParameters{
					reportingEnabled: true,
				},
			}, {
				Name: "disabled",
				Val: loginLogoutPolicyReportingParameters{
					reportingEnabled: false,
				},
			},
		},
		VarDeps: []string{
			reportingutil.EventsAPIKeyPath,
			tape.ServiceAccountVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceLoginLogout{}, pci.VerifiedValue),
		},
	})
}

func validateAddedRemovedEvents(ctx context.Context, events []reportingutil.InputEvent, reportingEnabled bool, email string) error {
	if !reportingEnabled && len(events) > 0 {
		return errors.New("added removed events found when reporting is disabled")
	}

	if reportingEnabled && len(events) > 1 {
		return errors.New("more than one addRemoveUserEvent found")
	}

	if reportingEnabled {
		addedEvent := events[0].WrappedEncryptedData.AddRemoveUserEvent
		if addedEvent.UserAddedEvent == nil {
			return errors.New("didn't find the UserAddedEvent")
		}
		if addedEvent.AffiliatedUser.UserEmail != email {
			return errors.Errorf("AffiliatedUser email didn't match on UserAddedEvent, have %v wanted %v", addedEvent.AffiliatedUser.UserEmail, email)
		}
	}
	return nil
}

func validateLoginLogoutEvents(ctx context.Context, events []reportingutil.InputEvent, reportingEnabled bool, email string) error {
	if !reportingEnabled && len(events) > 0 {
		return errors.New("Device login/logout events found when reporting is disabled")
	}

	if reportingEnabled && len(events) > 2 {
		return errors.New("more than one addRemoveUserEvent found")
	}

	if reportingEnabled {
		loginEvent := events[1].WrappedEncryptedData.LoginLogoutEvent
		logoutEvent := events[0].WrappedEncryptedData.LoginLogoutEvent
		if loginEvent.LoginEvent == nil {
			return errors.New("didn't find the LoginEvent")
		}
		if loginEvent.AffiliatedUser.UserEmail != email {
			return errors.Errorf("AffiliatedUser email didn't match on UserAddedEvent, have %v wanted %v", loginEvent.AffiliatedUser.UserEmail, email)
		}
		if logoutEvent.LogoutEvent == nil {
			return errors.New("didn't find the LogoutEvent")
		}
		if logoutEvent.AffiliatedUser.UserEmail != email {
			return errors.Errorf("AffiliatedUser email didn't match on UserAddedEvent, have %v wanted %v", logoutEvent.AffiliatedUser.UserEmail, email)
		}
	}
	return nil
}

func validateLockUnlockEvents(ctx context.Context, events []reportingutil.InputEvent, reportingEnabled bool, email string) error {
	if !reportingEnabled && len(events) > 0 {
		return errors.New("Device lock/unlock events found when reporting is disabled")
	}

	if reportingEnabled && len(events) > 3 {
		return errors.New("more than three lockUnlockEvents found")
	}
	if reportingEnabled {
		succesfulUnlockEvent := events[0].WrappedEncryptedData.LockUnlockEvent
		failedUnlockEvent := events[1].WrappedEncryptedData.LockUnlockEvent
		lockEvent := events[2].WrappedEncryptedData.LockUnlockEvent
		if succesfulUnlockEvent.UnlockEvent == nil {
			return errors.New("didn't find the UnlockEvent")
		}
		if succesfulUnlockEvent.AffiliatedUser.UserEmail != email {
			return errors.Errorf("AffiliatedUser email didn't match on UserAddedEvent, have %v wanted %v", succesfulUnlockEvent.AffiliatedUser.UserEmail, email)
		}
		if !succesfulUnlockEvent.UnlockEvent.Success {
			return errors.New("was expecting successful unlock event but got failed event")
		}
		if succesfulUnlockEvent.UnlockEvent.UnlockType != "PASSWORD" {
			return errors.Errorf("was expecting password for unlock type on succesfulUnlockEvent, got %v", succesfulUnlockEvent.UnlockEvent.UnlockType)
		}
		if failedUnlockEvent.UnlockEvent == nil {
			return errors.New("didn't find the UnlockEvent")
		}
		if failedUnlockEvent.AffiliatedUser.UserEmail != email {
			return errors.Errorf("AffiliatedUser email didn't match on UserAddedEvent, have %v wanted %v", failedUnlockEvent.AffiliatedUser.UserEmail, email)
		}
		if failedUnlockEvent.UnlockEvent.Success {
			return errors.New("was expecting failed unlock event but got succesful event")
		}
		if failedUnlockEvent.UnlockEvent.UnlockType != "PASSWORD" {
			return errors.Errorf("was expecting password for unlock type on failedUnlockEvent, got %v", succesfulUnlockEvent.UnlockEvent.UnlockType)
		}
		if lockEvent.LockEvent == nil {
			return errors.New("didn't find the LockEvent")
		}
		if lockEvent.AffiliatedUser.UserEmail != email {
			return errors.Errorf("AffiliatedUser email didn't match on UserAddedEvent, have %v wanted %v", lockEvent.AffiliatedUser.UserEmail, email)
		}
	}
	return nil
}

// LoginLogoutPolicyReporting tests data reported when the ReportLoginLogout policy is enabled.
func LoginLogoutPolicyReporting(ctx context.Context, s *testing.State) {
	reportingEnabled := s.Param().(loginLogoutPolicyReportingParameters).reportingEnabled
	APIKey := s.RequiredVar(reportingutil.EventsAPIKeyPath)
	sa := []byte(s.RequiredVar(tape.ServiceAccountVar))

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(ctx)

	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Minute)
	defer cancel()

	if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	pc := pspb.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(loginLogoutPolicyReportingTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(ctx)

	// Enable or disable the policies depending on the param.
	var telemetryAllowlist []string
	if reportingEnabled {
		telemetryAllowlist = []string{"report_login_logout"}
	} else {
		telemetryAllowlist = []string{}
	}

	if err := reportingutil.SetTelemetryPolicies(ctx, tapeClient, acc.RequestID, reportingutil.Custom, telemetryAllowlist, true); err != nil {
		s.Fatal("Failed to set the policy: ", err)
	}

	user := acc.Username
	pass := acc.Password

	testStartTime := time.Now()
	if _, err := pc.GAIAEnrollForReporting(ctx, &pspb.GAIAEnrollForReportingRequest{
		Username:           user,
		Password:           pass,
		DmserverUrl:        policy.DMServerAlphaURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnabledFeatures:    "EncryptedReportingPipeline, EnableTelemetryTestingRates, OobeConsolidatedConsent, ClientAutomatedTest",
		SkipLogin:          false,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer pc.StopChrome(ctx, &empty.Empty{})
	defer reportingutil.Deprovision(ctx, cl.Conn, sa, acc.CustomerID)

	c, err := pc.ClientID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to grab client ID from device: ", err)
	}

	pJSON, err := policy.MarshalList([]policy.Policy{
		&policy.ReportDeviceLoginLogout{Stat: policy.StatusSet, Val: reportingEnabled},
	})
	if err != nil {
		s.Fatal("Failed to marshall expected login/logout policy for verification: ", err)
	}
	// Wait some time for the policy to propagate.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := pc.VerifyPolicyStatus(ctx, &ps.VerifyPolicyStatusRequest{
			Policies: pJSON,
		})
		return err
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Error("Failed to verify login/logout policy: ", err)
	}

	// Lock the device.
	if _, err := pc.LockDevice(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to lock the device: ", err)
	}

	// Try to unlock the device with an incorrect password.
	if _, err := pc.UnlockDeviceWithPassword(ctx, &pspb.UnlockDeviceWithPasswordRequest{
		Username: user,
		Password: pass + "incorrectPassword",
	}); err != nil {
		s.Fatal("Failed to request device unlock: ", err)
	}

	// Unlock the device with the correct password.
	if _, err := pc.UnlockDeviceWithPassword(ctx, &pspb.UnlockDeviceWithPasswordRequest{
		Username: user,
		Password: pass,
	}); err != nil {
		s.Fatal("Failed to request device unlock: ", err)
	}

	// Logout of the user session.
	if _, err := pc.Logout(ctx, &empty.Empty{}); err != nil {
		s.Fatal("Failed to logout from the device: ", err)
	}

	testing.ContextLog(ctx, "Waiting for 1 min to check for reported events")
	// GoBigSleepLint: Wait 1 min for events to reach the server.
	if err = testing.Sleep(ctx, 1*time.Minute); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		addedRemovedEvents, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, acc.CustomerID, c.ClientId, APIKey, "ADDED_REMOVED_EVENTS", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up user added/removed events")
		}

		loginLogoutEvents, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, acc.CustomerID, c.ClientId, APIKey, "LOGIN_LOGOUT_EVENTS", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up device login/logout events")
		}

		lockUnlockEvents, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, acc.CustomerID, c.ClientId, APIKey, "LOCK_UNLOCK_EVENTS", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up device lock/unlock events")
		}

		// Verify user added/removed events.
		if err = validateAddedRemovedEvents(ctx, addedRemovedEvents, reportingEnabled, user); err != nil {
			return testing.PollBreak(errors.Wrap(err, "invalid  added user event"))
		}

		// Verify device login/logout events.
		if err = validateLoginLogoutEvents(ctx, loginLogoutEvents, reportingEnabled, user); err != nil {
			return testing.PollBreak(errors.Wrap(err, "invalid  added user event"))
		}

		// Verify device lock/unlock events.
		if err = validateLockUnlockEvents(ctx, lockUnlockEvents, reportingEnabled, user); err != nil {
			return testing.PollBreak(errors.Wrap(err, "invalid  added user event"))
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  6 * time.Minute,
		Interval: 3 * time.Minute,
	}); err != nil {
		s.Errorf("Failed to validate events: %v:", err)
	}
}
