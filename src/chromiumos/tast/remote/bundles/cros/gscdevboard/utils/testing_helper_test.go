// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package utils

import (
	"testing"
)

func TestCrc8(t *testing.T) {
	tests := []struct {
		want  byte
		input []byte
	}{
		{want: 0x00, input: []byte{}},
		{want: 0x00, input: []byte{0}},
		{want: 0xF3, input: []byte{0xFF}},
		{want: 0xCA, input: []byte{0xDE, 0xAD, 0xBE, 0xEF}},
		{want: 0x7B, input: []byte{0xAA, 0x55, 0x5A, 0xA5}},
		{want: 0x00, input: []byte{0x00, 0x00, 0x00, 0x00}},
		{want: 0xDE, input: []byte{0xFF, 0xFF, 0xFF, 0xFF}},
		{want: 0x85, input: []byte{0x01, 0x02, 0x03, 0x04, 0x05, 0x06, 0x07, 0x08, 0x09}},
	}
	for _, tc := range tests {
		got := Crc8(tc.input)
		if got != tc.want {
			t.Errorf("Want 0x%X, but got 0x%X for %v", tc.want, got, tc.input)
		}
	}
}
