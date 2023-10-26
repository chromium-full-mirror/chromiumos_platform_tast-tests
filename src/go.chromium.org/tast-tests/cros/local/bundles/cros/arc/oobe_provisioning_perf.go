// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast-tests/cros/local/oobe"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testParamOobeProvisioningPerf struct {
	browserType browser.Type
	username    string
	password    string
	metric      string
}

type oobeMetrics struct {
	provisioningTime time.Duration
	appKills         *arc.AppKills
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         OobeProvisioningPerf,
		LacrosStatus: testing.LacrosVariantExists,
		Desc:         "Navigate through Play Store Out-Of-Box Experience (OOBE) and perform ARC provisioning. Report provisioning time similar to UMA case Arc.UiAvailable.OobeProvisioning.TimeDelta.Unmanaged",
		Contacts: []string{
			"arc-performance@google.com",
			"khmel@chromium.org", // Original author.
		},
		// ChromeOS > Software > ARC++ > Performance
		BugComponent: "b:168382",
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		SoftwareDeps: []string{"chrome", "chrome_internal"},
		// This test steps through opt-in flow 5 times and each iteration takes ~2 min.
		Timeout: 20 * time.Minute,
		Params: []testing.Param{{
			Name:              "managed",
			ExtraSoftwareDeps: []string{"android_container"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeAsh,
				username:    "arc.OobeProvisioningPerf.managed_username",
				password:    "arc.OobeProvisioningPerf.managed_password",
				metric:      "Managed",
			},
		}, {
			Name:              "managed_vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeAsh,
				username:    "arc.OobeProvisioningPerf.managed_username",
				password:    "arc.OobeProvisioningPerf.managed_password",
				metric:      "Managed",
			},
		}, {
			Name:              "unmanaged",
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_container"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeAsh,
				metric:      "Unmanaged",
			},
		}, {
			Name:              "unmanaged_lacros",
			ExtraSoftwareDeps: []string{"android_container", "lacros"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeLacros,
				metric:      "Unmanaged",
			},
		}, {
			Name:              "unmanaged_vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeAsh,
				metric:      "Unmanaged",
			},
		}, {
			Name:              "unmanaged_vm_lacros",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeLacros,
				metric:      "Unmanaged",
			},
		}},
		VarDeps: []string{
			"arc.OobeProvisioningPerf.managed_username",
			"arc.OobeProvisioningPerf.managed_password",
			"arc.perfAccountPool",
		},
	})
}

func OobeProvisioningPerf(ctx context.Context, s *testing.State) {
	const (
		// successBootCount is the number of passing ARC boots to collect results.
		successBootCount = 5
		// maxFailBootCount is the number of allowed failures.
		maxFailBootCount = 1
	)

	param := s.Param().(testParamOobeProvisioningPerf)
	var gaia chrome.Option
	if param.username != "" {
		gaia = chrome.GAIALogin(chrome.Creds{User: s.RequiredVar(param.username), Pass: s.RequiredVar(param.password)})
	} else {
		gaia = chrome.GAIALoginPool(s.RequiredVar("arc.perfAccountPool"))
	}

	failBootCount := 0
	var provisioningTimes []time.Duration
	var appKills []*arc.AppKills

	for len(provisioningTimes) < successBootCount {
		s.Logf("Running ARC provisioning iteration #%d out of %d",
			len(provisioningTimes)+1, successBootCount)

		oobeMetrics, err := oobeProvisioningPerfIteration(ctx, s, gaia, param.metric)
		if err != nil {
			failBootCount++
			s.Log("Error found during the ARC provisioning: ", err)

			if failBootCount > maxFailBootCount {
				s.Fatalf("Too many ARC provisioning errors (%d time(s)), last error: %q", failBootCount, err)
			}

			continue
		}

		provisioningTimes = append(provisioningTimes, oobeMetrics.provisioningTime)
		if oobeMetrics.appKills != nil {
			appKills = append(appKills, oobeMetrics.appKills)
		}
	}

	perfValues := perf.NewValues()

	for _, x := range provisioningTimes {
		perfValues.Append(perf.Metric{
			Name:      "provisioning_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, x.Seconds())
	}

	for _, appKill := range appKills {
		appKill.AppendPerfMetrics(perfValues, "")
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

func oobeProvisioningPerfIteration(ctx context.Context, s *testing.State, gaia chrome.Option, metric string) (*oobeMetrics, error) {
	histogramName := "Arc.UiAvailable.OobeProvisioning.TimeDelta." + metric

	var result oobeMetrics

	cr, err := chrome.New(ctx,
		chrome.DontSkipOOBEAfterLogin(),
		chrome.ARCSupported(),
		gaia)

	if err != nil {
		return &result, err
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return &result, err
	}

	ui := uiauto.New(tconn)
	if err := oobe.CompleteOnboardingFlow(ctx, ui); err != nil {
		return &result, err
	}

	testing.ContextLog(ctx, "OOBE is done. Waiting for provisioning metric")
	metricValue, err := metrics.WaitForHistogram(ctx, tconn, histogramName, 3*time.Minute)
	if err != nil {
		return &result, errors.Wrapf(err, "failed to get %s histogram", histogramName)
	}

	timeMs, err := metricValue.Mean()
	if err != nil {
		return &result, errors.Wrapf(err, "failed to read %s histogram", histogramName)
	}
	result.provisioningTime = time.Duration(timeMs * float64(time.Millisecond))

	result.appKills, err = arc.GetAppKills(ctx, tconn)
	if err != nil {
		testing.ContextLog(ctx, "Failed to collect ARC app kill counts: ", err)
	}

	return &result, nil
}
