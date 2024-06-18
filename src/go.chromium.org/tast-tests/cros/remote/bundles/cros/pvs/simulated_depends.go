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

const simulatedDependsTestPlan = "simulated_depends_testplan.textproto"

func init() {
	testing.AddTest(&testing.Test{
		Func:         SimulatedDepends,
		Desc:         "Validates requirement dependencies work as expected using simualted environment",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"bbrotherton@google.com",
		},
		Data:    []string{simulatedDependsTestPlan},
		Attr:    []string{"group:pvs", "pvs_perbuild"},
		Timeout: 100 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// SimulatedDepends validates requirement dependencies work as expected using
// simulated environment.
func SimulatedDepends(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	// Add simulated args...
	pvsRunner := pvsutils.PVSRunner{
		Dut:         dut,
		ContainerID: s.FixtValue().(string),
		Env: pvsutils.RuntimeEnv{
			ReuseTLEDir:         pvsutils.TmpReuseTLEDir,
			SimulatedDut:        true,
			SimulatedTestRunner: true,
		},
	}

	testplan := pvsutils.CopyToPVSOutputDir(
		ctx, s, s.DataPath(simulatedDependsTestPlan),
	)

	pvsutils.EnsurePass(ctx, s, "run tests dlm_sku_id=3494", func(ctx context.Context, s *testing.State) {
		pvsRunner.Env.SimulatedDutInfo = "dedede:drawcia:262144:3494"

		output, _ := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01", // test-passfail-0001-v01 and test-pass-0001-v01 dependency
			"test-pass-0003-v01",
			"test-fail-0001-v01", // test-fail-0002-v01 dependency
			"test-fail-0002-v01", // skipped because test-fail-0001-v01 fails
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 3},
			{Pattern: pvsutils.RequirementLevelMay, Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultNA), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 2},
			{Pattern: pvsutils.PrintedResultPass, Count: 4},
			{Pattern: pvsutils.PrintedResultNA, Count: 1},
		})
	})

	pvsutils.EnsurePass(ctx, s, "run tests dlm_sku_id=3495", func(ctx context.Context, s *testing.State) {
		pvsRunner.Env.SimulatedDutInfo = "dedede:drawcia:262144:3495"
		pvsRunner.Env.SimulatedTestsFail = "stub_PassServer"

		output, _ := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01", // test-passfail-0001-v01 and test-pass-0001-v01 dependency
			"test-pass-0003-v01",
			"test-fail-0001-v01", // test-fail-0002-v01 dependency
			"test-fail-0002-v01", // skipped because test-fail-0001-v01 fails
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 3},
			{Pattern: pvsutils.RequirementLevelMay, Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultFail), Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultNA), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 6},
			{Pattern: pvsutils.PrintedResultPass, Count: 0},
			{Pattern: pvsutils.PrintedResultNA, Count: 1},
		})
	})

	pvsutils.EnsurePass(ctx, s, "verify results", func(ctx context.Context, s *testing.State) {
		pvsRunner.Env.SimulatedTestsFail = ""

		output, _ := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01",
			"test-pass-0003-v01",
			"test-fail-0001-v01",
			"test-fail-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 3},
			{Pattern: pvsutils.RequirementLevelMay, Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultFail), Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 4},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultNA), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 8},
			{Pattern: pvsutils.PrintedResultPass, Count: 4},
			{Pattern: pvsutils.PrintedResultNA, Count: 2},
			{Pattern: "dlm_sku_id:3494", Count: 6},
			{Pattern: "dlm_sku_id:3495", Count: 6},
		})
	})

	pvsutils.EnsurePass(ctx, s, "re-run tests dlm_sku_id=3494", func(ctx context.Context, s *testing.State) {
		pvsRunner.Env.SimulatedDutInfo = "dedede:drawcia:262144:3494"

		output, _ := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-fail-0001-v01", // test-fail-0002-v01 dependency
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 1},
			{Pattern: pvsutils.RequirementLevelMay, Count: 1},
			{Pattern: "stub_PassServer:", Count: 0},
			{Pattern: "SHOULD_NOT_RUN", Count: 0},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 2},
			{Pattern: pvsutils.PrintedResultPass, Count: 0},
			{Pattern: pvsutils.PrintedResultNA, Count: 0},
		})
	})

	pvsutils.EnsurePass(ctx, s, "verify results after re-run", func(ctx context.Context, s *testing.State) {
		output, _ := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-passfail-0001-v01",
			"test-pass-0001-v01",
			"test-pass-0002-v01",
			"test-fail-0001-v01",
			"test-fail-0002-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 3},
			{Pattern: pvsutils.RequirementLevelMay, Count: 3},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultFail), Count: 4},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 4},
			{Pattern: pvsutils.TestResultPattern("SHOULD_NOT_RUN", pvsutils.PrintedResultNA), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 8},
			{Pattern: pvsutils.PrintedResultPass, Count: 4},
			{Pattern: pvsutils.PrintedResultNA, Count: 2},
			{Pattern: "dlm_sku_id:3494", Count: 6},
			{Pattern: "dlm_sku_id:3495", Count: 6},
		})
	})
}
