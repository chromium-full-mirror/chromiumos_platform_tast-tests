// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentd tests security event reporting.
package secagentd

import (
	"context"
	"strconv"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/pci"
	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/reportingutil"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

type xdrReportingParameters struct {
	reportingEnabled bool
}

const xdrReportingTimeout = 7 * time.Minute

func init() {
	testing.AddTest(&testing.Test{
		Func: XdrReporting,
		Desc: "Sends and verifies that all xdr messages are received",
		Contacts: []string{
			"cros-enterprise-security@google.com",
			"aashay@google.com",
			"jasonling@google.com",
			"rborzello@google.com",
		},
		// ChromeOS > Security > ChromeOS Enterprise Security
		BugComponent: "b:1208373",
		Attr:         []string{"group:mainline", "informational", "group:dmserver-enrollment-daily", "group:enterprise-reporting", "group:hw_agnostic"},
		Timeout:      xdrReportingTimeout,
		SoftwareDeps: []string{"bpf", "reboot", "chrome"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service", "tast.cros.graphics.ScreenshotService"},
		Params: []testing.Param{
			{
				Name: "xdr_reporting_enabled",
				Val: xdrReportingParameters{
					reportingEnabled: true,
				},
			}, {
				Name: "xdr_reporting_disabled",
				Val: xdrReportingParameters{
					reportingEnabled: false,
				},
			},
		},
		VarDeps: []string{
			reportingutil.ManagedChromeCustomerIDPath,
			reportingutil.EventsAPIKeyPath,
			tape.ServiceAccountVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.DeviceReportXDREvents{}, pci.VerifiedValue),
		},
	})
}

// XdrReporting verifies all xdr events are sent and received.
func XdrReporting(ctx context.Context, s *testing.State) {
	param := s.Param().(xdrReportingParameters)
	customerID := s.RequiredVar(reportingutil.ManagedChromeCustomerIDPath)
	APIKey := s.RequiredVar(reportingutil.EventsAPIKeyPath)
	sa := []byte(s.RequiredVar(tape.ServiceAccountVar))

	_, err := s.DUT().Conn().CommandContext(ctx, "echo", "'clobber'", ">", "/mnt/stateful_partition/.update_available",
		"&&", "clear_tpm_owner_request=1").Output()
	if err != nil {
		s.Fatal("Failed to clear device ownership")
	}

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
	defer reportingutil.Deprovision(ctx, cl.Conn, sa, customerID)

	pc := ps.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(xdrReportingTimeout.Seconds())
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

	xdrPolicy := &tape.DeviceReportXdrEventsDevices{
		DeviceReportXdrEvents: param.reportingEnabled,
	}

	// API call to set xdr reporting policy.
	if err := tapeClient.SetPolicy(ctx, xdrPolicy, []string{"deviceReportXdrEvents"}, nil, acc.RequestID); err != nil {
		s.Fatal("Failed to set the policy: ", err)
	}

	// Enroll the device with the tape account and login. Login is
	// unfortunately required for the VerifyPolicyStatus call below because it
	// relies on a chrome extension.
	if _, err := pc.GAIAEnrollForReporting(ctx, &ps.GAIAEnrollForReportingRequest{
		Username:           acc.Username,
		Password:           acc.Password,
		DmserverUrl:        reportingutil.DmServerURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnabledFeatures:    "EncryptedReportingPipeline, ClientAutomatedTest",
		SkipLogin:          false,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer pc.StopChrome(ctx, &empty.Empty{})

	c, err := pc.ClientID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to grab client ID from device: ", err)
	}
	s.Logf("ClientID: %s", c.ClientId)

	pJSON, err := policy.MarshalList([]policy.Policy{
		&policy.DeviceReportXDREvents{Stat: policy.StatusSet, Val: param.reportingEnabled},
	})
	if err != nil {
		s.Fatal("Failed to marshall expected XDR policy for verification: ", err)
	}
	// Wait some time for the policy to propagate.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := pc.VerifyPolicyStatus(ctx, &ps.VerifyPolicyStatusRequest{
			Policies: pJSON,
		})
		return err
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Error("Failed to verify XDR policy: ", err)
	}

	// Stop secagentd from emitting events until test starts.
	_, err = s.DUT().Conn().CommandContext(ctx, "stop", "secagentd").Output()
	if err != nil {
		s.Fatal("Failed to stop secagentd")
	}

	testStartTime := time.Now()

	// Restart secagentd and set the heartbeat and batch interval to 5 seconds.
	_, err = s.DUT().Conn().CommandContext(ctx, "start", "secagentd",
		"SET_HEARTBEAT_PERIOD_S_FOR_TESTING=5", "PLUGIN_BATCH_INTERVAL_S_FOR_TESTING=5").Output()
	if err != nil {
		s.Fatal("Failed to start secagentd")
	}

	// Agent Events.
	var agentStartTime int64 = 0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		events, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, customerID, c.ClientId, APIKey, "CROS_SECURITY_AGENT", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up agent events")
		}
		err = reportingutil.ChronoSortEvents(events)
		if err != nil {
			return errors.Wrap(err, "failed to sort events")
		}
		if !param.reportingEnabled && len(events) > 0 {
			return testing.PollBreak(errors.New("agent events found when reporting is disabled"))
		}
		if param.reportingEnabled {
			if len(events) < 1 {
				return errors.New("no agent event found")
			} else if len(events) == 1 {
				return errors.New("no heartbeat events found")
			}
			agentStartTime, err = strconv.ParseInt(events[0].APIEvent.ReportingRecordEvent.Time, 0, 64)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  80 * time.Second,
		Interval: 20 * time.Second,
	}); err != nil {
		s.Errorf("Failed to validate agent events: %v:", err)
	}

	// Process Events.
	var firstProcessTime int64 = 0
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		events, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, customerID, c.ClientId, APIKey, "CROS_SECURITY_PROCESS", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up process events")
		}
		err = reportingutil.ChronoSortEvents(events)
		if err != nil {
			return errors.Wrap(err, "failed to sort events")
		}
		if !param.reportingEnabled && len(events) > 0 {
			return testing.PollBreak(errors.New("process events found when reporting is disabled"))
		}
		if param.reportingEnabled {
			if len(events) < 1 {
				return errors.New("no process event found")
			}
			firstProcessTime, err = strconv.ParseInt(events[0].APIEvent.ReportingRecordEvent.Time, 0, 64)
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  40 * time.Second,
		Interval: 20 * time.Second,
	}); err != nil {
		s.Errorf("Failed to validate process events: %v:", err)
	}

	// Verify that process events are only sent after agent.
	s.Logf("Test start time: %d", testStartTime.UnixMicro())
	s.Logf("Agent start time: %d", agentStartTime)
	s.Logf("First process time: %d", firstProcessTime)
	if agentStartTime > firstProcessTime {
		s.Fatal("Process Events without Agent Start")
	}
}
