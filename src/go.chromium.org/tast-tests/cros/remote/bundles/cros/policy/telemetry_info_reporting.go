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
	pspb "go.chromium.org/tast-tests/cros/services/cros/policy"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

const telemetryInfoReportingTimeout = 15 * time.Minute

type telemetryInfoReportingParameters struct {
	reportingEnabled bool // test should expect reporting enabled
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TelemetryInfoReporting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a device and verify telemetry and info data on reporting servers",
		Contacts: []string{
			"cros-reporting-team@google.com",
			"albertojuarez@google.com", // Test owner
		},
		BugComponent: "b:817866", // ChromeOS Server Projects > Enterprise Management > Reporting
		Attr:         []string{"group:enterprise-reporting-daily", "group:enterprise-reporting"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service", "tast.cros.graphics.ScreenshotService"},
		Timeout:      telemetryInfoReportingTimeout,
		Params: []testing.Param{
			{
				Name: "enabled",
				Val: telemetryInfoReportingParameters{
					reportingEnabled: true,
				},
			}, {
				Name: "disabled",
				Val: telemetryInfoReportingParameters{
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

type testType int

const (
	info testType = iota
	telemetry
)

func verifyTelemetry(event reportingutil.InputEvent, validator func(telemetry *reportingutil.TelemetryData) bool) bool {
	if w := event.WrappedEncryptedData; w != nil {
		if m := w.MetricData; m != nil {
			if i := m.TelemetryData; i != nil {
				if validator(i) {
					return true
				}
			}
		}
	}
	return false
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

// TelemetryInfoReporting tests the reporting of various info and telemetry metrics.
func TelemetryInfoReporting(ctx context.Context, s *testing.State) {
	param := s.Param().(telemetryInfoReportingParameters)
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

	timeout := int32(telemetryInfoReportingTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(ctx)

	// Enable or disable the policies depending on the param.
	var updatePolicy reportingutil.UpdatePolicy
	if param.reportingEnabled {
		updatePolicy = reportingutil.EnableAll
	} else {
		updatePolicy = reportingutil.DisableAll
	}

	if err := reportingutil.SetTelemetryPolicies(ctx, tapeClient, acc.RequestID, updatePolicy, []string{}, true); err != nil {
		s.Fatal("Failed to set the policy: ", err)
	}

	testStartTime := time.Now()
	if _, err := pc.GAIAEnrollForReporting(ctx, &pspb.GAIAEnrollForReportingRequest{
		Username:           acc.Username,
		Password:           acc.Password,
		DmserverUrl:        reportingutil.DmServerURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnabledFeatures:    "EncryptedReportingPipeline, EnableTelemetryTestingRates",
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

	// The info reporting normally takes a couple minutes to be reported but the
	// telemetry is reported every few hours if not using the
	// "EnableTelemetryTestingRates" feature enabled above which reports it
	// in 4-5 minutes.
	if err := reportingutil.SleepWithContextLog(ctx, 5); err != nil {
		s.Fatal("Failed to sleep and log into context: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		telemetryEvents, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, acc.CustomerID, c.ClientId, APIKey, "TELEMETRY_METRIC", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up telemetry events")
		}

		infoEvents, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, acc.CustomerID, c.ClientId, APIKey, "INFO_METRIC", testStartTime)
		if err != nil {
			return errors.Wrap(err, "failed to look up info events")
		}
		for _, internalParam := range []struct {
			// name is the subtest name.
			name string
			// enum to know if telemetry or info
			testType testType
			// function to verify the event
			validator reportingutil.VerifyEventTypeCallback
		}{
			{
				name:     "audioTelemetry",
				testType: telemetry,
				validator: func(event reportingutil.InputEvent) bool {
					return verifyTelemetry(event, func(telemetry *reportingutil.TelemetryData) bool {
						return telemetry.AudioTelemetry != nil
					})
				},
			},
			{
				name:     "networksTelemetry-httpsLatencyData",
				testType: telemetry,
				validator: func(event reportingutil.InputEvent) bool {
					return verifyTelemetry(event, func(telemetry *reportingutil.TelemetryData) bool {
						return telemetry.NetworksTelemetry != nil && telemetry.NetworksTelemetry.HTTPSLatencyData != nil
					})
				},
			},
			{
				name:     "networksTelemetry-networkTelemetry",
				testType: telemetry,
				validator: func(event reportingutil.InputEvent) bool {
					return verifyTelemetry(event, func(telemetry *reportingutil.TelemetryData) bool {
						return telemetry.NetworksTelemetry != nil && telemetry.NetworksTelemetry.NetworkTelemetry != nil
					})
				},
			},
			{
				name:     "displaysTelemetry",
				testType: telemetry,
				validator: func(event reportingutil.InputEvent) bool {
					return verifyTelemetry(event, func(telemetry *reportingutil.TelemetryData) bool {
						return telemetry.DisplaysTelemetry != nil
					})
				},
			},
			{
				name:     "networkInfo",
				testType: info,
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.NetworkInfo != nil
					})
				},
			},
			{
				name:     "memoryInfo",
				testType: info,
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.MemoryInfo != nil
					})
				},
			},
			{
				name:     "cpuInfo",
				testType: info,
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.CPUInfo != nil
					})
				},
			},
			{
				name:     "displayInfo",
				testType: info,
				validator: func(event reportingutil.InputEvent) bool {
					return verifyInfo(event, func(info *reportingutil.InfoData) bool {
						return info.DisplayInfo != nil
					})
				},
			},
		} {
			testing.ContextLog(ctx, "running sub-test: ", internalParam.name, " - reportingEnabled: ", param.reportingEnabled)
			events := telemetryEvents
			if internalParam.testType == info {
				events = infoEvents
			}
			prunedEvents, err := reportingutil.PruneEvents(ctx, events, func(e reportingutil.InputEvent) bool {
				return internalParam.validator(e)
			})
			if err != nil {
				return testing.PollBreak(errors.Wrap(err, "failed to prune events"))
			}
			if !param.reportingEnabled && len(prunedEvents) == 0 {
				testing.ContextLog(ctx, "succeeded verifying test - reporting disabled: ", internalParam.name)
			}
			if !param.reportingEnabled && len(prunedEvents) > 0 {
				return errors.Errorf("events found when reporting is disabled  %s with reportingEnabled set to %t", internalParam.name, param.reportingEnabled)
			}
			if param.reportingEnabled && internalParam.testType == telemetry && len(prunedEvents) > 3 {
				return errors.Errorf("more than one event reporting at test %s with reportingEnabled set to %t", internalParam.name, param.reportingEnabled)
			}
			if param.reportingEnabled && internalParam.testType == info && len(prunedEvents) > 1 {
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
		Timeout:  10 * time.Minute,
		Interval: 5 * time.Minute,
	}); err != nil {
		s.Errorf("Failed to validate telemetry and info events: %v:", err)
	}
}
