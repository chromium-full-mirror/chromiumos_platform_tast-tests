// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package util

import (
	"context"
	"encoding/json"
	"path/filepath"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/dut"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
)

var (
	// Configs lists all supported fio configurations.
	Configs = []string{
		"seq_read",
		"seq_write",
		"tbw_probe",
		"16k_read",
		"16k_write",
		"4k_read",
		"4k_write",
		"4k_read_qd4",
		"4k_write_qd4",
		"surfing",
		"suspend_stress",
		"64k_stress",
	}
)

// TestConfig provides extra test configuration arguments.
type TestConfig struct {
	// Job is the name of the fio profile to execute. Must be on of the Configs.
	Job string

	// JobFile is the absolute path and filename of the fio profile file corresponding to Job.
	JobFile string

	// Path to the fio target
	Path string

	// Size of the data write
	Size int

	// RunTimeSec represents the run time of the test
	RunTimeSec int

	// ResultWriter references the result processing object.
	ResultWriter *FioResultWriter
}

const fioResultPath = "/tmp/fio_result"

// remoteJobFile returns the path where to store/read the job file file on the
// DUT.
func (t TestConfig) remoteJobFile() string {
	return filepath.Join("/tmp", t.Job)
}

// fioArgList produces the arguments to fio from the TestConfig params.
func (t TestConfig) fioArgList() []string {
	result := []string{
		t.remoteJobFile(),
		"--output-format=json",
		"--end_fsync=1",
		"--thread",
	}

	if t.Path != "" {
		result = append(result, "--filename="+t.Path)
	}

	if t.Size != 0 {
		result = append(result, "--size="+strconv.Itoa(t.Size))
	}

	if t.RunTimeSec != 0 {
		result = append(result, "--runtime="+strconv.Itoa(t.RunTimeSec)+"s")
	}

	return result
}

// WithJob sets Job in TestConfig.
func (t TestConfig) WithJob(job string) TestConfig {
	t.Job = job
	return t
}

// WithJobFile sets JobFile in TestConfig.
func (t TestConfig) WithJobFile(jobFile string) TestConfig {
	t.JobFile = jobFile
	return t
}

// WithJobFromFile sets JobFile in TestConfig.
func (t TestConfig) WithJobFromFile(jobFile string) TestConfig {
	return t.WithJobFile(jobFile).WithJob(filepath.Base(jobFile))
}

// WithPath sets Path in TestConfig.
func (t TestConfig) WithPath(path string) TestConfig {
	t.Path = path
	return t
}

// WithSize sets Size in TestConfig.
func (t TestConfig) WithSize(size int) TestConfig {
	t.Size = size
	return t
}

// WithRunTimeSec sets RunTimeSec in TestConfig.
func (t TestConfig) WithRunTimeSec(runtime int) TestConfig {
	t.RunTimeSec = runtime
	return t
}

// WithResultWriter sets ResultWriter in TestConfig.
func (t TestConfig) WithResultWriter(resultWriter *FioResultWriter) TestConfig {
	t.ResultWriter = resultWriter
	return t
}

// WithDisk sets Path and Size from Disk
func (t TestConfig) WithDisk(disk *Disk) TestConfig {
	return t.WithPath(disk.Path).WithSize(disk.Size)
}

// Run executes the test config on the specified DUT. It copies the job file,
// executes fio on the remote host and writes down the parsed result.
func (t TestConfig) Run(ctx context.Context, dut *dut.DUT) error {
	if err := validateJob(ctx, t.Job); err != nil {
		return errors.Wrap(err, "failed validating job")
	}

	paths := map[string]string{t.JobFile: t.remoteJobFile()}

	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), paths, linuxssh.DereferenceSymlinks); err != nil {
		return errors.Wrap(err, "failed to copy job file to remote host")
	}

	res, err := runFIO(ctx, dut, t.fioArgList())
	if err != nil {
		return errors.Wrapf(err, "%v failed", t.Job)
	}

	for _, j := range res.Jobs {
		if j.TotalError != 0 {
			return errors.Errorf("detected errors during FIO job %q, first error: %v", j.Jobname, j.FirstError)
		}
	}

	if t.ResultWriter != nil {
		t.ResultWriter.Report(res.DiskUtil[0].Name, res)
	}

	return nil
}

// RunBackground executes the test config on the specified DUT in the background.
// It copies the job file, executes fio on the remote host and writes the results
// to a tmp file. Returns the pid of the fio process.
func (t TestConfig) RunBackground(ctx context.Context, dut *dut.DUT) (string, error) {
	if err := validateJob(ctx, t.Job); err != nil {
		return "", errors.Wrap(err, "failed validating job")
	}

	paths := map[string]string{t.JobFile: t.remoteJobFile()}

	if _, err := linuxssh.PutFiles(ctx, dut.Conn(), paths, linuxssh.DereferenceSymlinks); err != nil {
		return "", errors.Wrap(err, "failed to copy job file to remote host")
	}

	args := "{ nohup fio " + strings.Join(t.fioArgList(), " ") + " > " + fioResultPath + " & } 2>/dev/null ; echo $!"
	pidFio, err := RunCmdWithStringOutput(ctx, dut, "bash", "-c", args)
	if err != nil {
		return "", errors.Wrapf(err, "%v failed", t.Job)
	}
	testing.ContextLogf(ctx, "fio pid is %s", pidFio)

	return pidFio, nil
}

// ParseFioResults reads the fio results file and checks for errors and writes the parsed results.
func (t TestConfig) ParseFioResults(ctx context.Context, dut *dut.DUT) error {
	out, err := RunCmdWithOutput(ctx, dut, "cat", fioResultPath)
	if err != nil {
		return errors.Wrap(err, "failed to read fio results, device likely rebooted")
	}

	res := &fioResult{}
	if err := json.Unmarshal(out, res); err != nil {
		return errors.Wrap(err, "failed to parse fio result")
	}

	for _, j := range res.Jobs {
		if j.TotalError != 0 {
			return errors.Errorf("detected errors during FIO job %q, first error: %v", j.Jobname, j.FirstError)
		}
	}

	if t.ResultWriter != nil {
		t.ResultWriter.Report(res.DiskUtil[0].Name, res)
	}

	return nil

}

// CleanupFio removes the fio results file.
func CleanupFio(ctx context.Context, dut *dut.DUT) {
	if _, err := RunCmdWithOutput(ctx, dut, "rm", fioResultPath); err != nil {
		testing.ContextLog(ctx, "Failed to cleanup fio results: ", err)
	}
}

func validateJob(ctx context.Context, job string) error {
	for _, config := range Configs {
		if job == config {
			return nil
		}
	}
	return errors.Errorf("job = %q, want one of %q", job, Configs)
}

// runFIO runs "fio" storage stress with the given args.
func runFIO(ctx context.Context, dut *dut.DUT, args []string) (*fioResult, error) {
	out, err := RunCmdWithOutput(ctx, dut, "fio", args...)
	if err != nil {
		return nil, errors.Wrap(err, "failed to run fio")
	}
	result := &fioResult{}
	if err := json.Unmarshal(out, result); err != nil {
		return nil, errors.Wrap(err, "failed to parse fio result")
	}
	return result, nil
}
