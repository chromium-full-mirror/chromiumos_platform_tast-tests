// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/pvs/pvsutils"
	"go.chromium.org/tast/core/testing"
)

const dependsSkipTestPlan = "depends_skip_testplan.textproto"

func init() {
	testing.AddTest(&testing.Test{
		Func:         DependsSkip,
		Desc:         "Validates requirements skip as expected when dependencies pass and fail",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"jackgelinas@google.com",
		},
		Data:    []string{dependsSkipTestPlan},
		Attr:    []string{"group:pvs", "pvs_perbuild"},
		Timeout: 100 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// DependsSkip validates requirements skip as expected when dependencies pass
// and fail.
func DependsSkip(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	pvsRunner := pvsutils.PVSRunner{
		Dut:         dut,
		ContainerID: s.FixtValue().(string),
		Env: pvsutils.RuntimeEnv{
			ReuseTLEDir:         pvsutils.TmpReuseTLEDir,
			SimulatedDut:        true,
			SimulatedTestRunner: true,
			SimulatedTestsSkip:  "stub_Skip",
			SimulatedTestsError: "stub_Error",
		},
	}

	testplan := pvsutils.CopyToPVSOutputDir(
		ctx, s, s.DataPath(dependsSkipTestPlan),
	)

	pvsutils.EnsurePass(ctx, s, "list requirements", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passskip-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01",
			"test-fail-0001-v01",
			"test-fail-0002-v01",
			"test-err-0001-v01",
			"test-err-0002-v01",
			"test-skip-0001-v01",
			"test-skip-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 5},
			{Pattern: pvsutils.RequirementLevelMay, Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultNotRun), Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_Skip", pvsutils.PrintedResultNotRun), Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultNotRun), Count: 1},
			{Pattern: pvsutils.TestResultPattern("stub_Error", pvsutils.PrintedResultNotRun), Count: 1},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultNotRun), Count: 3},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 10},
			{Pattern: pvsutils.PrintedResultFail, Count: 0},
			{Pattern: pvsutils.PrintedResultPass, Count: 0},
		})
	})

	pvsutils.EnsurePass(ctx, s, "run tests", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passskip-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01",
			"test-fail-0001-v01",
			"test-fail-0002-v01",
			"test-err-0001-v01",
			"test-err-0002-v01",
			"test-skip-0001-v01",
			"test-skip-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 5},
			{Pattern: pvsutils.RequirementLevelMay, Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_Skip", pvsutils.PrintedResultNA), Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.TestResultPattern("stub_Error", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultNA), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 1},
			{Pattern: pvsutils.PrintedResultPass, Count: 2},
			{Pattern: pvsutils.PrintedResultNA, Count: 5},
			{Pattern: pvsutils.PrintedResultError, Count: 2},
			{Pattern: "reason: parent requirement ", Count: 3},
			{Pattern: "reason: simulated result", Count: 5},
		})
	})

	pvsutils.EnsurePass(ctx, s, "verify results", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passskip-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01",
			"test-fail-0001-v01",
			"test-fail-0002-v01",
			"test-err-0001-v01",
			"test-err-0002-v01",
			"test-skip-0001-v01",
			"test-skip-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 5},
			{Pattern: pvsutils.RequirementLevelMay, Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_Skip", pvsutils.PrintedResultNA), Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.TestResultPattern("stub_Error", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultNA), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 1},
			{Pattern: pvsutils.PrintedResultPass, Count: 2},
			{Pattern: pvsutils.PrintedResultError, Count: 2},
			{Pattern: pvsutils.PrintedResultNA, Count: 5},
		})
	})

	pvsutils.EnsurePass(ctx, s, "re-run tests", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-fail-0001-v01",
			"test-err-0001-v01",
			"test-err-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 1},
			{Pattern: pvsutils.RequirementLevelMay, Count: 2},
			{Pattern: "stub_PassServer:", Count: 0},
			{Pattern: "stub_Skip:", Count: 0},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.TestResultPattern("stub_Error", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 1},
			{Pattern: pvsutils.PrintedResultPass, Count: 0},
			{Pattern: pvsutils.PrintedResultError, Count: 2},
			{Pattern: pvsutils.PrintedResultNA, Count: 0},
		})
	})

	pvsutils.EnsurePass(ctx, s, "verify results after re-run", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passskip-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01",
			"test-fail-0001-v01",
			"test-fail-0002-v01",
			"test-err-0001-v01",
			"test-err-0002-v01",
			"test-skip-0001-v01",
			"test-skip-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 5},
			{Pattern: pvsutils.RequirementLevelMay, Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_Skip", pvsutils.PrintedResultNA), Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.TestResultPattern("stub_Error", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultNA), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 1},
			{Pattern: pvsutils.PrintedResultPass, Count: 2},
			{Pattern: pvsutils.PrintedResultError, Count: 2},
			{Pattern: pvsutils.PrintedResultNA, Count: 5},
		})
	})
}
