// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/reportingutil"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const heartbeatReportingTimeout = 7 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func:         HeartbeatReporting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a device and verify heartbeat reporting functionality",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com",
			"rzakarian@google.com",
		},
		BugComponent: "b:817866", // Chrome OS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service", "tast.cros.graphics.ScreenshotService"},
		Timeout:      heartbeatReportingTimeout,
		VarDeps: []string{
			reportingutil.ManagedChromeCustomerIDPath,
			reportingutil.EventsAPIKeyPath,
			tape.ServiceAccountVar,
		},
	})
}

// HeartbeatReporting tests that the ERP sends heartbeats to the server if enabled.
func HeartbeatReporting(ctx context.Context, s *testing.State) {
	customerID := s.RequiredVar(reportingutil.ManagedChromeCustomerIDPath)
	APIKey := s.RequiredVar(reportingutil.EventsAPIKeyPath)
	sa := []byte(s.RequiredVar(tape.ServiceAccountVar))

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(ctx)

	if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)
	defer reportingutil.Deprovision(ctx, cl.Conn, sa, customerID)

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	captureScreenshotOnError := func(ctx context.Context, hasError func() bool) {
		if !hasError() {
			return
		}

		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "reportingError"})
	}
	defer captureScreenshotOnError(ctx, s.HasError)

	policyClient := ps.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(heartbeatReportingTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(ctx)

	// Disable Asset ID screen on enrollment.
	if err := reportingutil.DisableUpdatingDeviceAttribute(ctx, tapeClient, acc.RequestID); err != nil {
		s.Fatal("Failed to set the asset policy: ", err)
	}

	testStartTime := time.Now()
	if _, err := policyClient.GAIAEnrollForReporting(ctx, &ps.GAIAEnrollForReportingRequest{
		Username:           acc.Username,
		Password:           acc.Password,
		DmserverUrl:        reportingutil.DmServerURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnabledFeatures:    "EncryptedReportingPipeline, EncryptedReportingManualTestHeartbeatEvent",
		SkipLogin:          true,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer policyClient.StopChrome(ctx, &empty.Empty{})

	c, err := policyClient.ClientID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatalf("Failed to grab client ID from device: %v:", err)
	}

	// TODO(b/296398085) : Remove once server solution is in place.
	// Check if the device is marked as deprovisioned on the
	// server, if it was then exit the test since the server
	// won't allow any events coming from this device.
	// Trying to get an idea of how many errors where coming from
	// this failure, will remove once a better solution is in place.
	currentAccount := accManager.Accounts[0]
	isDeprovisioned, err := tapeClient.Deprovisioned(ctx, c.ClientId, currentAccount.OrgUnitPath, currentAccount.CustomerID)
	if err != nil {
		s.Fatal("Failed to check if device is deprovisioned: ", err)
	}
	if isDeprovisioned {
		testing.ContextLog(ctx, "Device is deprovisioned - not checking events")
		return
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		events, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, customerID, c.ClientId, APIKey, "HEARTBEAT_EVENTS", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up events")
		}
		if len(events) < 1 {
			return errors.New("no event found")
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  2 * time.Minute,
		Interval: 30 * time.Second,
	}); err != nil {
		s.Errorf("Failed to validate heartbeat event: %v:", err)
	}
}
