// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package ui

import (
	"context"
	"time"

	uiperf "go.chromium.org/tast-tests/cros/local/bundles/cros/ui/perf"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/chrome/metrics"
	"go.chromium.org/tast-tests/cros/local/perfutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         OobePerf,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Test waits for out-of-box-experience (OOBE) Welcome screen and measures the time of the WebUI loading",
		Contacts: []string{
			"cros-oobe@google.com",
			"dkuzmin@google.com",
			"rrsilva@google.com",
			"bohdanty@google.com",
			"chromeos-sw-engprod@google.com",
		},
		BugComponent: "b:1263090", // ChromeOS > Software > OOBE
		Attr:         []string{"group:crosbolt", "crosbolt_perbuild"},
		VarDeps: []string{
			"ui.signinProfileTestExtensionManifestKey",
		},
		SoftwareDeps: []string{"chrome"},
        // TODO(b/286953538) - Reduce timeout once regression is fixed.
		Timeout:      8 * time.Minute,
	})
}

func OobePerf(ctx context.Context, s *testing.State) {
	const (
		histogramName = "OOBE.WebUI.LoadTime.FirstRun"
	)
	r := perfutil.NewRunner(nil, perfutil.RunnerOptions{IgnoreFirstRun: true, DropMinMaxValues: true})
	r.RunMultiple(ctx, "OobePerf", uiperf.Run(s, func(ctx context.Context, name string) ([]*metrics.Histogram, error) {
		// Load OOBE Welcome Screen (first OOBE screen). Test extension is required to fetch histograms.
		cr, err := chrome.New(ctx, chrome.NoLogin(), chrome.LoadSigninProfileExtension(s.RequiredVar("ui.signinProfileTestExtensionManifestKey")))
		if err != nil {
			s.Fatal("Failed to start Chrome: ", err)
		}

		defer cr.Close(ctx)

		tLoginConn, err := cr.SigninProfileTestAPIConn(ctx)
		if err != nil {
			return nil, errors.Wrap(err, "creating login test api connection failed")
		}
		// Wait for the WebUI load time histogram reported. 10 seconds should be enough even on the slowest boards. Making it 15 just in case.
        // TODO(b/286953538) - Reduce timeout once the regression is fixed.
		hist, err := metrics.WaitForHistogram(ctx, tLoginConn, histogramName, 45*time.Second)
		return []*metrics.Histogram{hist}, err
	}),
		perfutil.StoreAllWithHeuristics("Duration"))

	if err := r.Values().Save(ctx, s.OutDir()); err != nil {
		s.Fatal("Failed to save performance data in file: ", err)
	}
}
