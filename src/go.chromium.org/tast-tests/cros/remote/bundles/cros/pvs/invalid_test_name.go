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

const invalidTestNameTestPlan = "invalid_test_name_testplan.textproto"

func init() {
	testing.AddTest(&testing.Test{
		Func:         InvalidTestName,
		Desc:         "Validates an invalid test plan is handled correctly",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"jackgelinas@google.com",
		},
		Data:    []string{invalidTestNameTestPlan},
		Attr:    []string{"group:pvs", "pvs_perbuild"},
		Timeout: 100 * time.Minute,
		Fixture: "pvsShopUnpack",
	})
}

// InvalidTestName validates an invalid test plan is handled correctly.
func InvalidTestName(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	pvsRunner := pvsutils.PVSRunner{
		Dut:         dut,
		ContainerID: s.FixtValue().(string),
		Env: pvsutils.RuntimeEnv{
			ReuseTLEDir:            pvsutils.TmpReuseTLEDir,
			SimulatedTestsNoResult: "stub_PassServer",
		},
	}

	testplan := pvsutils.CopyToPVSOutputDir(
		ctx, s, s.DataPath(invalidTestNameTestPlan),
	)

	pvsutils.EnsurePass(ctx, s, "run tests", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "run", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-invalid-0001-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 1},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultPass), Count: 1},
			{Pattern: pvsutils.TestResultPattern("noResult", pvsutils.PrintedResultError), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.noResult", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 0},
			{Pattern: pvsutils.PrintedResultFail, Count: 0},
			{Pattern: "Error: 1 mandatory requirements failed see logs for more details", Count: 1},
		})
		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultError), Count: 1},
			{Pattern: pvsutils.PrintedResultPass, Count: 1},
			{Pattern: pvsutils.PrintedResultError, Count: 3},
		})
	})

	pvsutils.EnsurePass(ctx, s, "verify results", func(ctx context.Context, s *testing.State) {
		output := pvsRunner.RunPVSCommand(ctx, s, "list", "--test-plan", testplan)

		pvsutils.ValidateOutputContains(s, output, []string{
			"test-invalid-0001-v01",
		})

		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.RequirementLevelMust, Count: 1},
			{Pattern: pvsutils.TestResultPattern("tast.example.Pass", pvsutils.PrintedResultPass), Count: 1},
			{Pattern: pvsutils.TestResultPattern("noResult", pvsutils.PrintedResultNotRun), Count: 2},
			{Pattern: pvsutils.TestResultPattern("tast.noResult", pvsutils.PrintedResultNotRun), Count: 1},
			{Pattern: pvsutils.PrintedResultFail, Count: 0},
			{Pattern: pvsutils.PrintedResultError, Count: 0},
		})
		pvsutils.CountTestCases(s, output, []pvsutils.RepeatedWord{
			{Pattern: pvsutils.TestResultPattern("stub_PassServer", pvsutils.PrintedResultNotRun), Count: 1},
			{Pattern: pvsutils.PrintedResultNotRun, Count: 3},
			{Pattern: pvsutils.PrintedResultPass, Count: 1},
		})
	})
}
