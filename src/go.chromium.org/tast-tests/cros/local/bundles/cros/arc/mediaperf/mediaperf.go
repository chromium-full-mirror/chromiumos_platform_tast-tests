// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package mediaperf provides functions to assist with instrumenting and uploading
// performance metrics for ARC mediaperf tasts.
package mediaperf

import (
	"context"
	"os"
	"path/filepath"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast-tests/cros/local/power/setup"

	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// TestConfig defines input params for mediaperftest.RunTest function.
type TestConfig struct {
	ClassName  string
	Prefix     string
	Subtest    string
	Iterations int
	PerfValues *perf.Values
	ApkPath    string
	OutDir     string
}

const (
	// X86ApkName is the name of the ArcMediaPerfTester APK for x86/x86_64 devices.
	X86ApkName = "ArcMediaPerfTester_x86.apk"
	// ArmApkName is the name of the ArcMediaPerfTester APK for Arm devices.
	ArmApkName = "ArcMediaPerfTester_arm.apk"

	// Package name of the ArcMediaPerfTester APK.
	packageName = "dev.arc.mediaperftester"
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
	unitName:  "ms",
	direction: perf.SmallerIsBetter,
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

// SetupTest initializes the test environment including setting up APK.
func SetupTest(ctx context.Context, config *TestConfig, a *arc.ARC) (retCleanup setup.CleanupCallback, retErr error) {
	testing.ContextLog(ctx, "Starting setup")

	// setup.Setup configures a DUT for a test, and returns cleanup callback.
	sup, cleanup := setup.New("mediaperf")

	testing.ContextLogf(ctx, "Installing APK: %s", config.ApkPath)
	sup.Add(setup.InstallApp(ctx, a, config.ApkPath, packageName))
	if err := sup.Check(ctx); err != nil {
		return nil, errors.Wrap(err, "failed to install apk app")
	}
	testing.ContextLog(ctx, "Finished setup")

	return cleanup, nil
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
			}, nil
		}
	}
	return perf.Metric{}, errors.Errorf("key could not be recognized: %s", key)
}

// RunTest executes subset of tests in APK determined by the test class name.
func RunTest(ctx context.Context, config *TestConfig, a *arc.ARC) (retScore float64, retErr error) {
	testName := packageName + "." + config.ClassName

	out, err := a.Command(ctx, "am", "instrument", "-w", "-e", "class", testName, packageName).CombinedOutput()
	if err != nil {
		return 0, errors.Wrap(err, "failed to execute test")
	}
	if out != nil {
		testing.ContextLog(ctx, string(out))
	}

	outputFile := filepath.Join(config.OutDir, config.Prefix+"_test_log.txt")
	if err := os.WriteFile(outputFile, []byte(out), 0644); err != nil {
		return 0, errors.Wrapf(err, "failed to save test output: %s", outputFile)
	}
	testing.ContextLog(ctx, "Finished writing to log: ", outputFile)

	var iterationSuffix string
	score, err := analyzeResults(ctx, config, iterationSuffix, out)
	if err != nil {
		return 0, errors.Wrap(err, "error while analyzing results")
	}
	return score, nil
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
	// INSTRUMENTATION_STATUS: sequential_decode_score=33900
	// INSTRUMENTATION_STATUS: stream_1_decode_time=44590
	for _, m := range regexp.MustCompile(`INSTRUMENTATION_STATUS: (.+?)=(\d+.?\d+)`).FindAllStringSubmatch(string(data), -1) {
		key := m[1]
		testing.ContextLog(ctx, key)
		testing.ContextLog(ctx, m[2])
		value, err := strconv.ParseFloat(m[2], 64)
		if err != nil {
			return score, errors.Wrap(err, "failed to parse float")
		}
		if strings.HasSuffix(key, "_score") {
			score = value
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
	testing.ContextLogf(ctx, "Finished test with status code: %d", result)

	if result != -1 {
		return score, errors.New("failed to pass instrumentation test")
	}

	return score, nil
}
