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

const passCriteriaKeyval = "pass_criteria_keyval.txt"
const passCritieraTestPlan = "pass_criteria_testplan.textproto"

func init() {
	testing.AddTest(&testing.Test{
		Func:         PassCriteria,
		Desc:         "Validates test results are evaluated correctly using pass criteria",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"jackgelinas@google.com",
		},
		Data:    []string{passCriteriaKeyval, passCritieraTestPlan},
		Attr:    []string{"group:pvs", "pvs_perbuild"},
		Timeout: 100 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// PassCriteria validates test results are evaluated correctly using pass criteria.
func PassCriteria(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	keyval := pvsutils.CopyToPVSOutputDir(
		ctx, s, s.DataPath(passCriteriaKeyval),
	)
	pvsRunner := pvsutils.PVSRunner{
		Dut:         dut,
		ContainerID: s.FixtValue().(string),
		Env: pvsutils.RuntimeEnv{
			ReuseTLEDir:         pvsutils.TmpReuseTLEDir,
			SimulatedDut:        true,
			SimulatedTestRunner: true,
			SimulatedTestKeyval: keyval,
		},
	}

	testplan := pvsutils.CopyToPVSOutputDir(
		ctx, s, s.DataPath(passCritieraTestPlan),
	)

	pvsutils.EnsurePass(ctx, s, "run tests", func(ctx context.Context, s *testing.State) {
		output, _ := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"boot-perf-0008-v01",
			"boot-perf-0009-v01",
		})
		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 1},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.TestResultPattern("power_UiResume.freeze", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 3},
			{Pattern: pvsutils.PrintedResultPass, Count: 1},
			{Pattern: "exceeds upper bound", Count: 2},
		})
	})

	pvsutils.EnsurePass(ctx, s, "verify results", func(ctx context.Context, s *testing.State) {
		output, _ := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"boot-perf-0008-v01",
			"boot-perf-0009-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 2},
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultPass), Count: 1},
			{Pattern: pvsutils.TestResultPattern("stub_FailServer", pvsutils.PrintedResultFail), Count: 1},
			{Pattern: pvsutils.TestResultPattern("power_UiResume.freeze", pvsutils.PrintedResultFail), Count: 2},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 3},
			{Pattern: pvsutils.PrintedResultPass, Count: 1},
			{Pattern: "exceeds upper bound", Count: 2},
		})
	})
}
