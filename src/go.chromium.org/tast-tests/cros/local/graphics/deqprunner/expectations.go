// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package deqprunner provides functions operating deqp-runner related binaries.
package deqprunner

import (
	"context"
	"io/ioutil"
	"os"
	"path/filepath"
	"strings"

	"go.chromium.org/tast-tests/cros/local/graphics/hardwareprobe"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

type runnerCategory string

var (
	// Deqp is to run deqprunner against deqp tests.
	Deqp runnerCategory = "deqp"
	// Piglit is to run deqprunner against piglit tests.
	Piglit runnerCategory = "piglit"
)

type runnerEnvironment string

var (
	// Host is running deqprunner in host environment.
	Host runnerEnvironment = ""
	// Borealis is running deqprunner in borealis environment.
	Borealis runnerEnvironment = "-borealis"
)

// CaseListFilters contain lists on their tests based on expectations
type CaseListFilters struct {
	Skips  []string
	Fails  []string
	Flakes []string
}

// MakeFilterCmd gathers tests based on expectations i.e. fails,flake,skips and return command line arguments.
func MakeFilterCmd(filters CaseListFilters, tmpDir string) ([]string, error) {
	var outCommand []string
	for _, t := range []struct {
		tests    []string
		filename string
		tag      string
	}{
		{filters.Fails, "/expected-fails.txt", "--baseline"},
		{filters.Flakes, "/known_flakes.txt", "--flakes"},
		{filters.Skips, "/skips.txt", "--skips"},
	} {
		file, err := os.Create(tmpDir + t.filename)
		defer file.Close()
		if err != nil {
			return outCommand, errors.Wrapf(err, "couldn't create file %v", t.filename)
		}
		file.WriteString(strings.Join(t.tests, "\n"))
		if err != nil {
			return outCommand, errors.New("could not write to file " + t.filename)
		}
		file.Sync()
		// Append the command example: --skips=path/to/skips.txt
		outCommand = append(outCommand, t.tag+"="+tmpDir+t.filename)
	}
	return outCommand, nil
}

// readExpectations reads expectations from all-chipsets and ${gpu} expectations files and return a combined list of expectations.
func readExpectations(ctx context.Context, category runnerCategory, gpu string, environment runnerEnvironment, suffix string) ([]string, error) {
	readLines := func(name string) ([]string, error) {
		path := filepath.Join("/usr/local/graphics/expectations/", string(category), name)
		out, err := ioutil.ReadFile(path)
		if err != nil {
			if errors.Is(err, os.ErrNotExist) {
				testing.ContextLogf(ctx, "No file found for %v, skip it", name)
				return []string{}, nil
			}
			return nil, errors.Wrapf(err, "failed to read %v", name)
		}
		testing.ContextLog(ctx, "Successfully read expectations for ", name)
		return strings.Split(strings.TrimSpace(string(out)), "\n"), nil
	}
	union := func(a, b []string) []string {
		m := make(map[string]bool)
		for _, item := range a {
			m[item] = true
		}
		for _, item := range b {
			if _, ok := m[item]; !ok {
				a = append(a, item)
			}
		}
		return a
	}

	var result []string
	// We compute an union of expectations common for all boards/chipsets expectations.
	testing.ContextLog(ctx, "Warning: Not attempting to load board-specific host expectations (rare)")
	for _, t := range []string{"all-chipsets", gpu} {
		fileName := t + string(environment) + suffix
		exp, err := readLines(fileName)
		if err != nil {
			return nil, err
		}
		result = union(result, exp)
	}
	return result, nil
}

// GetCaseListFilters reads the skip/fails/flakes filter list by probing the hardware information and returns list of expectations.
func GetCaseListFilters(ctx context.Context, category runnerCategory, environment runnerEnvironment) (CaseListFilters, error) {
	/* The expected fails list will be entries like
	   'dEQP-SUITE.test.name,Crash', such as you find in a failures.csv,
	   results.csv, or the "Some failures found:" stdout output of a previous
	   run.  Enter a test here when it has an expected state other than Pass or
	   Skip.

	   The skips list is a list of regexs to match test names to not run at
	   all. This is good for tests that are too slow or uninteresting to ever
	   see status for.

	   The flakes list is a list of regexes to match test names that may have
	   unreliable status.  Any unexpected result of that test will be marked
	   with the Flake status and not cause the test run to fail.  The subfolder
	   does automatic flake detection on its own to try to mitigate
	   intermittent failures, but even with that we can see too many spurious
	   failures in CI when run across many boards and many builds, so this lets
	   you run those tests while avoiding having them fail out CI runs until
	   the source of the flakiness can be resolved.

	   The primary source of board skip/flake/fails will be files in this test
	   directory under boards/, but we also list some common entries directly
	   in the code here to save repetition of the explanations.  The files may
	   contain empty lines or comments starting with '#'.

	   We could avoid adding filters for other apis than the one being tested,
	   but it's harmless to have unused tests in the lists and makes
	   copy-and-paste mistakes less likely.
	*/
	probeResult, err := hardwareprobe.GetHardwareProbeResult(ctx)
	if err != nil {
		return CaseListFilters{}, errors.Wrap(err, "failed to get hardware probe result")
	}
	targetGPU := probeResult.GPUInfo[0].Family
	// TODO(pwang): host may have two or more GPU (e.g. dGPU). Figure out the best action to handle multi-GPU cases.
	// Prefer NVIDIA GPU if we are testing borealis.
	if environment == Borealis {
		for _, gpuInfo := range probeResult.GPUInfo {
			if gpuInfo.Vendor == "nvidia" {
				targetGPU = gpuInfo.Family
				testing.ContextLogf(ctx, "Found NVIDIA GPU for testing borealis, loading %v expectations", targetGPU)
				break
			}
		}
	}

	skips, err := readExpectations(ctx, category, targetGPU, environment, "-skips.txt")
	if err != nil {
		return CaseListFilters{}, errors.Wrap(err, "failed to set up skips expectations")
	}
	fails, err := readExpectations(ctx, category, targetGPU, environment, "-fails.txt")
	if err != nil {
		return CaseListFilters{}, errors.Wrap(err, "failed to set up fails expectations")
	}
	flakes, err := readExpectations(ctx, category, targetGPU, environment, "-flakes.txt")
	if err != nil {
		return CaseListFilters{}, errors.Wrap(err, "failed to set up flakes expectations")
	}
	return CaseListFilters{skips, fails, flakes}, nil
}
