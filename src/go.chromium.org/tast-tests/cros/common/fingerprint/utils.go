// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package fingerprint

import (
	"strings"

	"go.chromium.org/tast/core/errors"
)

// FpInfo is a struct that contains the information returned by running fpinfo.
type FpInfo struct {
	FingerprintSensor map[string]string
	Image             map[string]string
}

// ParseColonDelimitedOutput parses colon delimited information to a map.
func ParseColonDelimitedOutput(output string) map[string]string {
	ret := map[string]string{}
	for _, line := range strings.Split(output, "\n") {
		// Note that the ectool version build info line uses ':'s as time of
		// date delimiters.
		splits := strings.SplitN(line, ":", 2)
		if len(splits) != 2 {
			continue
		}
		ret[strings.TrimSpace(splits[0])] = strings.TrimSpace(splits[1])
	}
	return ret
}

// ParseSpaceDelimitedOutput parses space delimited information in pairs to a map.
//
//	expects this format: "key1 value1 key2 value2 ..."
func ParseSpaceDelimitedOutput(output string) (map[string]string, error) {
	// Check that output has even number of fields
	fields := strings.Fields(output)
	if len(fields)%2 == 1 {
		return nil, errors.New("input has odd number of fields")
	}
	ret := map[string]string{}
	for i := 0; i < len(fields); i += 2 {
		ret[fields[i]] = fields[i+1]
	}
	return ret, nil
}

// ParseFpInfo returns a FpInfo struct of the information returned by running fpinfo.
func ParseFpInfo(input string) (*FpInfo, error) {
	outparse := ParseColonDelimitedOutput(input)
	// TODO(b/378254619): make this into a forloop which maps all fields of fpinfo.
	if outparse["Fingerprint sensor"] == "" {
		return nil, errors.New("input does not have Fingerprint sensor field")
	}
	ret := &FpInfo{}
	val, err := ParseSpaceDelimitedOutput(outparse["Fingerprint sensor"])
	if err != nil {
		return nil, err
	}
	ret.FingerprintSensor = val

	val, err = ParseSpaceDelimitedOutput(outparse["Image"])
	if err != nil {
		return nil, err
	}
	ret.Image = val

	return ret, nil
}
