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

const basicPassFailTestPlan = "basic_pass_fail_testplan.textproto"

func init() {
	testing.AddTest(&testing.Test{
		Func:         BasicPassFail,
		Desc:         "Validates the simple case of passing and failing tests",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"jackgelinas@google.com",
		},
		Data:    []string{basicPassFailTestPlan},
		Attr:    []string{"group:pvs", "pvs_perbuild"},
		Timeout: 100 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// BasicPassFail validates that a standard work flow of running passing and
// failing tests works as expected.
func BasicPassFail(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	pvsRunner := pvsutils.PVSRunner{
		Dut:         dut,
		ContainerID: s.FixtValue().(string),
		Env: pvsutils.RuntimeEnv{
			ReuseTLEDir: pvsutils.TmpReuseTLEDir,
		},
	}

	testplan := pvsutils.CopyToPVSOutputDir(
		ctx, s, s.DataPath(basicPassFailTestPlan),
	)

	pvsutils.EnsurePass(ctx, s, "list all requirements", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan, "--all")

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-fail-0001-v01",
			"test-none-0001-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultNotRun), Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultNotRun), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultNotRun), Count: 1},
			{Pattern: pvsutils.TestResultPattern("tast.example.Fail", pvsutils.PrintedResultNotRun), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 6},
			{Pattern: pvsutils.PrintedResultFail, Count: 0},
			{Pattern: pvsutils.PrintedResultPass, Count: 0},
		})
	})

	pvsutils.EnsurePass(ctx, s, "list requirements with tests", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-fail-0001-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultNotRun), Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultNotRun), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultNotRun), Count: 1},
			{Pattern: pvsutils.TestResultPattern("tast.example.Fail", pvsutils.PrintedResultNotRun), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 6},
			{Pattern: pvsutils.PrintedResultFail, Count: 0},
			{Pattern: pvsutils.PrintedResultPass, Count: 0},
			{Pattern: "Mandatory Requirements Satisfied: 0/3 \\(0\\%\\)", Count: 1},
		})
	})

	pvsutils.EnsurePass(ctx, s, "run tests", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-fail-0001-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultPass), Count: 1},
			{Pattern: pvsutils.TestResultPattern("tast.example.Fail", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 3},
			{Pattern: pvsutils.PrintedResultPass, Count: 3},
			{Pattern: "Error: 2 mandatory requirements failed see logs for more details", Count: 1},
			{Pattern: "Mandatory Requirements Satisfied: 1/3 \\(33\\%\\)", Count: 1},
		})
	})

	pvsutils.EnsurePass(ctx, s, "verify results", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-fail-0001-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultPass), Count: 1},
			{Pattern: pvsutils.TestResultPattern("tast.example.Fail", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 3},
			{Pattern: pvsutils.PrintedResultPass, Count: 3},
			{Pattern: "Mandatory Requirements Satisfied: 1/3 \\(33\\%\\)", Count: 1},
		})
	})

	pvsutils.EnsurePass(ctx, s, "re-run failed tests", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-fail-0001-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 2},
			{Pattern: "stub_PassServer:", Count: 0},
			{Pattern: "tast.example.Pass:", Count: 0},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.example.Fail", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 3},
			{Pattern: pvsutils.PrintedResultPass, Count: 0},
			{Pattern: "Error: 2 mandatory requirements failed see logs for more details", Count: 1},
			{Pattern: "Mandatory Requirements Satisfied: 0/2 \\(0\\%\\)", Count: 1},
		})
	})
}
