// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cuj

import (
	"testing"
)

func TestRegistryBaseTestNames(t *testing.T) {
	for _, test := range Registry {
		for _, baseTestName := range test.BaseTestNames {
			if baseTestName == "" {
				continue
			}

			if _, ok := Registry[baseTestName]; !ok {
				t.Fatalf("Found invalid test metadata, base test %s doesn't exist", baseTestName)
			}
		}
	}
}

func TestRegistryTestNames(t *testing.T) {
	for testName, test := range Registry {
		if test.TestName == "" {
			continue
		}

		if testName != test.TestName {
			t.Fatalf("Found invalid test name; got %s, expected blank or %s", test.TestName, testName)
		}
	}
}

func TestRegistryMetrics(t *testing.T) {
	for _, test := range Registry {
		if len(test.BaseTestNames) == 0 && len(test.Metrics) == 0 {
			t.Fatalf("Found invalid test metrics registration, if there is no base test, recommended metrics must be present")
		}
	}
}

func TestRegistryNoDuplicateMetricsOrCycles(t *testing.T) {
	// Use DFS to find test cycles (a test references a base test that
	// references the original test as its own base test), and to ensure
	// metrics are referenced only once for each base test chain. For
	// example, DesksCUJ.battery_saver shouldn't have any overlapping metrics
	// with DesksCUJ.
	for testName, test := range Registry {
		stack := []string{testName}
		firstElt := true
		tested := map[string]bool{}
		for len(stack) != 0 {
			lenStack := len(stack)
			currentTestName := stack[lenStack-1]
			stack = stack[:lenStack-1]

			if !firstElt && currentTestName == testName {
				t.Fatalf("Found cycle for test %s", testName)
			}

			if _, ok := tested[currentTestName]; ok {
				continue
			}

			tested[currentTestName] = true

			currentTest := Registry[currentTestName]

			if len(currentTest.BaseTestNames) == 0 {
				continue
			}

			for _, parentTestName := range currentTest.BaseTestNames {
				stack = append(stack, parentTestName)
			}

			if firstElt {
				firstElt = false
				continue
			}

			for _, testMetricCurrent := range currentTest.Metrics {
				for _, testMetric := range test.Metrics {
					if testMetricCurrent == testMetric {
						t.Fatalf("Found duplicate test metric registration, metric %s is registered in %s, and also registered in a parent test %s", testMetric, testName, currentTestName)
					}
				}
			}

		}
	}
}
