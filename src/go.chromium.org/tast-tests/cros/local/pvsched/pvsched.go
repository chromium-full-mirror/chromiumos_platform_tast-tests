// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

// Package pvsched provides wrappers for enabling/disabling the paravirt sched
// feature for tast tests.
package pvsched

import (
	"os"
	"strconv"
	"strings"
)

const kvmPvSchedPath = "/sys/module/kvm/parameters/kvm_pv_sched"

// Enable enables the paravirt sched feature
func Enable() error {
	return os.WriteFile(kvmPvSchedPath, []byte("1"), 0)
}

// Disable disables the paravirt sched feature
func Disable() error {
	return os.WriteFile(kvmPvSchedPath, []byte("0"), 0)
}

// Enabled returns true if paravirt feature is enabled, otherwise false.
func Enabled() (bool, error) {
	val, err := os.ReadFile(kvmPvSchedPath)
	if err != nil {
		return false, err
	}

	currPvSched, err := strconv.Atoi(strings.TrimSuffix(string(val), "\n"))
	if err != nil || (currPvSched != 1 && currPvSched != 0) {
		return false, err
	}

	return currPvSched == 1, nil
}
