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

type heartbeatTestParams struct {
	IsUserEvent     bool // If true, send user events from umanaged device, else send device events from managed device.
	EnabledFeatures string
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         HeartbeatReporting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify heartbeat reporting functionality",
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
		Params: []testing.Param{
			{
				Name: "report_user_heartbeat_event_from_unmanaged_device",
				Val: heartbeatTestParams{
					IsUserEvent: true,
					// Enable the reporting pipeline, user heartbeat events, reporting from unmanaged device, and enable multigenerational storage for FAST_BATCH priority (i.e. exclude FAST_BATCH from legacy_storage_enabled list).
					EnabledFeatures: "EncryptedReportingPipeline, EncryptedReportingManualTestUserHeartbeatEvent, EnableReportingFromUnmanagedDevices, ClientAutomatedTest, CrOSLateBootMissiveStorage:legacy_storage_enabled/IMMEDIATE,SLOW_BATCH,BACKGROUND_BATCH,MANUAL_BATCH,SECURITY,MANUAL_BATCH_LACROS",
				},
			},
			{
				Name: "report_device_heartbeat_event_from_managed_device",
				Val: heartbeatTestParams{
					IsUserEvent: false,
					// Enable the reporting pipeline, device heartbeat events.
					EnabledFeatures: "EncryptedReportingPipeline, EncryptedReportingManualTestHeartbeatEvent, ClientAutomatedTest",
				},
			},
			{
				Name: "report_device_heartbeat_event_from_managed_device_using_multigenerational_storage",
				Val: heartbeatTestParams{
					IsUserEvent: false,
					// Enable the reporting pipeline, device heartbeat events, and multigenerational storage for FAST_BATCH priority (i.e. exclude FAST_BATCH from legacy_storage_enabled list).
					EnabledFeatures: "EncryptedReportingPipeline, EncryptedReportingManualTestHeartbeatEvent, ClientAutomatedTest, CrOSLateBootMissiveStorage:legacy_storage_enabled/IMMEDIATE,SLOW_BATCH,BACKGROUND_BATCH,MANUAL_BATCH,SECURITY,MANUAL_BATCH_LACROS",
				},
			},
		},
	})
}

// HeartbeatReporting tests that the ERP sends heartbeats to the server if enabled.
func HeartbeatReporting(ctx context.Context, s *testing.State) {
	customerID := s.RequiredVar(reportingutil.ManagedChromeCustomerIDPath)
	APIKey := s.RequiredVar(reportingutil.EventsAPIKeyPath)
	sa := []byte(s.RequiredVar(tape.ServiceAccountVar))
	params := s.Param().(heartbeatTestParams)

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

	if params.IsUserEvent {
		// This is a user event. Login with managed user, but don't enroll the device.
		if _, err := policyClient.GAIALoginForReporting(ctx, &ps.GAIALoginForReportingRequest{
			Username:           acc.Username,
			Password:           acc.Password,
			DmserverUrl:        reportingutil.DmServerURL,
			ReportingServerUrl: reportingutil.ReportingServerURL,
			// Enable user heart beat events, reporting from unmanaged devices, and legacy/non-multigenerational storage for all priorities except FAST_BATCH (the priority that heartbeat events use).
			EnabledFeatures: params.EnabledFeatures,
		}); err != nil {
			s.Fatal("Failed to login to chrome with managed user: ", err)
		}
	} else {
		// This is a device event. Enroll device and maybe login, depending on `SkipLogin` setting.
		if _, err := policyClient.GAIAEnrollForReporting(ctx, &ps.GAIAEnrollForReportingRequest{
			Username:           acc.Username,
			Password:           acc.Password,
			DmserverUrl:        reportingutil.DmServerURL,
			ReportingServerUrl: reportingutil.ReportingServerURL,
			EnabledFeatures:    params.EnabledFeatures,
			SkipLogin:          true,
		}); err != nil {
			s.Fatal("Failed to enroll using chrome: ", err)
		}
	}

	testStartTime := time.Now()

	defer policyClient.StopChrome(ctx, &empty.Empty{})

	// Gather device info for device events
	var clientID string

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		var events []reportingutil.InputEvent
		var err error

		if params.IsUserEvent {
			// Look up user events using user account info.
			events, err = reportingutil.LookupUserEvents(ctx, reportingutil.ReportingServerURL, customerID, APIKey, "HEARTBEAT_EVENTS", acc.Username, testStartTime)
		} else {
			// Look up device events using client id.
			c, err := policyClient.ClientID(ctx, &empty.Empty{})
			clientID = c.ClientId
			if err != nil {
				s.Fatalf("Failed to grab client ID from device: %v:", err)
			}
			events, err = reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, customerID, clientID, APIKey, "HEARTBEAT_EVENTS", testStartTime)
		}
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
