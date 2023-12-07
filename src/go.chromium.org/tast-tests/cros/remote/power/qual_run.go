// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"encoding/json"
	"fmt"
	"io/fs"
	"os"
	"path"
	"regexp"
	"strconv"

	"github.com/golang/protobuf/ptypes/empty"
	"gonum.org/v1/gonum/stat"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/power"
	"go.chromium.org/tast-tests/cros/common/power/powerpb"
	"go.chromium.org/tast-tests/cros/common/utils"
	"go.chromium.org/tast-tests/cros/remote/power/config"
	"go.chromium.org/tast-tests/cros/remote/power/result"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/rpc"
	"go.chromium.org/tast/core/testing"
)

// QualRun holds the power qual run information.
type QualRun struct {
	// Config defines the test configuration.
	Config *config.Config
	// Tests includes all the tests.
	Tests []string
	// OrderedTests includes tests that must run first with the given order.
	OrderedTests []string
	// UnorderedTests contains all tests that can be run in any order.
	UnorderedTests []string
	// WeightedTests maps test name to boolean that indicates if the test
	// weight > 0 or not.
	WeightedTests map[string]bool

	// testPowers holds the power results for each test.
	testPowers map[string]*result.Power
	// deviceInfo contains the DUT information.
	deviceInfo *powerpb.DeviceInfo
	// otherInfo contains other device or test information, such as the
	// backlight percentage, in a map.
	otherInfo map[string]interface{}
	// skippedTests holds skipped tests.
	skippedTests []string
}

// NewQualRun returns a new QualRun from a test configuration URL.
func NewQualRun(ctx context.Context, url string, dut *dut.DUT, rpcHint *testing.RPCHint) (*QualRun, error) {
	configJSON, err := utils.FetchFromURL(ctx, url)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to fetch configuration from %s", url)
	}

	cfg := &config.Config{}
	if err := json.Unmarshal([]byte(configJSON), cfg); err != nil {
		return nil, errors.Wrap(err, "failed to unmarshal configuration")
	}

	tests, orderedTests, unorderedTests, err := config.ValidateConfig(cfg)
	if err != nil {
		return nil, errors.Wrap(err, "failed to validate configuration")
	}

	cl, err := rpc.Dial(ctx, dut, rpcHint)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to the RPC service")
	}
	defer cl.Close(ctx)
	client := powerpb.NewLocalInfoServiceClient(cl.Conn)
	deviceInfo, err := client.GetDeviceInfoFromDUT(ctx, &empty.Empty{})
	if err != nil {
		return nil, errors.Wrap(err, "failed to get DUT device info")
	}

	weightedTests := config.FindWeightedTests(cfg)

	return &QualRun{
		Config:         cfg,
		Tests:          tests,
		OrderedTests:   orderedTests,
		UnorderedTests: unorderedTests,
		WeightedTests:  weightedTests,
		testPowers:     make(map[string]*result.Power),
		deviceInfo:     deviceInfo,
		otherInfo:      make(map[string]interface{}),
	}, nil
}

func (r *QualRun) isTestSkipped(test string) bool {
	for _, t := range r.skippedTests {
		if t == test {
			return true
		}
	}
	return false
}

// AddTestResults adds power qual run test results.
func (r *QualRun) AddTestResults(ctx context.Context, tests, skippedTests []string, resultDir string) error {
	if len(skippedTests) > 0 && r.Config.Control.FailOnSkippedTest {
		return errors.Errorf("skipped tests are not allowed from the test control but got skipped test(s) %v ", skippedTests)
	}

	// Get test directories.
	testsDir := path.Join(resultDir, "tests")
	dirs, err := os.ReadDir(testsDir)
	if err != nil {
		return errors.Wrapf(err, "failed to read tests directory under %s", resultDir)
	}

	r.skippedTests = append(r.skippedTests, skippedTests...)

	// Read the power test result for each test.
	for _, t := range tests {
		if r.isTestSkipped(t) {
			continue
		}
		if !r.WeightedTests[t] {
			continue
		}
		dir, err := findTestDir(t, dirs)
		if err != nil {
			return errors.Wrapf(err, "failed to find %s test dir from %s", t, testsDir)
		}
		// Read the power metrics from the test power_log json file.
		average, err := readPowerMetrics(path.Join(testsDir, dir, "power_log.json"))
		if err != nil {
			return errors.Wrapf(err, "failed to read power metrics for test %s", t)
		}
		r.testPowers[t] = &result.Power{Average: result.Average{
			MinutesBatteryLife:       average[power.MinutesBatteryLifeKey].(float64),
			MinutesBatteryLifeTested: average[power.MinutesBatteryLifeTestedKey].(float64),
		}}
		// Record other average values.
		for _, key := range []string{power.BacklightPercentNonlinearKey, power.BacklightPercentLinearKey} {
			if value, ok := average[key]; ok {
				r.otherInfo[key] = value
			}
		}

	}
	return nil
}

// GenerateReport generates the power qual run test report.
func (r *QualRun) GenerateReport(ctx context.Context, outputDir, testName string) error {
	// The power qual test final result.
	res := result.Result{
		FormatVersion: result.FormatVersion,
		Name:          r.Config.Name,
		Version:       r.Config.Version,
	}

	// The power log used to generate power_logs.json.
	var powerLogs []byte
	// Performance values used to generate results-chart.json.
	pv := perf.NewValues()

	// Calculate result for each persona.
	for _, p := range r.Config.Personas {
		persona := result.Persona{
			Name: p.Name,
		}

		var values []float64
		var weights []float64
		minutesBatteryLifeTestedTotal := 0.0
		for _, t := range p.Tests {
			if r.isTestSkipped(t.Name) {
				persona.Skipped = append(persona.Skipped, t.Name)
				continue
			}
			if t.Weight == 0 {
				continue
			}
			power := r.testPowers[t.Name]
			if power == nil {
				return errors.Errorf("no power test result for %s", t.Name)
			}
			if power.Average.MinutesBatteryLifeTested < t.MinRunningTime {
				return errors.Errorf("test running time %f is less than min_running_time %f", power.Average.MinutesBatteryLifeTested, t.MinRunningTime)
			}
			// Collect minutes_battery_life_tested for each subtest in perf.Values.
			pv.Set(perf.Metric{
				Name:      p.Name + "." + t.Name + "." + "minutes_battery_life_tested",
				Unit:      "minute",
				Direction: perf.SmallerIsBetter,
			}, power.Average.MinutesBatteryLifeTested)
			persona.Tests = append(persona.Tests, result.Test{Name: t.Name, Weight: t.Weight, Power: *power})
			values = append(values, power.Average.MinutesBatteryLife)
			weights = append(weights, t.Weight)
			minutesBatteryLifeTestedTotal += power.Average.MinutesBatteryLifeTested
		}

		minutesBatteryLife := stat.HarmonicMean(values, weights)

		// Collect battery metrics for each persona in perf.Values.
		pv.Set(perf.Metric{
			Name:      p.Name + "." + power.MinutesBatteryLifeKey,
			Unit:      "minute",
			Direction: perf.BiggerIsBetter,
		}, minutesBatteryLife)
		pv.Set(perf.Metric{
			Name:      p.Name + "." + power.MinutesBatteryLifeTestedKey,
			Unit:      "minute",
			Direction: perf.SmallerIsBetter,
		}, minutesBatteryLifeTestedTotal)

		// Persona local perf values that will be used to generate power log.
		pvLocal := perf.NewValues()
		// Set these known metrics, so the power log library won't generate
		// them again. Set no prefix to the metric name to satisfy the library
		// checking.
		pvLocal.Set(perf.Metric{
			Name:      power.MinutesBatteryLifeKey,
			Unit:      "minute",
			Direction: perf.BiggerIsBetter,
		}, minutesBatteryLife)
		pvLocal.Set(perf.Metric{
			Name:      power.MinutesBatteryLifeTestedKey,
			Unit:      "minute",
			Direction: perf.SmallerIsBetter,
		}, minutesBatteryLifeTestedTotal)
		for _, key := range []string{power.BacklightPercentNonlinearKey, power.BacklightPercentLinearKey} {
			if value, ok := r.otherInfo[key].(float64); ok {
				pvLocal.Set(perf.Metric{
					Name:      key,
					Unit:      power.GeneralPerfMetricTypeUnit,
					Direction: perf.BiggerIsBetter,
				}, value)
			}
		}
		powerLog, err := power.CreateSaveUploadPowerLog(ctx, outputDir, testName+"."+p.Name, "_"+p.Name, pvLocal, nil, r.deviceInfo, nil)
		if err != nil {
			return errors.Wrap(err, "failed to create power log")
		}
		plBytes, err := json.MarshalIndent(powerLog, "", "  ")
		if err != nil {
			return errors.Wrap(err, "failed to marshal power log results into JSON")
		}
		powerLogs = append(powerLogs, plBytes...)

		persona.Power = result.Power{
			Average: result.Average{MinutesBatteryLife: minutesBatteryLife, MinutesBatteryLifeTested: minutesBatteryLifeTestedTotal},
		}

		res.Personas = append(res.Personas, persona)
	}

	qualResultBytes, err := json.MarshalIndent(res, "", " ")
	if err != nil {
		return errors.Wrap(err, "failed to marshal run results into JSON")
	}
	if err := os.WriteFile(path.Join(outputDir, "power_qual_result.json"), qualResultBytes, 0644); err != nil {
		return errors.Wrap(err, "failed to write power qaul result to file")
	}
	if err := os.WriteFile(path.Join(outputDir, "power_log.json"), powerLogs, 0644); err != nil {
		return errors.Wrap(err, "failed to write power log to file")
	}
	// Generate results-chart.json.
	if err = pv.Save(outputDir); err != nil {
		return errors.Wrap(err, "failed to store performance values")
	}
	return nil
}

// findTestDir finds the final directory for a test.
func findTestDir(test string, dirs []fs.DirEntry) (dir string, err error) {
	// The result for a test is put under the directory with the same name as the test.
	// A test can be retried, and each retry will add a number suffix increasingly to the dir name.
	// Valid test dir name examples:
	// "power.ExampleUI.ash", "power.ExampleUI.ash.1", "power.ExampleUI.ash.2"

	// Regular expression to match the test name or test name with number suffix.
	re := regexp.MustCompile(fmt.Sprintf(`^%s(\.\d+)?$`, test))

	var suffix int64 = -1
	for _, d := range dirs {
		if !d.IsDir() {
			continue
		}
		matches := re.FindStringSubmatch(d.Name())
		if len(matches) == 0 {
			continue
		}
		var newSuffix int64 = 0
		if matches[1] != "" {
			newSuffix, err = strconv.ParseInt(matches[1], 10, 64)
			if err != nil {
				return
			}
		}
		// Find the dir name with the largest suffix.
		if newSuffix > suffix {
			dir = d.Name()
			suffix = newSuffix
		}
	}
	if suffix == -1 {
		err = errors.Errorf("test directory is not found for %s", test)
	}
	return
}

// powerLogResult is the mapping to the power_log.json content.
type powerLogResult struct {
	// We are interested in some fields from the power log.
	Power struct {
		Average map[string]interface{} `json:"average"`
	} `json:"power"`
}

// readPowerMetrics reads the power metric values from the given power_log.json file.
func readPowerMetrics(file string) (map[string]interface{}, error) {
	bytes, err := os.ReadFile(file)
	if err != nil {
		return nil, errors.Wrapf(err, "failed to read file %s", file)
	}
	res := &powerLogResult{}
	if err := json.Unmarshal(bytes, res); err != nil {
		return nil, errors.Wrapf(err, "failed to unmarshal json from file %s", file)
	}
	return res.Power.Average, nil
}
