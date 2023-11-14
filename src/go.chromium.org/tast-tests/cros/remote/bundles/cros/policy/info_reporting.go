// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package policy

import (
	"context"
	"time"

	"github.com/golang/protobuf/ptypes/empty"

	"go.chromium.org/tast-tests/cros/common/policy"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/remote/reportingutil"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const infoReportingTimeout = 8 * time.Minute

type infoReportingParameters struct {
	reportingEnabled bool // test should expect reporting enabled
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         InfoReporting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a device and verify info data on reporting servers",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test owner
		},
		BugComponent: "b:817866", // ChromeOS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:mainline", "informational", "group:enterprise-reporting-daily", "group:enterprise-reporting"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service", "tast.cros.graphics.ScreenshotService"},
		Timeout:      infoReportingTimeout,
		Params: []testing.Param{
			{
				Name: "enabled",
				Val: infoReportingParameters{
					reportingEnabled: true,
				},
			}, {
				Name: "disabled",
				Val: infoReportingParameters{
					reportingEnabled: false,
				},
			},
		},
		VarDeps: []string{
			reportingutil.EventsAPIKeyPath,
			tape.ServiceAccountVar,
		},
	})
}

func verifyInfo(event reportingutil.InputEvent, validator func(info *reportingutil.InfoData) bool) bool {
	if w := event.WrappedEncryptedData; w != nil {
		if m := w.MetricData; m != nil {
			if i := m.InfoData; i != nil {
				if validator(i) {
					return true
				}
			}
		}
	}
	return false
}

// InfoReporting tests the reporting of various info metrics.
func InfoReporting(ctx context.Context, s *testing.State) {
	param := s.Param().(infoReportingParameters)
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

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	captureScreenshotOnError := func(ctx context.Context, hasError func() bool) {
		if !hasError() {
			return
		}

		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "reportingError"})
	}
	defer captureScreenshotOnError(ctx, s.HasError)

	pc := pspb.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(infoReportingTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(ctx)

	// Enable or disable the policies depending on the param.
	var updatePolicy reportingutil.UpdatePolicy
	var infoAllowlist []string
	if param.reportingEnabled {
		updatePolicy = reportingutil.Custom
		infoAllowlist = []string{"report_graphics_status", "report_cpu_info", "report_network_status", "report_memory_info"}
	} else {
		updatePolicy = reportingutil.DisableAll
		infoAllowlist = []string{}
	}

	if err := reportingutil.SetTelemetryPolicies(ctx, tapeClient, acc.RequestID, updatePolicy, infoAllowlist, true); err != nil {
		s.Fatal("Failed to set the policy: ", err)
	}

	testStartTime := time.Now()
	if _, err := pc.GAIAEnrollForReporting(ctx, &pspb.GAIAEnrollForReportingRequest{
		Username:           acc.Username,
		Password:           acc.Password,
		DmserverUrl:        policy.DMServerAlphaURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnabledFeatures:    "EncryptedReportingPipeline, ClientAutomatedTest",
		SkipLogin:          true,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer pc.StopChrome(ctx, &empty.Empty{})
	defer reportingutil.Deprovision(ctx, cl.Conn, sa, acc.CustomerID)

	c, err := pc.ClientID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to grab client ID from device: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		infoEvents, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, acc.CustomerID, c.ClientId, APIKey, "INFO_METRIC", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up info events")
		}
		for _, internalParam := range []struct {
			// name is the sub-test name.
			name string
			// function to verify the event
			validator reportingutil.VerifyEventTypeCallback
		}{
			{
				name: "networkInfo",
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.NetworkInfo != nil
					})
				},
			},
			{
				name: "memoryInfo",
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.MemoryInfo != nil
					})
				},
			},
			{
				name: "cpuInfo",
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.CPUInfo != nil
					})
				},
			},
			{
				name: "displayInfo",
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.DisplayInfo != nil
					})
				},
			},
			{
				name: "privacyScreenInfo",
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.PrivacyScreenInfo != nil
					})
				},
			},
		} {
			testing.ContextLog(ctx, "running sub-test: ", internalParam.name, " - reportingEnabled: ", param.reportingEnabled)
			events := infoEvents
			prunedEvents, err := reportingutil.PruneEvents(ctx, events, func(e reportingutil.InputEvent) bool {
				return internalParam.validator(e)
			})
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to prune events in sub-test: "+internalParam.name))
			}
			if !param.reportingEnabled && len(prunedEvents) == 0 {
				testing.ContextLog(ctx, "succeeded verifying test - reporting disabled: ", internalParam.name)
			}
			if !param.reportingEnabled && len(prunedEvents) > 0 {
				return errors.Errorf("events found when reporting is disabled  %s with reportingEnabled set to %t", internalParam.name, param.reportingEnabled)
			}
			if param.reportingEnabled && len(prunedEvents) > 1 {
				return errors.Errorf("more than one event reporting at test %s with reportingEnabled set to %t", internalParam.name, param.reportingEnabled)
			}
			if param.reportingEnabled && len(prunedEvents) == 0 {
				return errors.Errorf("no events found while reporting enabled at test %s with reportingEnabled set to %t", internalParam.name, param.reportingEnabled)
			}
			if param.reportingEnabled {
				testing.ContextLog(ctx, "succeeded verifying test - reporting enabled: ", internalParam.name)
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout:  6 * time.Minute,
		Interval: 2 * time.Minute,
	}); err != nil {
		s.Errorf("Failed to validate info events: %v:", err)
	}
}
