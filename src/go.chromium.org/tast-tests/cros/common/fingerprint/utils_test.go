// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import (
	"reflect"
	"testing"

	"github.com/stretchr/testify/assert"
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

func TestParseFpInfo(t *testing.T) {
	t.Run("Valid input", func(t *testing.T) {
		input := `Fingerprint sensor: vendor 20435046 product 9 model 1401 version 1
		Image: size 56x192 bpp 8
		Error flags:
		Dead pixels: UNKNOWN
		Templates: version 4 size 47616 count 0/5 dirty bitmap 0`
		expected := &FpInfo{
			FingerprintSensor: map[string]string{
				"vendor":  "20435046",
				"product": "9",
				"model":   "1401",
				"version": "1",
			},
			Image: map[string]string{
				"size": "56x192",
				"bpp":  "8",
			},
		}
		result, err := ParseFpInfo(input)
		assert.NoError(t, err)
		assert.Equal(t, expected, result)
	})

	t.Run("Missing Fingerprint sensor field", func(t *testing.T) {
		input := `Image: width:1080 height:1920`
		_, err := ParseFpInfo(input)
		assert.Error(t, err)
		assert.EqualError(t, err, "input does not have Fingerprint sensor field")
	})

	t.Run("Invalid Fingerprint sensor format", func(t *testing.T) {
		input := `Fingerprint sensor: name:goodix:invalid
Image: width:1080 height:1920`
		_, err := ParseFpInfo(input)
		assert.Error(t, err) // Assuming ParseSpaceDelimitedOutput returns an error for invalid format
	})

	t.Run("Invalid Image format", func(t *testing.T) {
		input := `Fingerprint sensor: name:goodix area:100
Image: width:1080:invalid`
		_, err := ParseFpInfo(input)
		assert.Error(t, err) // Assuming ParseSpaceDelimitedOutput returns an error for invalid format
	})
}
