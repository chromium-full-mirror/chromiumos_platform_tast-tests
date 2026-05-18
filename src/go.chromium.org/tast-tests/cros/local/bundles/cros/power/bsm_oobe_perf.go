// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast-tests/cros/common/dma"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/ui"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto/faillog"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast-tests/cros/local/power"
	"go.chromium.org/tast-tests/cros/local/power/setup"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testParamBsmOobePerf struct {
	username  string
	password  string
	metric    string
	enableBsm bool
}

func init() {
	// TODO(b/418724317): Remove from lab after BSM slows down login is fixed.
	testing.AddTest(&testing.Test{
		Func: BsmOobePerf,
		Desc: "Navigate through Play Store Out-Of-Box Experience (OOBE) and record provisioning time and resource usage under battery saver mode",
		Contacts: []string{
			"chromeos-power-team@google.com",
			"zactu@google.com",
		},
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Core Power
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		Fixture:      setup.PowerOobe,
		// This test steps through opt-in flow 5 times and each iteration takes ~2 min.
		Timeout: 20 * time.Minute,
		Params: []testing.Param{{
			Name:              "managed_vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamBsmOobePerf{
				username: "power.BsmOobePerf.managed_username",
				password: "power.BsmOobePerf.managed_password",
				metric:   "Managed",
			},
		}, {
			Name:              "unmanaged_vm",
			ExtraAttr:         []string{"group:power", "power_daily", "power_weekly"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamBsmOobePerf{
				metric: "Unmanaged",
			},
		}, {
			Name:              "managed_vm_bsm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamBsmOobePerf{
				username:  "power.BsmOobePerf.managed_username",
				password:  "power.BsmOobePerf.managed_password",
				metric:    "Managed",
				enableBsm: true,
			},
		}, {
			Name:              "unmanaged_vm_bsm",
			ExtraAttr:         []string{"group:power", "power_daily", "power_weekly"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamBsmOobePerf{
				metric:    "Unmanaged",
				enableBsm: true,
			},
		}},
		VarDeps: []string{
			"power.BsmOobePerf.managed_username",
			"power.BsmOobePerf.managed_password",
		},
	})
}

func BsmOobePerf(ctx context.Context, s *testing.State) {
	const (
		// successBootCount is the number of passing ARC boots to collect results.
		successBootCount = 5
		// maxFailBootCount is the number of allowed failures.
		maxFailBootCount = 1
	)

	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Second)
	defer cancel()

	// Retrieve chrome login credentials.
	param := s.Param().(testParamBsmOobePerf)
	var gaia chrome.Option
	if param.username != "" {
		gaia = chrome.GAIALogin(chrome.Creds{User: s.RequiredVar(param.username), Pass: s.RequiredVar(param.password)})
	} else {
		gaiaCreds, err := credconfig.PickRandomCreds(dma.CredsFromPool(ui.GaiaPoolDefaultVarName))
		if err != nil {
			s.Fatal("Failed to parse creds: ", err)
		}
		gaia = chrome.GAIALogin(gaiaCreds)
	}

	// Using the power recorder to profile the resource usage of OOBE. The power
	// consumption is not the focus in this test.
	r := power.NewRecorder(ctx, 5*time.Second, s.OutDir(), s.TestName())
	defer r.Close(cleanupCtx)

	if err := r.Start(ctx); err != nil {
		s.Fatal("Cannot start collecting power metrics: ", err)
	}

	// Perform OOBE provisioning repeatedly.
	failBootCount := 0
	var provisioningTimes []time.Duration

	for len(provisioningTimes) < successBootCount {
		s.Logf("Running ARC provisioning iteration #%d out of %d",
			len(provisioningTimes)+1, successBootCount)

		provisioningTime, err := oobeProvisioningPerfIteration(ctx, s, gaia, param.metric, param.enableBsm, r, failBootCount+len(provisioningTimes)+1)

		if err != nil {
			failBootCount++
			s.Log("Error found during the ARC provisioning: ", err)
			if failBootCount > maxFailBootCount {
				s.Fatalf("Too many ARC provisioning errors (%d time(s)), last error: %q", failBootCount, err)
			}
			continue
		}

		provisioningTimes = append(provisioningTimes, provisioningTime)
	}

	// Saving results.
	provisioningTimePerfVals := perf.NewValues()

	for _, x := range provisioningTimes {
		provisioningTimePerfVals.Append(perf.Metric{
			Name:      "provisioning_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, x.Seconds())
	}

	if err := r.Finish(ctx, provisioningTimePerfVals); err != nil {
		s.Error("Cannot finish collecting power metrics: ", err)
	}
}

func oobeProvisioningPerfIteration(ctx context.Context, s *testing.State, gaia chrome.Option, metric string, enableBsm bool, r *power.Recorder, iteration int) (time.Duration, error) {
	histogramName := "Arc.UiAvailable.OobeProvisioning.TimeDelta." + metric

	var bsmFeatures []string
	if enableBsm {
		bsmFeatures = []string{"CrosBatterySaver", "CrosBatterySaverAlwaysOn"}
	}

	// Signing in.
	testing.ContextLog(ctx, "Sign-in started")
	cr, err := chrome.New(ctx,
		chrome.DontSkipOOBEAfterLogin(),
		chrome.ARCSupported(),
		chrome.EnableFeatures(bsmFeatures...),
		gaia)

	if err != nil {
		return time.Duration(0), err
	}
	defer cr.Close(ctx)
	defer cr.ResetState(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return time.Duration(0), err
	}

	ui := uiauto.New(tconn)

	// Perform OOBE provisioning.
	testing.ContextLog(ctx, "OOBE provisioning started")
	err = oobe.CompleteOnboardingFlow(ctx, ui)

	if err != nil {
		faillog.DumpUITree(ctx, s.OutDir(), tconn)
		return time.Duration(0), err
	}

	testing.ContextLog(ctx, "Waiting for provisioning metric")
	metricValue, err := metrics.WaitForHistogram(ctx, tconn, histogramName, 3*time.Minute)
	if err != nil {
		return time.Duration(0), errors.Wrapf(err, "failed to get %s histogram", histogramName)
	}

	testing.ContextLog(ctx, "Idling for 30 seconds")
	// GoBigSleepLint: record resource usage post provision.
	if err := testing.Sleep(ctx, 30*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	timeMs, err := metricValue.Mean()
	if err != nil {
		return time.Duration(0), errors.Wrapf(err, "failed to read %s histogram", histogramName)
	}

	return time.Duration(timeMs * float64(time.Millisecond)), nil
}
