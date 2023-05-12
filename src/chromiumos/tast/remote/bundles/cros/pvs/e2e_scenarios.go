// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pvs

import (
	"context"
	"fmt"
	"time"

	"chromiumos/tast/remote/bundles/cros/pvs/pvsutils"

	"go.chromium.org/tast/core/testing"
)

const (
	scenarioTestRunnerPath = "/usr/libexec/pvs_scenario_test"
	testDataPath           = "/usr/share/pvs/testdata"
)

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
				Val:  "TestBasicPassFailScenario",
			},
			{
				Name: "dependencies",
				Val:  "TestDependenciesScenario",
			},
			{
				Name: "dependencies_skip",
				Val:  "TestDependenciesSkipScenario",
			},
			{
				Name: "dependencies_edge_case",
				Val:  "TestDependenciesScenarioEdgeCase",
			},
			{
				Name: "process_control_characters",
				Val:  "TestProcessControlCharacters",
			},
			{
				Name: "count_test_cases",
				Val:  "TestCountTestCases",
			},
			{
				Name: "invalid_test_name",
				Val:  "TestInvalidTestName",
			},
			{
				Name: "pass_criteria",
				Val:  "TestPassCriteria",
			},
			{
				Name: "dependencies_multi_sku",
				Val:  "TestDependenciesMultiSKUScenario",
			},
		},
	})
}

// E2EScenarios runs the PVS scenario tests against a DUT using the NextGen
// workflow
// Current preconditions:
//   - .gitcookies are populated in ${CHRONOS_HOME}/.gitcookies
//   - upload_config dir is populated in ${CHRONOS_HOME}/.pvs/
//   - reverse tunnel is set up from the host to the dut on port 2223
func E2EScenarios(ctx context.Context, s *testing.State) {
	dut := s.DUT().Conn()
	containerID := s.FixtValue().(string)
	scenarioTest := s.Param().(string)
	runScenarioTest := fmt.Sprintf(
		`docker exec -e TESTDATA_DIR=%q %q /usr/bin/gosu pvs %q -test.v -test.run "^%v\$"`,
		testDataPath,
		containerID,
		scenarioTestRunnerPath,
		scenarioTest,
	)
	if _, err := pvsutils.RunAsChronos(ctx, dut, runScenarioTest); err != nil {
		s.Fatal("Error occured when running scenario test: ", err)
	}
}
