// Copyright 2020 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package apploading provides functions to assist with instrumenting and uploading
// performance metrics for ARC apploading tasts.
package apploading

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast-tests/cros/local/cpu"
	"go.chromium.org/tast-tests/cros/local/disk"
	mem_metrics "go.chromium.org/tast-tests/cros/local/memory/metrics"
	"go.chromium.org/tast-tests/cros/local/power/metrics"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestConfig defines input params for apploading.RunTest function.
type TestConfig struct {
	ClassName          string
	Prefix             string
	Subtest            string
	Iterations         int
	PerfValues         *perf.Values
	WifiInterfacesMode setup.WifiInterfacesMode
	ApkPath            string
	OutDir             string
	CollectPsiStats    bool
}

const (
	// NethelperPort is the port used for nethelper to listen for connections.
	NethelperPort = 1235

	// X86ApkName is the name of the ArcAppLoadingTest APK for x86/x86_64 devices.
	X86ApkName = "ArcAppLoadingTest_x86.apk"
	// ArmApkName is the name of the ArcAppLoadingTest APK for Arm devices.
	ArmApkName = "ArcAppLoadingTest_arm.apk"

	// Package name of the ArcAppLoadingTest APK.
	packageName = "org.chromium.arc.testapp.apploading"
)

// Used to keep information for a key, identified by the array of possible suffixes.
var keyInfo = []struct {
	// Possible suffixes for the key, for example "_score"
	suffixes []string
	// Unit name, for example "us"
	unitName string
	// Performance direction, for example perf.BiggerIsBetter.
	direction perf.Direction
}{{
	suffixes:  []string{"_score"},
	unitName:  "mbps",
	direction: perf.BiggerIsBetter,
},
}

// ApkNameForArch gets the name of the APK file to install on the DUT.
func ApkNameForArch(ctx context.Context, a *arc.ARC) (string, error) {
	out, err := a.Command(ctx, "getprop", "ro.product.cpu.abi").Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to get abi")
	}

	if strings.HasPrefix(string(out), "x86") {
		return X86ApkName, nil
	}
	return ArmApkName, nil
}

// coolDownConfig returns the config to wait for the machine to cooldown for AppLoading tests.
// This overrides the default config timeout (5 minutes) and temperature threshold (46 C)
// settings to reduce test flakes on low-end devices.
func coolDownConfig() cpu.CoolDownConfig {
	cdConfig := cpu.DefaultCoolDownConfig(cpu.CoolDownPreserveUI)
	cdConfig.PollTimeout = 7 * time.Minute
	cdConfig.TemperatureThreshold = 61000
	return cdConfig
}

// SetupTest initializes the test environment including setting up APK and power measurements.
func SetupTest(ctx context.Context, config *TestConfig, a *arc.ARC, cr *chrome.Chrome) (retCleanup setup.CleanupCallback, retErr error) {
	testing.ContextLog(ctx, "Starting setup")

	// Some configuration actions need a test connection to Chrome.
	tconn, err := cr.TestAPIConn(ctx)
	if err != nil {
		return nil, errors.Wrap(err, "failed to connect to test API")
	}

	// setup.Setup configures a DUT for a test, and returns cleanup callback.
	sup, cleanup := setup.New("apploading")

	// Add the default power test configuration.
	sup.Add(setup.PowerTest(ctx, tconn,
		setup.PowerTestOptions{
			Wifi:       config.WifiInterfacesMode,
			NightLight: setup.DisableNightLight,
			Multicast:  setup.DisableMulticast,
		},
		setup.NewBatteryDischarge(false /*discharge*/, true /*ignoreErr*/, setup.DefaultDischargeThreshold),
	))
	if err := sup.Check(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to setup power test")
	}

	testing.ContextLogf(ctx, "Installing APK: %s", config.ApkPath)
	sup.Add(setup.InstallApp(ctx, a, config.ApkPath, packageName))
	if err := sup.Check(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to install apk app")
	}
	testing.ContextLog(ctx, "Finished setup")

	return cleanup, nil
}

// RunTest executes subset of tests in APK determined by the test class name.
func RunTest(ctx context.Context, config *TestConfig, a *arc.ARC, cr *chrome.Chrome) (retScore float64, retErr error) {
	const tPowerSnapshotInterval = 10 * time.Second

	testName := packageName + "." + config.ClassName
	if config.Subtest != "" {
		testName += "#" + config.Subtest
	}
	metrics, err := perf.NewTimeline(ctx, metrics.TestMetrics(), perf.Prefix(config.Prefix+"_"), perf.Interval(tPowerSnapshotInterval))
	if err != nil {
		return 0, errors.Wrap(err, "failed to build metrics")
	}

	// Drop caches before starting test,
	if err := disk.DropCaches(ctx); err != nil {
		return 0, errors.Wrap(err, "failed to drop caches")
	}

	testing.ContextLog(ctx, "Waiting until CPU is stabilized")
	if _, err := cpu.WaitUntilStabilized(ctx, coolDownConfig()); err != nil {
		return 0, errors.Wrap(err, "failed to wait until CPU is stabilized")
	}

	testing.ContextLogf(ctx, "Running test: %s", testName)
	if err := metrics.Start(ctx); err != nil {
		return 0, errors.Wrap(err, "failed to start metrics")
	}

	if err := metrics.StartRecording(ctx); err != nil {
		return 0, errors.Wrap(err, "failed to start recording")
	}

	var avgScore float64
	for i := 0; i < config.Iterations; i++ {
		var basemem *mem_metrics.BaseMemoryStats
		if config.CollectPsiStats {
			// For collecting metrics such as smaps_rollup per process, zram usage,
			// adb dumpsys meminfo and PSI memory metrics.
			basemem, err = mem_metrics.NewBaseMemoryStats(ctx, a)
			if err != nil {
				return 0, errors.Wrap(err, "failed to start collection of extra memory stats")
			}
		}

		out, err := a.Command(ctx, "am", "instrument", "-w", "-e", "class", testName, packageName).CombinedOutput()
		if err != nil {
			return 0, errors.Wrap(err, "failed to execute test")
		}

		var iterationSuffix string
		if config.Iterations > 1 {
			iterationSuffix = "_" + strconv.Itoa(i)
		}
		outputFile := filepath.Join(config.OutDir, config.Prefix+"_test_log"+iterationSuffix+".txt")
		if err := os.WriteFile(outputFile, []byte(out), 0644); err != nil {
			return 0, errors.Wrapf(err, "failed to save test output: %s", outputFile)
		}
		testing.ContextLog(ctx, "Finished writing to log: ", outputFile)

		score, err := analyzeResults(ctx, config, iterationSuffix, out)
		if err != nil {
			return 0, errors.Wrapf(err, "error while analyzing results, see: %s", outputFile)
		}
		avgScore += score

		if basemem != nil {
			if err := mem_metrics.LogMemoryStats(ctx, basemem, a, config.PerfValues, config.OutDir, "."+config.Prefix+iterationSuffix); err != nil {
				return 0, errors.Wrap(err, "failed to dump memory stats")
			}
		}
	}
	avgScore /= float64(config.Iterations)

	powerPerfValues, err := metrics.StopRecording(ctx)
	if err != nil {
		return 0, errors.Wrap(err, "error while recording power metrics")
	}

	// Merge previous perf metrics with new power metrics.
	config.PerfValues.Merge(powerPerfValues)

	return avgScore, nil
}

// makeMetricInfo creates a metric description that can be supplied for reporting with the actual
// value. Returns an error in case key is not recognized.
func makeMetricInfo(key string) (perf.Metric, error) {
	for _, ki := range keyInfo {
		for _, suffix := range ki.suffixes {
			if !strings.HasSuffix(key, suffix) {
				continue
			}
			return perf.Metric{
				Name:      key,
				Unit:      ki.unitName,
				Direction: ki.direction,
				Multiple:  false,
			}, nil
		}
	}

	return perf.Metric{}, errors.Errorf("key could not be recognized: %s", key)
}

// analyzeResults parses the output of the test instrumentation, checks for errors,
// and returns the benchmark score.
func analyzeResults(ctx context.Context, config *TestConfig, suffix string, data []byte) (float64, error) {
	// Make sure test is completed successfully.
	if !regexp.MustCompile(`\nOK \(\d+ tests?\)\n*$`).Match(data) {
		return 0, errors.New("test is not completed successfully")
	}

	testing.ContextLog(ctx, "Analyzing results")

	// Total up all scores from the test.
	var score float64

	// Output may be prepended by other chars, and order of elements is not defined.
	// Examples:
	// INSTRUMENTATION_STATUS: MemoryTest_score=7834091.30
	// .INSTRUMENTATION_STATUS: MemoryTest_byte_count=230989
	// org.chromium.arc.testapp.apploading.ArcAppLoadTest:INSTRUMENTATION_STATUS: FileTest_duration=239890435.78
	for _, m := range regexp.MustCompile(`INSTRUMENTATION_STATUS: (.+?)=(\d+.?\d*)`).FindAllStringSubmatch(string(data), -1) {
		key := m[1]
		value, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return score, errors.Wrap(err, "failed to parse float")
		}
		if strings.HasSuffix(key, "_score") {
			score += value
			if suffix != "" {
				key = config.Prefix + suffix + "." + key
			}
			info, err := makeMetricInfo(key)
			if err != nil {
				return score, errors.Wrap(err, "failed to parse key")
			}
			config.PerfValues.Set(info, value)
		}
	}

	var result int
	// There may be several INSTRUMENTATION_STATUS_CODE: X (x = 0 or x = -1)
	for _, m := range regexp.MustCompile(`INSTRUMENTATION_STATUS_CODE: (-?\d+)`).FindAllStringSubmatch(string(data), -1) {
		if val, err := strconv.Atoi(m[1]); err != nil {
			return score, errors.Wrapf(err, "failed to convert %q to integer", m[1])
		} else if val == -1 {
			result = val
			break
		}
	}
	testing.ContextLogf(ctx, "Finished test with result: %d", result)

	if result != -1 {
		return score, errors.New("failed to pass instrumentation test")
	}

	return score, nil
}
