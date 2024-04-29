// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package croscalinux

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path"
	"time"

	"go.chromium.org/tast-tests/cros/common/chrome/credconfig"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

// DefaultDirPath specifies the location of the "cros_ca_linux" directory.
// It assumes that users execute tests inside the chroot.
const DefaultDirPath = "/mnt/host/source/src/platform/dev/contrib/cros_ca_linux"
const testEndFilePath = "/logs/.test_ended.txt"

// Run represents the execution result of the python program.
type Run struct {
	RunID     string  `json:"run_id"`
	EndTime   float64 `json:"end_time"`
	Error     string  `json:"error"`
	ResultDir string  `json:"result_dir"`
}

// GenerateCommand generates commands for python test program execution.
func GenerateCommand(dut, test, resultDir, creds string) ([]string, error) {
	args := []string{
		"/bin/bash",
		"-c",
		`source ./script/setup_venv.sh . && python ./bin/test_cros_remote.py "$0" "$@"`,
		dut,
		test,
		fmt.Sprintf("--result_dir=%s", resultDir),
	}

	if creds != "" {
		cs, err := credconfig.ParseCreds(creds)
		if err != nil {
			return nil, errors.Wrap(err, "failed to parse creds")
		}
		args = append(args, fmt.Sprintf("--var=gaia_account=%s", cs[0].User))
		args = append(args, fmt.Sprintf("--var=gaia_password=%s", cs[0].Pass))
	}

	return args, nil
}

// GetResultDirPath checks if the python program is ended.
// If yes, the test execution result will be recoded in .test_ended.txt.
// |result_dir| indicates where the test results are located.
func GetResultDirPath(ctx context.Context, projectDir string, timeout time.Duration) (string, error) {
	testEndFilePath := path.Join(projectDir, testEndFilePath)
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(testEndFilePath); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: timeout}); err != nil {
		return "", errors.Wrap(err, "failed to find .test_ended.txt")
	}

	b, err := os.ReadFile(testEndFilePath)
	if err != nil {
		return "", errors.Wrap(err, "failed to read .test_ended.txt")
	}

	var run Run
	if err := json.Unmarshal(b, &run); err != nil {
		return "", errors.Wrap(err, "failed to parse test results")
	}
	return run.ResultDir, nil
}

// Result indicates the result of each test.
type Result struct {
	Name            string `json:"name"`
	Location        string `json:"location"`
	Result          string `json:"result"`
	Error           string `json:"error"`
	RemoteResult    string `json:"remote_result"`
	RemoteError     string `json:"remote_error"`
	RemoteStartTime string `json:"remote_start_time"`
	RemoteEndTime   string `json:"remote_end_time"`
	LocalResult     string `json:"local_result"`
	LocalError      string `json:"local_error"`
	LocalStartTime  string `json:"local_start_time"`
	LocalEndTime    string `json:"local_end_time"`
}

// ParseResultsJSON parses results.json inside dir.
func ParseResultsJSON(ctx context.Context, dir string) ([]Result, error) {
	resultFilePath := path.Join(dir, "results.json")
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if _, err := os.Stat(resultFilePath); err != nil {
			return err
		}
		return nil
	}, &testing.PollOptions{Timeout: time.Minute}); err != nil {
		return nil, errors.Wrap(err, "failed to find results.json")
	}

	b, err := os.ReadFile(resultFilePath)
	if err != nil {
		return nil, errors.Wrap(err, "failed to read results.json")
	}

	var results []Result
	if err := json.Unmarshal(b, &results); err != nil {
		return nil, errors.Wrap(err, "failed to parse test results")
	}
	return results, nil
}
