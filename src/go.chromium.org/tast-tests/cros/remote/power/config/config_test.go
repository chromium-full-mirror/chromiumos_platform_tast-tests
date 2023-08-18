// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package config

import (
	"encoding/json"
	"reflect"
	"sort"
	"strings"

	gotesting "testing"
)

func TestUnmarshalConfig(t *gotesting.T) {
	const fullConfig = exampleConfig
	config := Config{}
	if err := json.Unmarshal([]byte(fullConfig), &config); err != nil {
		t.Fatal("failed to unmarshal fullConfig: ", err)
	}

	expected := Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control: Control{
			MaxDuration:       60,
			Retry:             1,
			FailOnSkippedTest: false,
			FixedOrderTests:   []string{"power.Browsing", "power.VideoPlayback.1080p_vp9"},
		},
		Personas: []Persona{
			{
				Name: "MKT",
				Tests: []Test{
					{"power.Browsing", 0.6},
					{"power.VideoPlayback.1080p_vp9", 0.2},
					{"power.VideoPlayback.1080p_h264", 0.2},
				},
			},
			{
				Name: "EDU",
				Tests: []Test{
					{"power.Browsing", 0.4},
					{"power.VideoPlayback.1080p_vp9", 0.2},
					{"power.VideoPlayback.1080p_h264", 0.2},
					{"power.YoutubeArc.1080p", 0.2},
				},
			},
			{
				Name: "VideoPlayback",
				Tests: []Test{
					{"power.VideoPlayback.1080p_vp9", 0.5},
					{"power.VideoPlayback.1080p_h264", 0.5},
				},
			},
		},
	}
	if !reflect.DeepEqual(expected, config) {
		t.Errorf("wrong result of fullConfig unmarshaling: expect %v, got %v", expected, config)
	}

	const shortConfig = `
		{
			"format_version": 1,
			"config_name": "LowEndChromebook",
			"config_version": "1.0"
		}
	`
	config = Config{}
	if err := json.Unmarshal([]byte(shortConfig), &config); err != nil {
		t.Fatal("failed to unmarshal shortConfig: ", err)
	}
	expected = Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control:       Control{MaxDuration: 0, Retry: 0, FailOnSkippedTest: false},
	}
	if !reflect.DeepEqual(expected, config) {
		t.Errorf("wrong result of shortConfig unmarshaling: expect %v, got %v", expected, config)
	}
}

func TestValidateConfig(t *gotesting.T) {
	correctConfig := Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control: Control{
			MaxDuration:       60,
			Retry:             1,
			FailOnSkippedTest: false,
			FixedOrderTests:   []string{"power.Browsing", "power.VideoPlayback.1080p_vp9"},
		},
		Personas: []Persona{
			{
				Name: "MKT",
				Tests: []Test{
					{"power.Browsing", 0.6},
					{"power.VideoPlayback.1080p_vp9", 0.2},
					{"power.VideoPlayback.1080p_h264", 0.2},
				},
			},
			{
				Name: "EDU",
				Tests: []Test{
					{"power.Browsing", 0.4},
					{"power.VideoPlayback.1080p_vp9", 0.2},
					{"power.VideoPlayback.1080p_h264", 0.2},
					{"power.YoutubeArc.1080p", 0.2},
				},
			},
			{
				Name: "VideoPlayback",
				Tests: []Test{
					{"power.VideoPlayback.1080p_vp9", 0.5},
					{"power.VideoPlayback.1080p_h264", 0.5},
				},
			},
		},
	}
	tests, orderedTests, unorderedTests, err := ValidateConfig(&correctConfig)
	if err != nil {
		t.Fatal("failed to validate correct config; error:", err)
	}

	expected := []string{
		"power.Browsing",
		"power.VideoPlayback.1080p_vp9",
		"power.VideoPlayback.1080p_h264",
		"power.YoutubeArc.1080p",
	}
	sort.Strings(expected)
	if !reflect.DeepEqual(tests, expected) {
		t.Errorf("ValidateConfig returned wrong tests: expect %v, got %v", expected, tests)
	}

	orderedExpected := []string{"power.Browsing", "power.VideoPlayback.1080p_vp9"}
	if !reflect.DeepEqual(orderedTests, orderedExpected) {
		t.Errorf("ValidateConfig returned wrong ordered tests: expect %v, got %v",
			orderedExpected, orderedTests)
	}

	unorderedExpected := []string{"power.VideoPlayback.1080p_h264", "power.YoutubeArc.1080p"}
	if !reflect.DeepEqual(unorderedTests, unorderedExpected) {
		t.Errorf("ValidateConfig returned wrong unordered tests: expect %v, got %v",
			unorderedExpected, unorderedTests)
	}

	wrongFormatVersion := Config{
		FormatVersion: 2,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control:       Control{MaxDuration: 0, Retry: 0, FailOnSkippedTest: false},
		Personas: []Persona{{
			Name: "VideoPlayback",
			Tests: []Test{
				{"power.VideoPlayback.1080p_vp9", 0.5},
				{"power.VideoPlayback.1080p_h264", 0.5},
			},
		}},
	}
	if _, _, _, err = ValidateConfig(&wrongFormatVersion); err == nil {
		t.Error("ValidateConfig didn't return error for wrong format version")
	} else if !strings.Contains(err.Error(), "the format version is not supported") {
		t.Error("ValidateConfig returned an incorrect error for wrong format version; got:", err)
	}

	missingPersonas := Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control:       Control{MaxDuration: 0, Retry: 0, FailOnSkippedTest: false},
		Personas:      []Persona{},
	}
	if _, _, _, err = ValidateConfig(&missingPersonas); err == nil {
		t.Error("ValidateConfig didn't return error for missing personas")
	} else if !strings.Contains(err.Error(), "no personas are given") {
		t.Error("ValidateConfig returned an incorrect error for missing personas; got:", err)
	}

	missingTests := Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control:       Control{MaxDuration: 0, Retry: 0, FailOnSkippedTest: false},
		Personas: []Persona{{
			Name:  "MKT",
			Tests: []Test{},
		}},
	}
	if _, _, _, err = ValidateConfig(&missingTests); err == nil {
		t.Error("ValidateConfig didn't return error for missing tests")
	} else if !strings.Contains(err.Error(), "no tests are given for persona") {
		t.Error("ValidateConfig returned an incorrect error for missing tests; got:", err)
	}

	emptyTestName := Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control:       Control{MaxDuration: 0, Retry: 0, FailOnSkippedTest: false},
		Personas: []Persona{{
			Name: "MKT",
			Tests: []Test{
				{"power.Browsing", 0.5},
				{"", 0.5},
			},
		}},
	}
	if _, _, _, err = ValidateConfig(&emptyTestName); err == nil {
		t.Error("ValidateConfig didn't return error for empty test name")
	} else if !strings.Contains(err.Error(), "test name is empty") {
		t.Error("ValidateConfig returned an incorrect error for empty test name; got:", err)
	}

	duplicateTests := Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control:       Control{MaxDuration: 0, Retry: 0, FailOnSkippedTest: false},
		Personas: []Persona{{
			Name: "MKT",
			Tests: []Test{
				{"power.Browsing", 0.5},
				{"power.Browsing", 0.5},
			},
		}},
	}
	if _, _, _, err = ValidateConfig(&duplicateTests); err == nil {
		t.Error("ValidateConfig didn't return error for duplicate tests")
	} else if !strings.Contains(err.Error(), "duplicated test") {
		t.Error("ValidateConfig returned an incorrect error for duplicate tests; got:", err)
	}

	negativeWeight := Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control:       Control{MaxDuration: 0, Retry: 0, FailOnSkippedTest: false},
		Personas: []Persona{{
			Name: "MKT",
			Tests: []Test{
				{"power.Browsing", -1},
			},
		}},
	}
	if _, _, _, err = ValidateConfig(&negativeWeight); err == nil {
		t.Error("ValidateConfig didn't return error for negative weight")
	} else if !strings.Contains(err.Error(), "has negative weight") {
		t.Error("ValidateConfig returned an incorrect error for negative weight; got:", err)
	}

	unknownOrderedTests := Config{
		FormatVersion: 1,
		Name:          "LowEndChromebook",
		Version:       "1.0",
		Control: Control{
			MaxDuration:       0,
			Retry:             0,
			FailOnSkippedTest: false,
			FixedOrderTests:   []string{"power.DrainBattery"},
		},
		Personas: []Persona{{
			Name: "MKT",
			Tests: []Test{
				{"power.Browsing", 0.5},
				{"power.YoutubeArc.1080p", 0.5},
			},
		}},
	}
	if _, _, _, err = ValidateConfig(&unknownOrderedTests); err == nil {
		t.Error("ValidateConfig didn't return error for unknown ordered tests")
	} else if !strings.Contains(err.Error(), "fixed order test") {
		t.Error("ValidateConfig returned an incorrect error for unknown ordered tests; got:", err)
	}
}
