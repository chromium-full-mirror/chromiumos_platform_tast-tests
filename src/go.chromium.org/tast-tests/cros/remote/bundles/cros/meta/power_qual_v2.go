// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package meta

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/meta/tastrun"
	"go.chromium.org/tast-tests/cros/remote/power"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         PowerQualV2,
		Desc:         "Run power test cases based on the given configuration",
		LacrosStatus: testing.LacrosVariantUnneeded,
		BugComponent: "b:1361410", // ChromeOS > Platform > System > Power
		Contacts:     []string{"chromeos-platform-power@google.com"},
		Vars: []string{
			// Optional. If given, it overrides the default URL.
			"meta.PowerQualV2.configURL",
		},
		ServiceDeps: []string{"tast.common.power.powerpb.LocalInfoService"},
		// Use a big enough timeout value for the tests. Actual test context timeout value will
		// be adjusted based on the configuration.
		Timeout: 24 * time.Hour,
		Params: []testing.Param{
			// For each pramamerized test, the param Val will be the URL of the configuration.
			{
				// Provide an unnamed subtest intentionally so the test can be triggered by giving the
				// test name directly without providing a test parameter.
				Val: "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/example.json",
			}, {
				Name: "videoplayback_ash_test",
				Val:  "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/videoplayback_ash_test.json",
			},
			{
				Name: "videoplayback_lacros_test",
				Val:  "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/videoplayback_lacros_test.json",
			},
			{
				Name:              "qual",
				Val:               "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/qual.json",
				ExtraRequirements: []string{"pwr-batLife-0009-v03", "pwr-batLife-0010-v03", "pwr-batLife-0011-v03", "pwr-batLife-0012-v01"},
			},
			{
				Name:              "browsingheavy",
				Val:               "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/browsing_heavy.json",
				ExtraRequirements: []string{"pwr-batLife-0009-v03"},
			},
			{
				Name:              "videoplayback",
				Val:               "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/video_playback.json",
				ExtraRequirements: []string{"pwr-batLife-0010-v03"},
			},
			{
				Name:              "videocall",
				Val:               "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/video_call.json",
				ExtraRequirements: []string{"pwr-batLife-0011-v03"},
			},
			{
				Name:              "browsing",
				Val:               "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/browsing.json",
				ExtraRequirements: []string{"pwr-batLife-0012-v01"},
			},
			{
				Name: "short",
				Val:  "https://storage.googleapis.com/chromiumos-test-assets-public/tast/cros/power/powerqual-config/short.json",
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

	run, err := power.NewQualRun(ctx, configURL, s.DUT(), s.RPCHint())
	if err != nil {
		s.Fatal("Failed to create new power qual run: ", err)
	}
	config := run.Config
	control := config.Control

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
	// Define test groups. Tests in one group don't have to run in fixed order,
	// but the groups must be executed in sequence.
	var testGroups [][]string
	for _, t := range run.OrderedTests {
		// Each ordered test will be run as a single test group.
		testGroups = append(testGroups, []string{t})
	}
	if len(run.UnorderedTests) != 0 {
		// UnorderedTests will be put into a single group and run together.
		testGroups = append(testGroups, run.UnorderedTests)
	}

	for i, tests := range testGroups {
		resultsDir := filepath.Join(s.OutDir(), fmt.Sprintf("power_qual_tests_%d", i))
		s.Logf("Start to run test(s) %v and save results in %s", tests, resultsDir)
		skippedTests, testErrs := runAndEvaluate(runCtx, s, flags, tests, resultsDir, skipPolicy)

		if len(testErrs) != 0 {
			s.Log("Current test group has the following errors:")
			for index, testErr := range testErrs {
				s.Logf("Error %d: %v", index, testErr)
			}
			s.Log("Continue to run the following test group")
			continue
		}
		if err := run.AddTestResults(ctx, tests, skippedTests, resultsDir); err != nil {
			s.Log("Failed to add test results: ", err)
		}
	}

	if err := run.GenerateReport(ctx, s.OutDir(), s.TestName()); err != nil {
		s.Log("Failed to generate power qual test results: ", err)
	}
}

// runAndEvaluate runs tests and checks the results, propagating any test failure.
// It returns (the names of) any skipped tests and test executation or result parsing errors.
func runAndEvaluate(ctx context.Context, s *testing.State, flags, patterns []string, resultsDir string, policy tastrun.SkipPolicy) ([]string, []error) {
	var testErrs []error
	// Run tests.
	flags = append(flags, "-resultsdir="+resultsDir)
	if stdout, _, err := tastrun.Exec(ctx, s, "run", flags, patterns); err != nil {
		lines := strings.Split(strings.TrimSpace(string(stdout)), "\n")
		tasterr := errors.Errorf("failed to run tast: %v (last line: %q", err, lines[len(lines)-1])
		testErrs = append(testErrs, tasterr)
	}

	// Evaluate results.
	results, err := tastrun.ParseResultsJSON(resultsDir)
	if err != nil {
		parsingErr := errors.Wrap(err, "failed to parse test results")
		testErrs = append(testErrs, parsingErr)
	}

	var skippedTests []string
	for _, result := range results {
		if len(result.Errors) != 0 {
			testErr := errors.Errorf("Test %s failed: %v", result.Name, result.Errors)
			testErrs = append(testErrs, testErr)
		}
		if result.SkipReason != "" {
			s.Logf("Test %s was skipped: %s", result.Name, result.SkipReason)
			skippedTests = append(skippedTests, result.Name)
		}
	}
	if len(skippedTests) > 0 && policy == tastrun.SkipPolicyDisallowSkipping {
		s.Errorf("Skipped %d test(s) in total: %v", len(skippedTests), skippedTests)
	}
	return skippedTests, testErrs
}
