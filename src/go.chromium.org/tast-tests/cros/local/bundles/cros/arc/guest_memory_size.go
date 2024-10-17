// Copyright 2024 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package arc

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/local/arc"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         GuestMemorySize,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verify ARCVM boots with >3GiB of memory",
		Contacts: []string{
			"cros-vm-technology@google.com",
		},
		// ChromeOS > Platform > baseOS > Virtualization
		BugComponent: "b:882513",
		Attr:         []string{"group:mainline", "informational", "group:hw_agnostic", "group:criticalstaging"},
		SoftwareDeps: []string{"chrome", "android_vm"},
		Fixture:      "arcBooted",
		Timeout:      time.Minute,
	})
}

func GuestMemorySize(ctx context.Context, s *testing.State) {
	a := s.FixtValue().(*arc.PreData).ARC

	output, err := a.Command(ctx, "cat", "/proc/meminfo").Output(testexec.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to get guest meminfo: ", err)
	}

	re := regexp.MustCompile(`MemTotal:\s*(\d*) kB`)
	groups := re.FindStringSubmatch(string(output))
	if len(groups) != 2 {
		s.Fatal("Failed to find MemTotal")
	}

	memTotalKiB, err := strconv.Atoi(groups[1])
	if err != nil {
		s.Fatal("Failed to parse MemTotal: ", err)
	}

	// Certain apps require >= 3GiB to be found in the Play Store
	if memTotalKiB < 3*1024*1024 {
		s.Fatalf("Guest memory of size %d is too small", memTotalKiB)
	}
}
