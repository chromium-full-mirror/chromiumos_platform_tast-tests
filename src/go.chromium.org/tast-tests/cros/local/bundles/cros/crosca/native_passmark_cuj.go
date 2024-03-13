// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crosca

import (
	"context"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/sysutil"
	"go.chromium.org/tast-tests/cros/local/ui/cujrecorder"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/fsutil"
	"go.chromium.org/tast/core/testing"
	"gopkg.in/yaml.v2"
)

const (
	workspaceDir   = "/usr/local/passmark_cuj"
	libncurses     = "libncurses.so.5"
	libstdc        = "libstdc++.so.6"
	libtinfo       = "libtinfo.so.5"
	libraryPath64  = "/usr/local/lib64/"
	libraryPath32  = "/usr/local/lib/"
	passmarkX86_64 = "x86_64"
	passmarkArm64  = "arm64"
	passmarkArm32  = "arm32"
	passmarkBinary = "pt_linux"
	passmarkPrefix = "Benchmark.Passmark."
	resultFileName = "results_all.yml"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         NativePassmarkCUJ,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Run Passmark natively to test device performance",
		Contacts: []string{
			"chromeos-competitive-analysis@google.com",
			"cienet-development@googlegroups.com",
			"alstonhuang@google.com",
		},
		BugComponent: "b:1485133", // ChromeOS > Platform > baseOS > Performance > Competitive Analysis
		SoftwareDeps: []string{"chrome"},
		Data: []string{
			"libncurses.so.5_arm32",
			"libstdc.so.6_arm32",
			"libtinfo.so.5_arm32",
			"pt_linux_arm32",
			"libncurses.so.5_arm64",
			"libstdc.so.6_arm64",
			"libtinfo.so.5_arm64",
			"pt_linux_arm64",
			"libncurses.so.5_x86_64",
			"libstdc.so.6_x86_64",
			"libtinfo.so.5_x86_64",
			"pt_linux_x86_64",
		},
		Fixture: "loggedInToCUJUserWithoutCooldown",
		Timeout: 15*time.Minute + cujrecorder.CooldownTimeout,
	})
}

func NativePassmarkCUJ(ctx context.Context, s *testing.State) {
	if _, err := os.Stat(workspaceDir); !os.IsNotExist(err) {
		testing.ContextLogf(ctx, "Workspace directory %s already exists, removing", workspaceDir)
		if err := os.RemoveAll(workspaceDir); err != nil {
			s.Fatal("Failed to remove workspace directory: ", err)
		}
	}
	if err := os.Mkdir(workspaceDir, 0755); err != nil {
		s.Fatal("Failed to create workspace directory: ", err)
	}
	defer os.RemoveAll(workspaceDir)

	suffix, err := getFileNameSuffixByArch(ctx)
	if err != nil {
		s.Fatal("Failed to get file name suffix: ", err)
	}
	execName := fmt.Sprintf("%s_%s", passmarkBinary, suffix)
	execFilePath := filepath.Join(workspaceDir, execName)
	libstdcName := fmt.Sprintf("%s_%s", strings.ReplaceAll(libstdc, "+", ""), suffix)
	libncursesName := fmt.Sprintf("%s_%s", libncurses, suffix)
	libtinfoName := fmt.Sprintf("%s_%s", libtinfo, suffix)
	pmFiles := map[string]string{
		s.DataPath(execName):       execFilePath,
		s.DataPath(libstdcName):    filepath.Join(workspaceDir, libstdc),
		s.DataPath(libncursesName): filepath.Join(workspaceDir, libncurses),
		s.DataPath(libtinfoName):   filepath.Join(workspaceDir, libtinfo),
	}

	for srcFilePath, dstFilePath := range pmFiles {
		if err := fsutil.CopyFile(srcFilePath, dstFilePath); err != nil {
			s.Fatal("Failed to push files: ", err)
		}
		defer os.Remove(dstFilePath)

		if err := os.Chmod(dstFilePath, 0755); err != nil {
			s.Fatal("Failed to change execute permission: ", err)
		}
	}

	// Cool the system before running the benchmark.
	cujrecorder.WaitForCPUStabilization(ctx)

	execDir := filepath.Dir(execFilePath)
	cmd := testexec.CommandContext(ctx, execFilePath, "-r", "3")
	cmd.Env = append(os.Environ(), "LD_LIBRARY_PATH="+workspaceDir, "TERM=xterm")
	cmd.Dir = execDir
	_, err = cmd.Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to execute Passmark: ", err)
	}

	outDir, ok := testing.ContextOutDir(ctx)
	if !ok || outDir == "" {
		s.Fatal("Failed to get the out directory: ", err)
	}

	resultPath := filepath.Join(execDir, resultFileName)
	logFilePath := filepath.Join(outDir, resultFileName)
	if err := fsutil.MoveFile(resultPath, logFilePath); err != nil {
		s.Fatal("Failed to get result file from DUT: ", err)
	}

	scores, err := retrieveScore(ctx, logFilePath)
	if err != nil {
		s.Fatal("Failed to retrieve Passmark score: ", err)
	}

	pv := perf.NewValues()
	for metric, value := range scores {
		pv.Set(perf.Metric{
			Name:      metric,
			Unit:      "score",
			Direction: perf.BiggerIsBetter,
		}, value)
	}

	if err := pv.Save(outDir); err != nil {
		s.Fatal("Failed to save the performance data: ", err)
	}
}

// getFileNameSuffixByArch reads the sys arch and returns the corresponding file suffix.
func getFileNameSuffixByArch(ctx context.Context) (string, error) {
	_, arch, err := sysutil.KernelVersionAndArch()
	if err != nil {
		return "", errors.Wrap(err, "failed to get system arch")
	}

	var suffix string
	switch arch {
	case "x86_64":
		suffix = passmarkX86_64
	case "aarch64":
		fileCmd := testexec.CommandContext(ctx, "file", "/sbin/init")
		b, err := fileCmd.Output(testexec.DumpLogOnError)
		if err != nil {
			return "", errors.Wrap(err, "failed to determine the architecture bit width")
		}
		re := regexp.MustCompile(`[0-9]+-bit`)
		switch bit := re.FindString(string(b)); bit {
		case "64-bit":
			suffix = passmarkArm64
		case "32-bit":
			suffix = passmarkArm32
		default:
			return "", errors.Errorf("unsupported architecture bit width: %s", bit)
		}
	default:
		return "", errors.Errorf("unsupported architecture: %s", arch)
	}

	return suffix, nil
}

// retrieveScore formats the metric values for Passmark and inputs them in a map.
func retrieveScore(ctx context.Context, filePath string) (map[string]float64, error) {
	bytes, err := os.ReadFile(filePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read the results")
	}

	var data map[string]interface{}
	if err := yaml.Unmarshal(bytes, &data); err != nil {
		return nil, err
	}

	// results holds the raw test result. Example:
	// Results:
	//   Results Complete: true
	//   NumTestProcesses: 4
	//   CPU_INTEGER_MATH: 19043.004666666668
	//   CPU_FLOATINGPOINT_MATH: 9920.4128308582985
	//   CPU_PRIME: 12.566251367856644
	//   CPU_SORTING: 4817.0298504896327
	//   CPU_ENCRYPTION: 2989.6424822154941
	//   CPU_COMPRESSION: 37635.73029942975
	//   CPU_SINGLETHREAD: 1713.4659170469877
	//   CPU_PHYSICS: 270.97116085349921
	//   CPU_MATRIX_MULT_SSE: 1681.4497940634781
	//   CPU_mm: 372.20504564690111
	//   CPU_sse: 1246.7030064217065
	//   CPU_fma: 2337.5469819223877
	//   CPU_avx: 1460.0993938463398
	//   CPU_avx512: 0
	scores := make(map[string]float64)
	for k, v := range data["Results"].(map[interface{}]interface{}) {
		// Keep the results where the value type is a integer or a float.
		if subtest, ok := k.(string); ok {
			if score, ok := v.(float64); ok {
				scores[passmarkPrefix+subtest] = score
			} else if score, ok := v.(int); ok {
				scores[passmarkPrefix+subtest] = float64(score)
			}
		}
	}

	return scores, nil
}
