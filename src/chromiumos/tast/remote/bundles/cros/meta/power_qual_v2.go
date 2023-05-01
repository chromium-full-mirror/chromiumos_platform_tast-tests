// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"fmt"
	"path/filepath"
	"time"

	"chromiumos/tast/remote/bundles/cros/meta/tastrun"
	"chromiumos/tast/remote/power"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerQualV2,
		Desc:         "Run power test cases based on the given configuration",
		LacrosStatus: testing.LacrosVariantUnneeded,
		BugComponent: "b:167191", // ChromeOS > Platform > System > Power
		Contacts:     []string{"chromeos-platform-power@google.com"},
		Vars: []string{
			// Optional. If given, it overrides the default URL.
			"meta.PowerQualV2.configURL",
		},
		// Use a big enough timeout value for the tests. Actual test context timeout value will
		// be adjusted based on the configuration.
		Timeout: 24 * time.Hour,
		Params: []testing.Param{
			// For each pramamerized test, the param Val will be the URL of the configuration.
			{
				// Provide an unnamed subtest intentionally so the test can be triggered by giving the
				// test name directly without providing a test parameter.
				Val: "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/example.json",
			},
			// TODO(b/274972858): add other parameterized tests.
			// For example, add "essential" and "advanced" tests that use different configurations.
		},
	})
}

func PowerQualV2(ctx context.Context, s *testing.State) {
	configURL := s.Param().(string)

	if v, ok := s.Var("meta.PowerQualV2.configURL"); ok {
		// Override the default configuration file URL.
		configURL = v
	}

	run, err := power.NewQualRun(ctx, configURL)
	if err != nil {
		s.Fatal("Failed to create new power qual run: ", err)
	}
	config := run.Config
	control := config.Control

	resultsDir := filepath.Join(s.OutDir(), "power_qual_tests")
	flags := []string{fmt.Sprintf("-retries=%d", control.Retry)}
	var skipPolicy tastrun.SkipPolicy = tastrun.SkipPolicyAllowSkipping
	if control.FailOnSkippedTest {
		skipPolicy = tastrun.SkipPolicyDisallowSkipping
	}
	runCtx := ctx
	if control.MaxDuration > 0 {
		var cancel context.CancelFunc
		runCtx, cancel = context.WithTimeout(ctx, time.Duration(control.MaxDuration)*time.Minute)
		defer cancel()
	}

	s.Logf("Start power qual test: %s (version %s)", config.Name, config.Version)
	skippedTests := tastrun.RunAndEvaluate(runCtx, s, flags, run.Tests, resultsDir, skipPolicy)

	// RunAndEvaluate propagates any test errors to the testing state s.
	// Check if there are errors before generating report.
	if s.HasError() {
		// Just return. The propagated errors will be logged and test will fail.
		return
	}
	if err := run.GenerateReport(ctx, skippedTests, resultsDir, s.OutDir()); err != nil {
		s.Fatal("Failed to generate power qual test results: ", err)
	}
}
