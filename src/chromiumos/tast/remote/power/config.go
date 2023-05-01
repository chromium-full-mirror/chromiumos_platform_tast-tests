// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

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
	Retry             int32 `json:"retry"`
	MaxDuration       int64 `json:"max_duration"`
	FailOnSkippedTest bool  `json:"fail_on_skipped_test"`
}

// Config is the power test persona configuration.
type Config struct {
	FormatVersion int32     `json:"format_version"`
	Name          string    `json:"config_name"`
	Version       string    `json:"config_version"`
	Control       Control   `json:"test_control"`
	Personas      []Persona `json:"persona"`
}

// validateConfig validates the configuration and returns all the tests.
func validateConfig(c *Config) ([]string, error) {
	formatSupported := false
	for _, v := range supportedFormatVersions {
		if c.FormatVersion == v {
			formatSupported = true
			break
		}
	}
	if !formatSupported {
		return nil, errors.Errorf("the format version is not supported; want one of %v, got %d",
			supportedFormatVersions, c.FormatVersion)
	}

	if len(c.Personas) == 0 {
		return nil, errors.New("no personas are given")
	}

	allTests := make(map[string]bool)
	for _, persona := range c.Personas {
		if len(persona.Tests) == 0 {
			return nil, errors.Errorf("no tests are given for persona %q", persona.Name)
		}
		// Check test name duplication and weight.
		personaTests := make(map[string]bool)
		var totalWeight float64 = 0.0
		for _, t := range persona.Tests {
			if t.Name == "" {
				return nil, errors.Errorf("test name is empty in persona %q", persona.Name)
			}
			if personaTests[t.Name] {
				return nil, errors.Errorf("duplicated test %q is given for persona %q", t.Name, persona.Name)
			}
			personaTests[t.Name] = true
			allTests[t.Name] = true
			// Allow 0 weight, but not negative numbers.
			if t.Weight < 0 {
				return nil, errors.Errorf("test %s in persona %q has negative weight %s", t.Name, persona.Name,
					strconv.FormatFloat(t.Weight, 'f', -1, 64))
			}
			totalWeight += t.Weight
		}
		if totalWeight != 1 {
			return nil, errors.Errorf("total weight for tests in persona %q is not 1.0; got %s", persona.Name,
				strconv.FormatFloat(totalWeight, 'f', -1, 64))
		}
	}
	var tests []string
	for t := range allTests {
		tests = append(tests, t)
	}
	sort.Strings(tests)
	return tests, nil
}
