// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package deqprunner provides functions operating deqp-runner related binaries.
package deqprunner

import (
	"fmt"
	"os"
	"path/filepath"
	"sort"
	"strings"

	"go.chromium.org/tast/core/errors"
)

// Result is the parsed result of the deqprunner.
type Result struct {
	Failures []string // List of tests that failed.
	Summary  string   // A short summary of the failures.
}

// Parse parse the failures.csv and returns the parsed Result.
func Parse(resultDir string) (Result, error) {
	result := Result{}
	out, err := os.ReadFile(filepath.Join(resultDir, "failures.csv"))
	if err != nil {
		return result, errors.Wrap(err, "failed to read failures.csv")
	}
	result.Failures = strings.Split(strings.TrimSpace(string(out)), "\n")
	sort.Strings(result.Failures)

	if len(result.Failures) == 0 {
		return result, errors.New("no failures in failures.csv")
	} else if len(result.Failures) == 1 {
		result.Summary = fmt.Sprintf("Failed test: %v", result.Failures[0])
		return result, nil
	} else if len(result.Failures) == 2 {
		result.Summary = fmt.Sprintf("Failed tests: %v", result.Failures)
		return result, nil
	}
	// 3+ failures.
	// We tried to count how many failures in first layer of deqp components. e.g. `3 tests in dEQP-VK.api, 16 tests in dEQP-VK.memory`
	counter := make(map[string]int)
	for _, test := range result.Failures {
		components := strings.Split(test, ".")
		prefix := strings.Join(components[:2], ".")
		counter[prefix]++
	}
	// Sort the keys so we have stable results.
	var keys []string
	for k := range counter {
		keys = append(keys, k)
	}
	sort.Strings(keys)

	var summary []string
	for _, k := range keys {
		summary = append(summary, fmt.Sprintf("%v failures in %v", counter[k], k))
	}
	result.Summary = strings.Join(summary, ", ") + " (see failures.csv)"
	return result, nil
}
