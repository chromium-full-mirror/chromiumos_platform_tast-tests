// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package crash

import (
	"context"
	"io/ioutil"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         KernelLogCollector,
		Desc:         "Verify that kernel_log_collector.sh can read logs",
		Contacts:     []string{"chromeos-data-eng@google.com", "tbrandston@google.com"},
		BugComponent: "b:1032705",
		Attr:         []string{"group:mainline", "informational"},
	})
}

func KernelLogCollector(ctx context.Context, s *testing.State) {
	s.Log("Triggering kernel log")

	// Trigger a kernel 'sysrq HELP' log.
	ioutil.WriteFile("/proc/sysrq-trigger", []byte("?"), 0666)

	s.Log("Using kernel_log_collector.sh to find logs")

	// limit to things matching "sysrq" from the last 30 seconds.
	logsOutput, err := testexec.CommandContext(ctx, "/usr/sbin/kernel_log_collector.sh", "sysrq", "30").Output()
	if err != nil {
		s.Error("Failed to run kernel_log_collector.sh sysrq 30: ", err)
	}

	// Not sure what the best thing to look for would be, but this should be part
	// of the HELP message and is unlikely to be removed/renamed.
	if !strings.Contains(string(logsOutput[:]), "reboot(b)") {
		s.Error("sysrq help message not found: ", logsOutput)
	}
}
