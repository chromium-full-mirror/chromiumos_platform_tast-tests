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
	"go.chromium.org/tast-tests/cros/common/policy/reportingutil"
	"go.chromium.org/tast-tests/cros/common/tape"
	"go.chromium.org/tast-tests/cros/remote/policyutil"
	"go.chromium.org/tast-tests/cros/services/cros/graphics"
	ps "go.chromium.org/tast-tests/cros/services/cros/policy"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const privacyScreenReportingTimeout = 7 * time.Minute

type privacyScreenReportingParameters struct {
	reportingEnabled    bool // test should expect reporting enabled
	privacyScreenDevice bool // test being ran on a device with a privacy screen
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         PrivacyScreenReporting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a device and verify privacy screen info reporting",
		Contacts: []string{
			"cros-reporting-alerts+tast@google.com",
			"albertojuarez@google.com", // Test owner
		},
		BugComponent: "b:817866", // ChromeOS Server Projects > Enterprise Management > Reporting
		Attr: []string{
			"group:golden_tier",
			"group:medium_low_tier",
			"group:hardware",
			"group:complementary",
			"group:enterprise-reporting-daily",
			"group:enterprise-reporting"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service", "tast.cros.graphics.ScreenshotService"},
		Timeout:      privacyScreenReportingTimeout,
		Params: []testing.Param{
			{
				Name:              "non_privacy_screen_reporting_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.NoPrivacyScreen()),
				Val: privacyScreenReportingParameters{
					reportingEnabled:    true,
					privacyScreenDevice: false,
				},
			}, {
				Name:              "privacy_screen_reporting_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.PrivacyScreen()),
				Val: privacyScreenReportingParameters{
					reportingEnabled:    true,
					privacyScreenDevice: true,
				},
			},
			{
				Name:              "non_privacy_screen_reporting_disabled",
				ExtraHardwareDeps: hwdep.D(hwdep.NoPrivacyScreen()),
				Val: privacyScreenReportingParameters{
					reportingEnabled:    false,
					privacyScreenDevice: false,
				},
			}, {
				Name:              "privacy_screen_reporting_disabled",
				ExtraHardwareDeps: hwdep.D(hwdep.PrivacyScreen()),
				Val: privacyScreenReportingParameters{
					reportingEnabled:    false,
					privacyScreenDevice: true,
				},
			},
		},
		VarDeps: []string{
			reportingutil.EventsAPIKeyPath,
			tape.ServiceAccountVar,
		},
		SearchFlags: []*testing.StringPair{
			pci.SearchFlag(&policy.ReportDeviceGraphicsStatus{}, pci.VerifiedValue),
		},
	})
}

func privacyScreenInfo(event reportingutil.InputEvent) *reportingutil.PrivacyScreenInfo {
	if w := event.WrappedEncryptedData; w != nil {
		if m := w.MetricData; m != nil {
			if i := m.InfoData; i != nil {
				if m := i.PrivacyScreenInfo; m != nil {
					return m
				}
			}
		}
	}
	return nil
}

func validatePrivacyScreenInfo(ctx context.Context, events []reportingutil.InputEvent, reportingEnabled, privacyScreenDevice bool) error {
	if !reportingEnabled && len(events) > 0 {
		return errors.New("info found while reporting disabled at test privacyScreenReporting")
	} else if !reportingEnabled {
		return nil
	}

	if reportingEnabled {
		if len(events) == 0 {
			return errors.New("no info found while reporting enabled at test privacyScreenReporting")
		} else if len(events) > 1 {
			return errors.New("more than 1 info found while reporting enabled at test privacyScreenReporting")
		} else if len(events) == 1 {
			privacyScreenInfo := events[0].WrappedEncryptedData.MetricData.InfoData.PrivacyScreenInfo
			if privacyScreenInfo.Supported && !privacyScreenDevice {
				return errors.New("got supported info event in a non-privacy screen enabled device")
			}
			if !privacyScreenInfo.Supported && privacyScreenDevice {
				return errors.New("got not supported info event in a privacy screen enabled device")
			}
		}
	}
	return nil
}

// PrivacyScreenReporting tests reporting related to privacy screens.
func PrivacyScreenReporting(ctx context.Context, s *testing.State) {
	param := s.Param().(privacyScreenReportingParameters)
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

	screenshotService := graphics.NewScreenshotServiceClient(cl.Conn)
	captureScreenshotOnError := func(ctx context.Context, hasError func() bool) {
		if !hasError() {
			return
		}

		screenshotService.CaptureScreenshot(ctx, &graphics.CaptureScreenshotRequest{FilePrefix: "reportingError"})
	}
	defer captureScreenshotOnError(ctx, s.HasError)

	pc := ps.NewPolicyServiceClient(cl.Conn)

	tapeClient, err := tape.NewClient(ctx, []byte(s.RequiredVar(tape.ServiceAccountVar)))
	if err != nil {
		s.Fatal("Failed to create tape client: ", err)
	}

	timeout := int32(privacyScreenReportingTimeout.Seconds())
	// Create an account manager and lease a test account for the duration of the test.
	accManager, acc, err := tape.NewOwnedTestAccountManagerFromClient(ctx, tapeClient, true /*lock*/, tape.WithTimeout(timeout), tape.WithPoolID(tape.DefaultManaged))
	if err != nil {
		s.Fatal("Failed to create an account manager and lease an account: ", err)
	}
	defer accManager.CleanUp(ctx)

	// Enable or disable the policies depending on the param.
	var telemetryAllowlist []string
	if param.reportingEnabled {
		telemetryAllowlist = []string{"report_graphics_status"}
	} else {
		telemetryAllowlist = []string{}
	}

	if err := reportingutil.SetTelemetryPolicies(ctx, tapeClient, acc.RequestID, reportingutil.Custom, telemetryAllowlist, true); err != nil {
		s.Fatal("Failed to set the policy: ", err)
	}

	testStartTime := time.Now()
	if _, err := pc.GAIAEnrollForReporting(ctx, &ps.GAIAEnrollForReportingRequest{
		Username:           acc.Username,
		Password:           acc.Password,
		DmserverUrl:        policy.DMServerAlphaURL,
		ReportingServerUrl: reportingutil.ReportingServerURL,
		EnabledFeatures:    "EncryptedReportingPipeline, ClientAutomatedTest",
		SkipLogin:          false,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer pc.StopChrome(ctx, &empty.Empty{})
	defer reportingutil.Deprovision(ctx, cl.Conn, sa)

	c, err := pc.ClientID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to grab client ID from device: ", err)
	}

	pJSON, err := policy.MarshalList([]policy.Policy{
		&policy.ReportDeviceGraphicsStatus{Stat: policy.StatusSet, Val: param.reportingEnabled},
	})
	if err != nil {
		s.Fatal("Failed to marshall expected graphics policy for verification: ", err)
	}
	// Wait some time for the policy to propagate.
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		_, err := pc.VerifyPolicyStatus(ctx, &ps.VerifyPolicyStatusRequest{
			Policies: pJSON,
		})
		return err
	}, &testing.PollOptions{Timeout: 30 * time.Second}); err != nil {
		s.Error("Failed to verify graphics policy: ", err)
	}

	// GoBigSleepLint: Events sent from the metric reporting manager won't be reported for the first minute.
	if err = testing.Sleep(ctx, 60*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		events, err := reportingutil.LookupEvents(ctx, reportingutil.ReportingServerURL, acc.CustomerID, c.ClientId, APIKey, "INFO_METRIC", testStartTime)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to look up events"))
		}
		if err := reportingutil.SaveCrosboltEventCountMetric("info_events", len(events), s.OutDir()); err != nil {
			s.Log("Failed to save perf metric: ", err)
		}

		prunedEvents, err := reportingutil.PruneEvents(ctx, events, func(e reportingutil.InputEvent) bool {
			return privacyScreenInfo(e) != nil
		})
		if err != nil {
			testing.PollBreak(errors.Wrap(err, "failed to prune events"))
		}
		if err := reportingutil.SaveCrosboltEventCountMetric("privacy_events", len(prunedEvents), s.OutDir()); err != nil {
			s.Log("Failed to save pruned perf metric: ", err)
		}

		// Validate info.
		if err = validatePrivacyScreenInfo(ctx, prunedEvents, param.reportingEnabled, param.privacyScreenDevice); err != nil {
			return testing.PollBreak(errors.Wrap(err, "invalid privacy screen info"))
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  6 * time.Minute,
		Interval: 3 * time.Minute,
	}); err != nil {
		s.Errorf("Failed to validate privacy screen info: %v:", err)
	}
}
