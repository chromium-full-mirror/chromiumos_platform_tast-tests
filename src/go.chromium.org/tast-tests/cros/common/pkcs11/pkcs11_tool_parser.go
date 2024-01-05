// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package pkcs11

import (
	"strings"

	"go.chromium.org/tast/core/errors"
)

// Section is a section in pkcs11-tool output
type section struct {
	// The header of the section
	header string

	// The keys inside the section (string-value pairs)
	keys map[string]string
}

// parsePkcs11ToolOutput parses a section-key-value style output of pkcs11-tool. It expects the following format:
// Section
//
//	Key1  :  Value1
//	Key2  :  Value2
//
// Any whitespace on keys or values is trimmed.
func parsePkcs11ToolOutput(data string) ([]section, error) {
	var result []section
	for _, s := range strings.Split(string(data), "\n") {
		if len(strings.TrimSpace(s)) == 0 {
			continue
		}
		if isSectionHeader(s) {
			result = append(result, section{})
			result[len(result)-1].header = s
			result[len(result)-1].keys = make(map[string]string)
		} else {
			if len(result) < 1 {
				return nil, errors.Errorf("non-section entry outside of section: %q", s)
			}
			n := strings.Index(s, ":")
			if n == -1 {
				return nil, errors.Errorf("failed to parse section entry %q", s)
			}
			key := strings.TrimSpace(s[:n])
			value := strings.TrimSpace(s[n+1:])
			result[len(result)-1].keys[key] = value
		}
	}
	return result, nil
}

func isSectionHeader(s string) bool {
	// Assume that the string is ASCII and that indenting whitespace characters could be space or tab.
	return len(s) > 0 && s[0] != ' ' && s[0] != '\t'
}
