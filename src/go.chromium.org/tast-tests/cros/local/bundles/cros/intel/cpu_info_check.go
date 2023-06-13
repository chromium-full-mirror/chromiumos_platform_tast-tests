// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CPUInfoCheck,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies CPU info test",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291",
		Attr:         []string{"group:intel-nda"},
		SoftwareDeps: []string{"chrome"},
		HardwareDeps: hwdep.D(hwdep.Model("rex")),
	})
}

func CPUInfoCheck(ctx context.Context, s *testing.State) {
	// cmdOutput exceutes CPU commands and returns output.
	cmdOutput := func(ctx context.Context, cmd string) string {
		out, err := testexec.CommandContext(ctx, "sh", "-c", cmd).Output(testexec.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to exceute %q command: %v", cmd, err)
		}
		return string(out)
	}

	const (
		intelHfiCommand   = "cat /proc/cpuinfo | grep hfi"
		intelHfiWcCommand = "cat /proc/cpuinfo | grep hfi | wc -l"
		lscpuCommand      = `lscpu | grep "CPU(s):"`
	)

	intelHfiOut := cmdOutput(ctx, intelHfiCommand)
	re := regexp.MustCompile(`hfi`)
	results := re.FindAllString(intelHfiOut, -1)
	intelHfiCount := len(results)

	hfiCountOut := cmdOutput(ctx, intelHfiWcCommand)
	intelHfiWordCount, err := strconv.Atoi(strings.TrimSpace(hfiCountOut))
	if err != nil {
		s.Fatal("Failed to convert hfiCount to integer: ", err)
	}

	lscpuOut := cmdOutput(ctx, lscpuCommand)
	res := strings.Trim(lscpuOut, "CPU(s): ")
	cpuCount, err := strconv.Atoi(strings.TrimSpace(res))
	if err != nil {
		s.Fatal("Failed to convert lscpu output to integer: ", err)
	}

	if intelHfiCount != intelHfiWordCount {
		s.Fatalf("Failed: intelHfiWordCount and intelHfiCount must be equal; got: %v, want: %v", intelHfiWordCount, intelHfiCount)
	}

	if intelHfiCount != cpuCount {
		s.Fatalf("Failed: cpuCount and intelHfiCount must be equal; got: %v, want: %v", cpuCount, intelHfiCount)
	}
}
