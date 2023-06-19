// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kdump

import (
	"context"
	"io/ioutil"
	"strings"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: ProbeKernelLimits,
		Desc: "Check if the kernel limits are set to zero",
		Contacts: []string{"chromeos-kdump@google.com",
			"ribalda@google.com"},
		BugComponent: "b:1193840",
		Attr:         []string{"group:mainline"},
	})
}

func ProbeKernelLimits(ctx context.Context, s *testing.State) {
	valBytes, err := ioutil.ReadFile("/proc/sys/kernel/kexec_load_disabled")
	if err != nil {
		s.Log("Kernel does not support kexec")
		return
	}
	val := strings.TrimSuffix(string(valBytes), "\n")
	if val != "1" {
		s.Log("kexec disabled via kexec_load_disabled")
		return
	}

	limits := []string{
		"kexec_load_limit_reboot",
		"kexec_load_limit_panic",
	}
	for _, limit := range limits {
		valBytes, err := ioutil.ReadFile("/proc/sys/kernel/" + limit)
		if err != nil {
			s.Fatal("Error opening "+limit, err)
		}

		val := strings.TrimSuffix(string(valBytes), "\n")
		if val != "0" {
			s.Fatalf("Invalid value of %s: %s instead of 0", limit, string(val))
		}
	}
}
