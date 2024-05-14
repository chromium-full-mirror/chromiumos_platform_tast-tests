// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kernel

import (
	"context"
	"io/ioutil"
	"strconv"
	"strings"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: StackEntropy,
		Desc: "Verifies that kernel stack entropy is sufficiently high",
		Contacts: []string{
			"chromeos-hardening@google.com",
			"chromeos-kernel-test@google.com",
			"swboyd@chromium.org",
		},
		// ChromeOS > Security > Hardening
		BugComponent: "b:1040049",
		Attr:         []string{"group:mainline", "informational", "group:criticalstaging"},
		// TODO(b/201790026): The lkdtm resides on the debugfs,
		// which is not accessible when integrity mode is
		// enabled.
		//
		// We either need to refactor the test not to use lkdtm
		// to trigger crashes or we need to modify the kernel to
		// allow access to the required path. Skip on reven for
		// now, since reven uses integrity mode.
		HardwareDeps: hwdep.D(hwdep.SkipOnModel("reven")),
		SoftwareDeps: []string{"kstack_random"},
	})
}

func StackEntropy(ctx context.Context, s *testing.State) {
	const lkdtm = "/sys/kernel/debug/provoke-crash/DIRECT"

	for i := 0; i < 1000; i++ {
		if err := ioutil.WriteFile(lkdtm, []byte("REPORT_STACK"), 0); err != nil {
			s.Fatal("Failed to report kernel stack offset with lkdtm: ", err)
		}
	}

	// Measure stack entropy by counting the unique stack offsets and converting that to a number of bits.
	script := `echo "obase=2; $(dmesg | grep -m1 -A10000 'Starting stack offset' | grep 'Stack offset' | awk '{print $NF}' | sort | uniq -c | wc -l)" | bc | wc -L`

	out, err := testexec.CommandContext(ctx, "sh", "-c", script).Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to calculate kernel stack entropy: ", err)
	}

	bits, err := strconv.ParseInt(strings.TrimSuffix(string(out), "\n"), 10, 64)
	if err != nil {
		s.Fatal("Failed to convert bits to integer: ", err)
	}

	if bits < 5 {
		s.Fatalf("Kernel stack entropy %v is too low", bits)
	}

	s.Log("Kernel stack entropy: ", bits)
}
