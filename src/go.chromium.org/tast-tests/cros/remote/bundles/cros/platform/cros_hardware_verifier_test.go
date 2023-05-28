// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package platform

import (
	"bytes"
	"testing"
)

func TestDecodeResult(t *testing.T) {
	for i, tc := range []struct {
		encoded  string
		decoded  []byte
		hasError bool
	}{
		{ // All ASCII chars without a nul byte at the end.
			encoded:  "   array of bytes \"hello world\"\n",
			decoded:  []byte("hello world"),
			hasError: false,
		},
		{ // All ASCII chars with a nul byte at the end.
			encoded:  "   array of bytes \"hello world\"\\0\n",
			decoded:  []byte("hello world\x00"),
			hasError: false,
		},
		{ // Hex encoded bytes.
			encoded: `   array of bytes [
      24 3F 6A 88 85 A3 08 D3 13 19 8A 2E 03 70 73 44 A4 09 38 22 29 9F 31 D0
      08 2E FA 98 EC 4E 6C 89 45
   ]
`,
			decoded: []byte{
				0x24, 0x3F, 0x6A, 0x88, 0x85, 0xA3, 0x08, 0xD3, 0x13, 0x19, 0x8A,
				0x2E, 0x03, 0x70, 0x73, 0x44, 0xA4, 0x09, 0x38, 0x22, 0x29, 0x9F,
				0x31, 0xD0, 0x08, 0x2E, 0xFA, 0x98, 0xEC, 0x4E, 0x6C, 0x89, 0x45,
			},
			hasError: false,
		},
		{ // Non-hex encoded bytes.
			encoded: `   array of bytes [
      XX YY ZZ
   ]
`,
			decoded:  nil,
			hasError: true,
		},
		{ // Missing newline at the end of the string.
			encoded:  "   array of bytes \"hello world\"",
			decoded:  nil,
			hasError: true,
		},
		{ // incorrect format.
			encoded:  "random output",
			decoded:  nil,
			hasError: true,
		},
	} {
		decoded, err := decodeResult(tc.encoded)
		hasError := err != nil
		if bytes.Compare(decoded, tc.decoded) != 0 || hasError != tc.hasError {
			t.Errorf("testcase %d failed; input: %q; decoded (got: %v; want: %v); hasError (got: %v; want: %v)",
				i, tc.encoded, decoded, tc.decoded, hasError, tc.hasError)
		}
	}
}
