// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/pvs/pvsutils"

	"go.chromium.org/tast/core/testing"
)

const (
	scenarioTestRunnerPath = "/usr/libexec/pvs_scenario_test"
	testDataPath           = "/usr/share/pvs/testdata"
)

type testCase struct {
	scenarioName string
	isSimulated  bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func:         E2EScenarios,
		Desc:         "Validate PVS",
		BugComponent: "b:1110659",
		Contacts: []string{
			"chromeos-pvs-eng@google.com",
			"jackgelinas@google.com",
		},
		Attr:    []string{"group:pvs", "pvs_perbuild"},
		Timeout: 600 * time.Minute,
		Fixture: "pvsShopUnpack",
		Params: []testing.Param{
			{
				Name: "basic_pass_fail",
				Val: testCase{
					scenarioName: "TestBasicPassFailScenario",
					isSimulated:  false,
				},
			},
			{
				Name: "dependencies",
				Val: testCase{
					scenarioName: "TestDependenciesScenario",
					isSimulated:  false,
				},
			},
			{
				Name: "dependencies_skip",
				Val: testCase{
					scenarioName: "TestDependenciesSkipScenario",
					isSimulated:  true,
				},
			},
			{
				Name: "dependencies_edge_case",
				Val: testCase{
					scenarioName: "TestDependenciesScenarioEdgeCase",
					isSimulated:  false,
				},
			},
			{
				Name: "process_control_characters",
				Val: testCase{
					scenarioName: "TestProcessControlCharacters",
					isSimulated:  false,
				},
			},
			{
				Name: "count_test_cases",
				Val: testCase{
					scenarioName: "TestCountTestCases",
					isSimulated:  false,
				},
			},
			{
				Name: "invalid_test_name",
				Val: testCase{
					scenarioName: "TestInvalidTestName",
					isSimulated:  false,
				},
			},
			{
				Name: "pass_criteria",
				Val: testCase{
					scenarioName: "TestPassCriteria",
					isSimulated:  true,
				},
			},
			{
				Name: "dependencies_multi_sku",
				Val: testCase{
					scenarioName: "TestDependenciesMultiSKUScenario",
					isSimulated:  true,
				},
			},
		},
	})
}

// E2EScenarios runs the PVS scenario tests against a DUT using the NextGen
// workflow
func E2EScenarios(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	containerID := s.FixtValue().(string)
	testCase := s.Param().(testCase)
	envVars := []string{
		fmt.Sprintf("TESTDATA_DIR=%q", testDataPath),
	}
	if testCase.isSimulated {
		envVars = append(envVars, "SIMULATED_DUT=1", "SIMULATED_TEST_RUNNER=1")
	}
	runScenarioTest := fmt.Sprintf(
		`docker exec %v %q /usr/bin/gosu pvs %q -test.v -test.run "^%v\$"`,
		fmt.Sprintf("-e %v", strings.Join(envVars, " -e ")),
		containerID,
		scenarioTestRunnerPath,
		testCase.scenarioName,
	)
	if _, err := pvsutils.RunAsChronos(ctx, dut, runScenarioTest); err != nil {
		s.Fatal("Error occured when running scenario test: ", err)
	}
}
