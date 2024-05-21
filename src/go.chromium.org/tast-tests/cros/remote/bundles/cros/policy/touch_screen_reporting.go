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

const touchScreenReportingTimeout = 7 * time.Minute

type touchScreenReportingParameters struct {
	reportingEnabled  bool // test should expect reporting enabled
	touchScreenDevice bool // test being ran on a device with a touch screen
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         TouchScreenReporting,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "GAIA Enroll a device and verify touch screen info reporting",
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
			"group:enterprise-reporting"},
		SoftwareDeps: []string{"reboot", "chrome"},
		ServiceDeps:  []string{"tast.cros.policy.PolicyService", "tast.cros.hwsec.OwnershipService", "tast.cros.tape.Service", "tast.cros.graphics.ScreenshotService"},
		Timeout:      touchScreenReportingTimeout,
		Params: []testing.Param{
			{
				Name:              "non_touch_screen_reporting_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.NoTouchScreen()),
				Val: touchScreenReportingParameters{
					reportingEnabled:  true,
					touchScreenDevice: false,
				},
			}, {
				Name:              "touch_screen_reporting_enabled",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Val: touchScreenReportingParameters{
					reportingEnabled:  true,
					touchScreenDevice: true,
				},
			}, {
				Name:              "non_touch_screen_reporting_disabled",
				ExtraHardwareDeps: hwdep.D(hwdep.NoTouchScreen()),
				Val: touchScreenReportingParameters{
					reportingEnabled:  false,
					touchScreenDevice: false,
				},
			}, {
				Name:              "touch_screen_reporting_disabled",
				ExtraHardwareDeps: hwdep.D(hwdep.TouchScreen()),
				Val: touchScreenReportingParameters{
					reportingEnabled:  false,
					touchScreenDevice: true,
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

func touchScreenInfo(event reportingutil.InputEvent) *reportingutil.TouchScreenInfo {
	if w := event.WrappedEncryptedData; w != nil {
		if m := w.MetricData; m != nil {
			if i := m.InfoData; i != nil {
				if m := i.TouchScreenInfo; m != nil {
					return m
				}
			}
		}
	}
	return nil
}

func validateTouchScreenInfo(ctx context.Context, events []reportingutil.InputEvent, reportingEnabled, touchScreenDevice bool) error {
	if !reportingEnabled && len(events) > 0 {
		return errors.New("info found while reporting disabled at test touchScreenReporting")
	}
	if reportingEnabled && !touchScreenDevice && len(events) > 0 {
		return errors.New("info found when it is not a touch screen device")
	}
	if reportingEnabled && touchScreenDevice {
		if len(events) == 0 {
			return errors.New("no info found while reporting enabled at test touchScreenReporting")
		} else if len(events) > 1 {
			return errors.New("more than 1 info found while reporting enabled at test touchScreenReporting")
		} else if len(events) == 1 {
			touchScreenInfo := events[0].WrappedEncryptedData.MetricData.InfoData.TouchScreenInfo
			if touchScreenInfo.LibraryName == "" {
				return errors.New("no library name found on test touchScreenReporting")
			}
			if touchScreenInfo.TouchScreenDevices[0].DisplayName == "" {
				return errors.New("no device name found on test touchScreenReporting")
			}
			if touchScreenInfo.TouchScreenDevices[0].TouchPoints < 1 {
				return errors.New("incorrect touch points found on test touchScreenReporting")
			}
		}
	}
	return nil
}

func TouchScreenReporting(ctx context.Context, s *testing.State) {
	param := s.Param().(touchScreenReportingParameters)
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

	timeout := int32(touchScreenReportingTimeout.Seconds())
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
		SkipLogin:          true,
	}); err != nil {
		s.Fatal("Failed to enroll using chrome: ", err)
	}
	defer pc.StopChrome(ctx, &empty.Empty{})
	defer reportingutil.Deprovision(ctx, cl.Conn, sa)

	c, err := pc.ClientID(ctx, &empty.Empty{})
	if err != nil {
		s.Fatal("Failed to grab client ID from device: ", err)
	}

	// GoBigSleepLint: Info sent from the metric reporting manager won't be reported for the first minute.
	if err = testing.Sleep(ctx, 60*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	if err := testing.Poll(ctx, func(ctx context.Context) error {
		events, err := reportingutil.LookupEvents(ctx, acc.CustomerID, c.ClientId, APIKey, "INFO_METRIC", testStartTime)
		if err != nil {
			return testing.PollBreak(errors.Wrap(err, "failed to look up events"))
		}
		if err := reportingutil.SaveCrosboltEventCountMetric("info_events", len(events), s.OutDir()); err != nil {
			s.Log("Failed to save perf metric: ", err)
		}

		prunedEvents, err := reportingutil.PruneEvents(ctx, events, func(e reportingutil.InputEvent) bool {
			return touchScreenInfo(e) != nil
		})
		if err != nil {
			testing.PollBreak(errors.Wrap(err, "failed to prune events"))
		}
		if err := reportingutil.SaveCrosboltEventCountMetric("touch_events", len(prunedEvents), s.OutDir()); err != nil {
			s.Log("Failed to save pruned perf metric: ", err)
		}

		// Verify info.
		if err = validateTouchScreenInfo(ctx, prunedEvents, param.reportingEnabled, param.touchScreenDevice); err != nil {
			return testing.PollBreak(errors.Wrap(err, "invalid touch screen info"))
		}

		return nil
	}, &testing.PollOptions{
		Timeout:  6 * time.Minute,
		Interval: 3 * time.Minute,
	}); err != nil {
		s.Errorf("Failed to validate touchscreen info: %v:", err)
	}
}
