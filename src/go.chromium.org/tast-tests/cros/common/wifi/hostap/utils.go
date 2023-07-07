// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package hostap provides related utilities that are used both in
// common and remote packages.
package hostap

import (
	"fmt"
	"strings"
)

// EncodeSSID encodes ssid into the format that hostapd can read.
// The "%q" format in golang does not work for the case as it contains more
// escape sequence than what printf_decode in hostapd can understand.
// Duplicate here the logic of printf_encode in hostapd (original can be
// found at: https://w1.fi/cgit/hostap/tree/src/utils/common.c?h=hostap_2_10#n477)
func EncodeSSID(s string) string {
	var builder strings.Builder

	// Always start with 'P"' prefix as printf-encoded format in hostapd.
	builder.WriteString("P\"")
	for i := 0; i < len(s); i++ {
		switch s[i] {
		case '\\', '"':
			builder.WriteByte('\\')
			builder.WriteByte(s[i])
		case '\033':
			builder.WriteString("\\e")
		case '\n':
			builder.WriteString("\\n")
		case '\r':
			builder.WriteString("\\r")
		case '\t':
			builder.WriteString("\\t")
		default:
			if s[i] >= 32 && s[i] <= 126 {
				builder.WriteByte(s[i])
			} else {
				builder.WriteString(fmt.Sprintf("\\x%02x", s[i]))
			}
		}
	}
	// Close the format string.
	builder.WriteByte('"')
	return builder.String()
}
