// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package config defines the power qual run configuration.
package config

import (
	"sort"
	"strconv"

	"go.chromium.org/tast/core/errors"
)

// supportedFormatVersions contains a list of supported version number.
var supportedFormatVersions = []int32{1}

// Test has the information for a test in a power test persona.
type Test struct {
	Name   string  `json:"name"`
	Weight float64 `json:"weight"`
}

// Persona has the inforamtion for one power test persona.
type Persona struct {
	Name  string `json:"name"`
	Tests []Test `json:"tests"`
}

// Control has the test control information for power tests.
type Control struct {
	// Retry specifies the retry number for failed tests.
	Retry int32 `json:"retry"`
	// MaxDuration specifies the max test duration in minutes.
	MaxDuration int64 `json:"max_duration"`
	// FailOnSkippedTest indicates whether to fail if there are skipped tests.
	FailOnSkippedTest bool `json:"fail_on_skipped_test"`
	// FixedOrderTests specifies the tests that need to be run in the given
	// order. These tests will be run before other tests.
	FixedOrderTests []string `json:"fixed_order_tests"`
}

// Config is the power test persona configuration.
type Config struct {
	FormatVersion int32     `json:"format_version"`
	Name          string    `json:"config_name"`
	Version       string    `json:"config_version"`
	Control       Control   `json:"test_control"`
	Personas      []Persona `json:"persona"`
}

// ValidateConfig validates the configuration and returns all the tests.
// Tests are grouped into ordered tests and unordered tests.
func ValidateConfig(c *Config) (tests, orderedTests, unorderedTests []string, err error) {
	formatSupported := false
	for _, v := range supportedFormatVersions {
		if c.FormatVersion == v {
			formatSupported = true
			break
		}
	}
	if !formatSupported {
		err = errors.Errorf("the format version is not supported; want one of %v, got %d",
			supportedFormatVersions, c.FormatVersion)
		return nil, nil, nil, err
	}

	if len(c.Personas) == 0 {
		err = errors.New("no personas are given")
		return nil, nil, nil, err
	}

	allTests := make(map[string]bool)
	for _, persona := range c.Personas {
		if len(persona.Tests) == 0 {
			err = errors.Errorf("no tests are given for persona %q", persona.Name)
			return nil, nil, nil, err
		}
		// Check test name duplication and weight.
		personaTests := make(map[string]bool)
		for _, t := range persona.Tests {
			if t.Name == "" {
				err = errors.Errorf("test name is empty in persona %q", persona.Name)
				return nil, nil, nil, err
			}
			if personaTests[t.Name] {
				err = errors.Errorf("duplicated test %q is given for persona %q", t.Name, persona.Name)
				return nil, nil, nil, err
			}
			personaTests[t.Name] = true
			allTests[t.Name] = true
			// Allow 0 weight, but not negative numbers.
			if t.Weight < 0 {
				err = errors.Errorf("test %s in persona %q has negative weight %s", t.Name, persona.Name,
					strconv.FormatFloat(t.Weight, 'f', -1, 64))
				return nil, nil, nil, err
			}
		}
	}

	testsInOrder := make(map[string]bool)
	for _, t := range c.Control.FixedOrderTests {
		testsInOrder[t] = true
	}

	for t := range allTests {
		if !testsInOrder[t] {
			unorderedTests = append(unorderedTests, t)
		}
		tests = append(tests, t)
	}

	// Sort all tests and unordered tests alphabetically for easy lookup if printed in logs.
	sort.Strings(tests)
	sort.Strings(unorderedTests)
	orderedTests = c.Control.FixedOrderTests

	return tests, orderedTests, unorderedTests, nil
}

// FindWeightedTests finds the weighted in config file.
func FindWeightedTests(c *Config) map[string]bool {
	weightedTests := make(map[string]bool)

	// Find all the tests with weight > 0.
	for _, persona := range c.Personas {
		for _, test := range persona.Tests {
			if test.Weight > 0 {
				weightedTests[test.Name] = true
			}
		}
	}

	return weightedTests
}
