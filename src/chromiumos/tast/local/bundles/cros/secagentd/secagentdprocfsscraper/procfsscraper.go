// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package secagentdprocfsscraper contains shared code for scraping and parsing
// kernel procfs files.
package secagentdprocfsscraper

import (
	"fmt"
	"io/ioutil"
	"strings"

	"chromiumos/tast/errors"
)

// GetCmdLineParts returns a list of cmdline arguments for the given pid.
func GetCmdLineParts(pid uint64) ([]string, error) {
	cmdLine := fmt.Sprintf("/proc/%d/cmdline", pid)
	buff, err := ioutil.ReadFile(cmdLine)
	if err != nil {
		return nil, errors.Wrapf(err, "unable to read %s", cmdLine)
	}
	// Split on NUL and returns the parts.
	return strings.Split(string(buff), string(rune(0))), nil
}

// GetCmdLine returns a joined and quoted string of cmdline arguments for the
// given pid.
func GetCmdLine(pid uint64) (string, error) {
	parts, err := GetCmdLineParts(pid)
	if err != nil {
		return "", err
	}
	quotedParts := make([]string, len(parts))
	for _, part := range parts {
		quotedParts = append(quotedParts, fmt.Sprintf("'%s'", part))
	}
	return strings.Join(quotedParts, " "), nil
}
