// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package remotecommands

import (
	"context"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/policy/reportingutil"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const (
	fetchSupportPacketEnrollmentTimeout       = 5 * time.Minute
	fetchSupportPacketCommandExecutionTimeout = 10 * time.Minute
	fetchSupportPacketReportingTimeout        = 3 * time.Minute
	fetchSupportPacketCleanupTimeout          = 3 * time.Minute
	fetchSupportPacketTestTimeout             = fetchSupportPacketEnrollmentTimeout + fetchSupportPacketCommandExecutionTimeout + fetchSupportPacketReportingTimeout + fetchSupportPacketCleanupTimeout
)

type testingParam struct {
	skipLogin bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FetchSupportPacket,
		Desc: "Verifies that remote log upload with FETCH_SUPPORT_PACKET command works",
		Contacts: []string{
			"chromeos-commercial-supportability@google.com", // Team
			"iremuguz@google.com",                           // Test author
		},
		BugComponent: "b:1111615",
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:hw_agnostic",
		},
		SoftwareDeps: []string{"chrome", "vpd"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		VarDeps: []string{
			reportingutil.EventsAPIKeyPath,
			tape.ServiceAccountVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.LogUploadEnabled{}, pci.VerifiedFunctionalityOS),
		},
		Timeout: fetchSupportPacketTestTimeout,
		Params: []testing.Param{
			{
				Name: "logged_in_user_session",
				Val: testingParam{
					skipLogin: false,
				},
			},
			{
				Name: "login_screen",
				Val: testingParam{
					skipLogin: true,
				},
			},
		},
	})
}

func FetchSupportPacket(ctx context.Context, s *testing.State) {
	param := s.Param().(testingParam)
	APIKey := s.RequiredVar(reportingutil.EventsAPIKeyPath)
	sa := []byte(s.RequiredVar(tape.ServiceAccountVar))

	defer func(ctx context.Context) {
		if err := policyutil.EnsureTPMAndSystemStateAreReset(ctx, s.DUT(), s.RPCHint()); err != nil {
			s.Error("Failed to reset TPM after test: ", err)
		}
	}(ctx)

	if err := policyutil.EnsureTPMAndSystemStateAreResetRemote(ctx, s.DUT()); err != nil {
		s.Fatal("Failed to reset TPM: ", err)
	}

	cl, err := rpc.Dial(ctx, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to connect to the RPC service on the DUT: ", err)
	}
	defer cl.Close(ctx)

	pc := ps.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(fetchSupportPacketTestTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient /*lock=*/, true, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(ctx)

	systemLogUploadPolicy := &tape.LogUploadEnabledDevices{LogUploadEnabled: true}

	if err := tapeClient.SetPolicy(ctx, systemLogUploadPolicy, []string{"logUploadEnabled"}, nil, acc.RequestID); err != nil {
		s.Fatal("Failed to set the log upload enabled policy: ", err)
	}

	testStartTime := time.Now()

	// Defer deprovision before the enrollment in case enrollement fails after provisioning.
	defer reportingutil.Deprovision(ctx, cl.Conn, sa)
	if _, err := pc.GAIAEnrollForReporting(ctx, &ps.GAIAEnrollForReportingRequest{
		Username:           acc.Username,
		Password:           acc.Password,
		DmserverUrl:        policy.DMServerAlphaURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnabledFeatures:    "EncryptedReportingPipeline, OobeConsolidatedConsent, ClientAutomatedTest",
		SkipLogin:          param.skipLogin,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer pc.StopChrome(ctx, &empty.Empty{})

	policyClient := pspb.NewPolicyServiceClient(cl.Conn)
	deviceAndCustomerIDResponse, err := policyClient.DeviceAndCustomerID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to get device and customer id: ", err)
	}
	deviceID := deviceAndCustomerIDResponse.DeviceID
	customerID := deviceAndCustomerIDResponse.CustomerID

	s.Log("Supportability: Triggering FETCH_SUPPORT_PACKET remote command")
	command := tape.RemoteCommand{CommandType: tape.CommandTypeFetchSupportPacket, Payload: "{\"supportPacketDetails\":{\"issueCaseId\":\"issue_case_id\",\"issueDescription\":\"issuedescription\",\"requestedDataCollectors\":[1,2,3]}}"}
	response, err := tapeClient.IssueCommand(ctx, deviceID, customerID, command)
	if err != nil {
		s.Fatal("Failed to issue command: ", err)
	}

	s.Logf("Supportability: Command issued with response: %s. Waiting for the command result", response)

	commandID := response.CommandID

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		commandStatus, err := tapeClient.GetCommand(ctx, deviceID, customerID, commandID)
		if err != nil {
			return errors.Wrap(err, "failed to get command status")
		}
		if commandStatus.State == "EXECUTED_BY_CLIENT" {
			if commandStatus.CommandResult.Result != "SUCCESS" {
				s.Fatal("Capturing logs failed: ", commandStatus.CommandResult.ErrorMessage)
			}
			return nil
		}
		return errors.Errorf("Command state is %s", commandStatus.State)
	}, &testing.PollOptions{
		Interval: 10 * time.Second,
		Timeout:  fetchSupportPacketCommandExecutionTimeout}); err != nil {
		s.Fatal("Fetching logs failed: ", err)
	}

	s.Log("Supportability: Verifying LogUploadEvents on the reporting server")

	// Look up device events using client id.
	c, err := pc.ClientID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatalf("Failed to grab client ID from device: %v:", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		logUploadInputEvents, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, acc.CustomerID, c.ClientId, APIKey, "LOG_UPLOAD", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up log upload events")
		}
		if err := reportingutil.SaveCrosboltEventCountMetric("log_upload_events", len(logUploadInputEvents), s.OutDir()); err != nil {
			s.Log("Supportability: Failed to save log upload event metric: ", err)
		}

		// Verify events.
		for _, logUploadInputEvent := range logUploadInputEvents {
			logUploadEvent := logUploadInputEvent.WrappedEncryptedData.LogUploadEvent
			// We skip the check if the event is not relevant.
			if !isRelevantEvent(*logUploadEvent) {
				continue
			}
			//  Verify that the command ID is set correctly.
			if *logUploadEvent.CommandID != commandID {
				return errors.New("expected command ID not found")
			}
			// The upload shouldn't contain error.
			uploadTracker := logUploadEvent.UploadTracker
			if uploadTracker.Status != nil {
				return errors.Errorf("upload status contains an error: (%s) %s", *uploadTracker.Status.Code, *uploadTracker.Status.ErrorMessage)
			}
			// The access parameters will contain the location of uploaded file in the server. It can't be empty.
			if *uploadTracker.AccessParameters == "" {
				return errors.New("access parameters can't be empty")
			}
			// Verify that the uploaded file size matches the total file size.
			if err := verifyUploadedSize(uploadTracker); err != nil {
				return errors.Wrap(err, "can't verify the uploaded size")
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  fetchSupportPacketReportingTimeout,
		Interval: 1 * time.Minute,
	}); err != nil {
		s.Errorf("Failed to validate events: %v:", err)
	}
}

// isRelevantEvent checks if the log upload has finished on `event`. Only
// events that represent completed upload is relevant. The relevant event will
// contain either an access parameter for the uploaded log file or a status
// code that's related to the error.
func isRelevantEvent(event reportingutil.LogUploadEvent) bool {
	uploadTracker := event.UploadTracker
	if uploadTracker == nil {
		return false
	}
	if uploadTracker.Status == nil && uploadTracker.AccessParameters == nil {
		return false
	}
	return true
}

// verifyUploadedSize checks the total size and uploaded size in `tracker` and
// verifies the whole file is uploaded.
func verifyUploadedSize(uploadTracker *reportingutil.UploadTracker) error {
	total, err := strconv.ParseInt(*uploadTracker.Total, 10, 64)
	if err != nil {
		return errors.Wrap(err, "can't parse uploadTracker.total")
	}
	uploaded, err := strconv.ParseInt(*uploadTracker.Uploaded, 10, 64)
	if err != nil {
		return errors.Wrap(err, "can't parse uploadTracker.uploaded")
	}
	if total != uploaded {
		return errors.Errorf("total size (%d) doesn't match uploaded size (%d)", total, uploaded)
	}
	return nil
}
