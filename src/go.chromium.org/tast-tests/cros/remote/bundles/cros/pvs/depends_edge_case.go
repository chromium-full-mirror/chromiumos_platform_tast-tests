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

const dependsEdgeCaseTestPlan = "depends_edge_case_testplan.textproto"

func init() {
	testing.AddTest(&testing.Test{
		Func:         DependsEdgeCase,
		Desc:         "Validates requirement dependency edge cases",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"bbrotherton@google.com",
		},
		Data:    []string{dependsEdgeCaseTestPlan},
		Attr:    []string{"group:pvs", "pvs_perbuild"},
		Timeout: 100 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// DependsEdgeCase validates edge cases of running requirements with
// dependencies.
func DependsEdgeCase(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	pvsRunner := pvsutils.PVSRunner{
		Dut:         dut,
		ContainerID: s.FixtValue().(string),
		Env: pvsutils.RuntimeEnv{
			ReuseTLEDir: pvsutils.TmpReuseTLEDir,
		},
	}

	testplan := pvsutils.CopyToPVSOutputDir(
		ctx, s, s.DataPath(dependsEdgeCaseTestPlan),
	)

	pvsutils.EnsurePass(ctx, s, "list requirements", func(ctx context.Context, s *testing.State) {
		output, _ := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01",
			"test-other-0001-v01",
			"test-fail-0001-v01",
			"test-fail-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 4},
			{Pattern: pvsutils.RequirementLevelMay, Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultNotRun), Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultNotRun), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultNotRun), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 7},
			{Pattern: pvsutils.PrintedResultFail, Count: 0},
			{Pattern: pvsutils.PrintedResultPass, Count: 0},
		})
	})

	pvsutils.EnsurePass(ctx, s, "run tests", func(ctx context.Context, s *testing.State) {
		output, _ := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01", // test-passfail-0001-v01 and test-pass-0001-v01 dependency
			"test-other-0001-v01",
			"test-fail-0001-v01", // test-fail-0002-v01 dependency
			"test-fail-0002-v01", // skipped because test-fail-0001-v01 fails
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 4},
			{Pattern: pvsutils.RequirementLevelMay, Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultNA), Count: 1},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultPass), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 2},
			{Pattern: pvsutils.PrintedResultPass, Count: 4},
			{Pattern: pvsutils.PrintedResultNA, Count: 1},
		})
	})

	pvsutils.EnsurePass(ctx, s, "verify results", func(ctx context.Context, s *testing.State) {
		output, _ := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01",
			"test-other-0001-v01",
			"test-fail-0001-v01",
			"test-fail-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 4},
			{Pattern: pvsutils.RequirementLevelMay, Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultPass), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 2},
			{Pattern: pvsutils.PrintedResultPass, Count: 5},
		})
	})
}
