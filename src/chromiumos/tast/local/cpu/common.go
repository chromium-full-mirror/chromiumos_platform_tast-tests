// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package cpu

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/errors"
)

// GetNumberOfCPU returns total online CPU count from lscpu.
// Use this to retrieve the number of CPUs available
// on the DUT.
func GetNumberOfCPU(ctx context.Context) (int, error) {
	lscpu := testexec.CommandContext(ctx, "lscpu")
	out, err := lscpu.Output()
	if err != nil {
		return -1, errors.Wrap(err, "lscpu failed")
	}
	cpuRe := regexp.MustCompile(`^CPU\(s\):\s*(.*)$`)
	for _, line := range strings.Split(strings.TrimSpace(string(out)), "\n") {
		if !cpuRe.MatchString(line) {
			continue
		}
		cpus := cpuRe.FindStringSubmatch(line)
		ret, err := strconv.Atoi(cpus[1])
		if err != nil {
			return -1, errors.Wrap(err, "failed parsing number of CPUs")
		}
		return ret, nil
	}
	return -1, errors.New("can't find CPU(s) info in lscpu")
}
