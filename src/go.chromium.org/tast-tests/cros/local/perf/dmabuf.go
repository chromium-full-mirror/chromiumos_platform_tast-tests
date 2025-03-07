// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package perf

import (
	"bufio"
	"os"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast/core/errors"
)

const (
	// Path to the debugfs's dmabuf usage info file.
	dmabufInfoPath = "/sys/kernel/debug/dma_buf/bufinfo"
)

// The last line of bufinfo file is of the following pattern:
// Total <total_buffers> objects, <total_size> bytes
var dmabufInfoRegex = regexp.MustCompile(`Total (\d+) object[s]?, (\d+) byte[s]?`)

// GetDMABufUsage retrieves the number of buffers used and the total bytes used as DMA buffer.
func GetDMABufUsage() (totalBufferCount, totalBufferSize int64, err error) {
	infoFileContents, err := os.ReadFile(dmabufInfoPath)
	if err != nil {
		return -1, -1, errors.Wrap(err, "failed to get dmabuf info")
	}
	var lastLine string
	scanner := bufio.NewScanner(strings.NewReader(string(infoFileContents[:])))
	for scanner.Scan() {
		lastLine = scanner.Text()
	}
	matches := dmabufInfoRegex.FindStringSubmatch(lastLine)
	if matches == nil {
		return -1, -1, errors.Wrapf(err, "failed to parse dmabuf info from %q", string(lastLine))
	}
	totalBufferCount, err = strconv.ParseInt(matches[1], 10, 64)
	if err != nil {
		return -1, -1, errors.Wrapf(err, "failed to convert total buffer count %v to an integer", matches[1])
	}
	totalBufferSize, err = strconv.ParseInt(matches[2], 10, 64)
	if err != nil {
		return -1, -1, errors.Wrapf(err, "failed to convert total buffer size %v to an integer", matches[2])
	}

	return totalBufferCount, totalBufferSize, nil
}
