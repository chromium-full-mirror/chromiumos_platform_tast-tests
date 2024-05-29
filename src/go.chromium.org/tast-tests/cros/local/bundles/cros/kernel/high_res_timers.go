// Copyright 2019 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package kernel

import (
	"bufio"
	"context"
	"os"
	"regexp"
	"strconv"

	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: HighResTimers,
		Desc: "Fails if timers have nanosecond resolution that is not 1 ns",
		Contacts: []string{
			"chromeos-kernel-test@google.com",
			"briannorris@chromium.org",
		},
		BugComponent: "b:167278", // ChromeOS > Platform > System > Kernel
		// b/263289152: There is an ongoing effort to disable highres timers for
		// power efficiency reasons and experiments shows that disabling highres
		// timers helps in power efficiency without hurting performance.
		// Disabling as it will be obsolete when the lowres is rolled out.
		Attr: []string{"group:mainline", "informational"},
	})
}

// HighResTimers reads from /proc/timer_list to verify that any resolution
// listed in nsecs has a value of 1. This is intended to catch issues with our
// ability to switch to high-resolution timers within the kernel.
func HighResTimers(ctx context.Context, s *testing.State) {
	re := regexp.MustCompile(`^\s*\.resolution:\s(\d+)\s*nsecs$`)

	f, err := os.Open("/proc/timer_list")
	if err != nil {
		s.Fatal("Failed to open timer list: ", err)
	}
	defer f.Close()

	scanner := bufio.NewScanner(f)
	for scanner.Scan() {
		if matches := re.FindStringSubmatch(scanner.Text()); matches != nil {
			res, err := strconv.Atoi(matches[1])
			if err != nil {
				s.Error("Error convering resolution to int: ", err)
			}
			if res != 1 {
				s.Errorf("Unexpected timer resoultion: %d ns, want 1 ns", res)
			}
		}
	}
	if scanner.Err(); err != nil {
		s.Error("Error reading timers file: ", err)
	}
}
