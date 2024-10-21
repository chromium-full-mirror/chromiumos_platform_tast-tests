// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import (
	"reflect"
	"testing"
)

func TestParseColonDelimitedOutput(t *testing.T) {
	const ectoolVersionOutput = `

f1: A B C
	 f2	 : D E F
 f3 : G : H : I


`

	vals := ParseColonDelimitedOutput(ectoolVersionOutput)
	if keys := len(vals); keys != 3 {
		t.Fatalf("Wrong number of keys. Expected 3, but received %d keys.", keys)
	}
	for k, v := range vals {
		if v == "" {
			t.Fatalf("Key %s had blank value.", k)
		}
	}

	check := func(field, value string) {
		v, ok := vals[field]
		if !ok {
			t.Fatal("Missing field '" + field + "'.")
		}
		if vals[field] != value {
			t.Fatal("Field " + field + " contains invalid entry '" + v + "'.")
		}
		delete(vals, field)
	}

	check("f1", "A B C")
	check("f2", "D E F")
	check("f3", "G : H : I")

	if len(vals) != 0 {
		t.Fatal("Parsed extra values.")
	}
}

func TestParseSpaceDelimitedOutput(t *testing.T) {
	testCases := []struct {
		name      string
		input     string
		expected  map[string]string
		expectErr bool
	}{
		{
			"Even number of fields",
			"key1 value1 key2 value2",
			map[string]string{"key1": "value1", "key2": "value2"},
			false,
		},
		{
			"Odd number of fields",
			"key1 value1 key2",
			map[string]string{},
			true,
		},
		{
			"Empty string",
			"",
			map[string]string{},
			false,
		},
		{
			"Single key-value pair",
			"key1 value1",
			map[string]string{"key1": "value1"},
			false,
		},
	}

	for _, tc := range testCases {
		t.Run(tc.name, func(t *testing.T) {
			actual, err := ParseSpaceDelimitedOutput(tc.input)
			if tc.expectErr {
				if err == nil {
					t.Errorf("Expected an error, but got nil")
				}
			} else {
				if err != nil {
					t.Errorf("Unexpected error: %v", err)
				}
				if !reflect.DeepEqual(actual, tc.expected) {
					t.Errorf("Expected %v, but got %v", tc.expected, actual)
				}
			}
		})
	}
}
