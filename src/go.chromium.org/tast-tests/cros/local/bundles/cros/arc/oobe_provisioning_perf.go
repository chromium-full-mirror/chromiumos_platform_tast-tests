// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/local/bundles/cros/arc/oobeutil"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/browser"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/chrome/uiauto"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type testParamOobeProvisioningPerf struct {
	browserType browser.Type
	username    string
	password    string
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
			},
		}, {
			Name:              "managed_vm",
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeAsh,
				username:    "arc.OobeProvisioningPerf.managed_username",
				password:    "arc.OobeProvisioningPerf.managed_password",
			},
		}, {
			Name:              "unmanaged",
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_container"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeAsh,
			},
		}, {
			Name:              "unmanaged_lacros",
			ExtraSoftwareDeps: []string{"android_container", "lacros"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeLacros,
			},
		}, {
			Name:              "unmanaged_vm",
			ExtraAttr:         []string{"crosbolt_arc_perf_qual"},
			ExtraSoftwareDeps: []string{"android_vm"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeAsh,
			},
		}, {
			Name:              "unmanaged_vm_lacros",
			ExtraSoftwareDeps: []string{"android_vm", "lacros"},
			Val: testParamOobeProvisioningPerf{
				browserType: browser.TypeLacros,
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

	for len(provisioningTimes) < successBootCount {
		s.Logf("Running ARC provisioning iteration #%d out of %d",
			len(provisioningTimes)+1, successBootCount)

		provisioningTime, err := oobeProvisioningPerfIteration(ctx, s, gaia)
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

	perfValues := perf.NewValues()

	for _, x := range provisioningTimes {
		perfValues.Append(perf.Metric{
			Name:      "provisioning_time",
			Unit:      "seconds",
			Direction: perf.SmallerIsBetter,
			Multiple:  true,
		}, x.Seconds())
	}

	if err := perfValues.Save(s.OutDir()); err != nil {
		s.Fatal("Failed saving perf data: ", err)
	}
}

func oobeProvisioningPerfIteration(ctx context.Context, s *testing.State, gaia chrome.Option) (time.Duration, error) {
	const histogramName = "Arc.UiAvailable.OobeProvisioning.TimeDelta.Unmanaged"

	cr, err := chrome.New(ctx,
		chrome.DontSkipOOBEAfterLogin(),
		chrome.ARCSupported(),
		gaia)

	if err != nil {
		return 0, err
	}
	defer cr.Close(ctx)

	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return 0, err
	}

	ui := uiauto.New(tconn)
	if err := oobeutil.CompleteOnboardingFlow(ctx, ui); err != nil {
		return 0, err
	}

	testing.ContextLog(ctx, "OOBE is done. Waiting for provisioning metric")
	metric, err := metrics.WaitForHistogram(ctx, tconn, histogramName, 3*time.Minute)
	if err != nil {
		return 0, errors.Wrapf(err, "failed to get %s histogram", histogramName)
	}

	timeMs, err := metric.Mean()
	if err != nil {
		return 0, errors.Wrapf(err, "failed to read %s histogram", histogramName)
	}

	return time.Duration(timeMs * float64(time.Millisecond)), nil
}
