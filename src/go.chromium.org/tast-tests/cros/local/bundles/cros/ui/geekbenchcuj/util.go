// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package geekbenchcuj provide util functions for Geekbench benchmark.
package geekbenchcuj

import (
	"context"
	"encoding/json"
	"fmt"
	"io/ioutil"
	"regexp"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/vm"
	"go.chromium.org/tast/core/errors"
)

const (
	geekbenchPrefix  = "Benchmark.Geekbench."
	geekbenchArmv7   = "armv7"
	geekbenchAarch64 = "aarch64"
	geekbenchX86_64  = "x86_64"
	geekbenchPlar    = "geekbench.plar"
	geekbenchEmail   = "geekbench.email"
	geekbenchKey     = "geekbench.key"
)

// geekbenchDir will contain the path and method to remove directory.
type geekbenchDir struct {
	path  string
	rmDir func(context.Context)
}

// GeekbenchFiles will contain the files needed for running geekbench.
type geekbenchFiles struct {
	binarySrc  string
	binaryDest string
	plarSrc    string
	plarDest   string
	// For Geekbench 6 only.
	workloadSrc string
	// For Gekebench 6 only.
	workloadDest string
}

// workload is either a single-core or multi-core subtest.
type workload struct {
	Name  string  `json:"name"`
	Score float64 `json:"score"`
}

// section contains the test suite name (single or multi)
// and the corresponding subtests.
type section struct {
	Name      string     `json:"name"`
	Score     float64    `json:"score"`
	Workloads []workload `json:"workloads"`
}

// result contains a list of test suite results.
type result struct {
	Sections []section `json:"sections"`
}

// execCommand returns a testexec.Cmd object to execute the Geekbench binary.
func execCommand(ctx context.Context, cont *vm.Container, execFilePath, resultPath string) *testexec.Cmd {
	if cont != nil {
		return cont.Command(ctx, execFilePath, "--no-upload", "--export-json", resultPath)
	}
	return testexec.CommandContext(ctx, execFilePath, "--no-upload", "--export-json", resultPath)
}

// retrieveScore formats the metric values for Geekbench and inputs them in a map.
func retrieveScore(ctx context.Context, crosPath string, scores map[string]float64) error {
	bytes, err := ioutil.ReadFile(crosPath)
	if err != nil {
		return errors.Wrap(err, "failed to read the results")
	}

	result := result{}
	if err := json.Unmarshal(bytes, &result); err != nil {
		return errors.Wrap(err, "failed to unmarshal the results")
	}

	replacer := strings.NewReplacer("-", "_", " ", "_")

	for _, section := range result.Sections {
		if section.Name != "Single-Core" && section.Name != "Multi-Core" {
			return errors.Errorf("unrecognized test suite: %s", section.Name)
		}

		prefix := fmt.Sprintf("%s%s.", geekbenchPrefix, strings.ReplaceAll(section.Name, "-", ""))
		scores[prefix+"Score"] = section.Score
		for _, workload := range section.Workloads {
			scores[prefix+replacer.Replace(workload.Name)] = workload.Score
		}
	}

	return nil
}

// getExecutableFileName gets the name of GB executable for the DUT architecture.
// If ignoreARMBitWidth is set to true, 64 bit ARM (aarch64) will be
// returned without checking bit width. The option exists for Crostini as
// all ARM VM devices are set to ARM64.
func getExecutableFileName(ctx context.Context, ignoreARMBitWidth bool, version int) (string, error) {
	unameCmd := testexec.CommandContext(ctx, "uname", "-m")
	b, err := unameCmd.Output(testexec.DumpLogOnError)
	if err != nil {
		return "", errors.Wrap(err, "failed to determine the architecture")
	}

	geekbenchFmt := "geekbench%d_%s"

	switch arch := strings.TrimSpace(string(b)); arch {
	case "aarch64":
		if ignoreARMBitWidth {
			return fmt.Sprintf(geekbenchFmt, version, geekbenchAarch64), nil
		}
		fileCmd := testexec.CommandContext(ctx, "file", "/sbin/init")
		b, err := fileCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			return "", errors.Wrap(err, "failed to determine the architecture bit width")
		}
		re := regexp.MustCompile(`[0-9]+-bit`)
		switch bit := re.FindString(string(b)); bit {
		case "64-bit":
			return fmt.Sprintf(geekbenchFmt, version, geekbenchAarch64), nil
		case "32-bit":
			return fmt.Sprintf(geekbenchFmt, version, geekbenchArmv7), nil
		default:
			return "", errors.Errorf("unsupported architecture bit width: %s", bit)
		}
	case "x86_64":
		return fmt.Sprintf(geekbenchFmt, version, geekbenchX86_64), nil
	default:
		return "", errors.Errorf("unsupported architecture: %s", arch)
	}
}

// GetGBInfo gets the necessary GeekbenchInfo struct containing information to run
// the specific type of GeekbenchCUJ given environment, version, and needLicense.
func GetGBInfo(env string, version int, needLicense bool) geekbenchInfo {
	var gbInfo geekbenchInfo
	if strings.Contains(env, "native") {
		gbInfo = nativeGeekbenchInfo
		if strings.Contains(env, "custom") {
			gbInfo.name = "native_custom"
		}
	} else if strings.Contains(env, "crostini") {
		gbInfo = crostiniGeekbenchInfo
		if strings.Contains(env, "custom") {
			gbInfo.name = "crostini_custom"
		}
	} else {
		return geekbenchInfo{}
	}

	gbInfo.version = version
	gbInfo.needLicense = needLicense
	return gbInfo
}
