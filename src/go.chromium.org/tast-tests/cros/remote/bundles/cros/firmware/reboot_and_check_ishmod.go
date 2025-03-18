// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RebootAndCheckIshmod,
		Desc: "Verify that the ISH comes on after every warm reboot",
		Contacts: []string{
			"chromeos-faft@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:dsp", "dsp_ish"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.ChromeISH()),
		Params: []testing.Param{
			{
				Name: "short",
				// 10 iterations take 1 minute each
				Timeout:   20 * time.Minute,
				Val:       10,
				ExtraAttr: []string{"dsp_small"},
			},
			{
				Name: "medium",
				// 120 iterations take 1 minute each
				Timeout:   240 * time.Minute,
				Val:       120,
				ExtraAttr: []string{"dsp_small"},
			},
			{
				Name: "fw_qual",
				// 2500 iterations take 1 minute each
				Timeout:   5000 * time.Minute,
				Val:       2500,
				ExtraAttr: []string{"group:firmware", "firmware_stress", "dsp_large"},
			},
		},
	})
}

func RebootAndCheckIshmod(ctx context.Context, s *testing.State) {
	dut := s.DUT()

	// Compile the regular expressions
	requiredPatterns := []*regexp.Regexp{
		regexp.MustCompile(`cros_ec_ishtp`),
		regexp.MustCompile(`intel_ishtp_loader`),
		regexp.MustCompile(`intel_ish_ipc`),
		regexp.MustCompile(`intel_ishtp`),
	}

	for i := 0; i < s.Param().(int); i++ {
		s.Logf("Iteration %d: Rebooting DUT and checking the lsmod", i+1)

		// Perform a warm boot
		if err := dut.Conn().CommandContext(ctx, "sh", "-c", "{ sleep 2; sync; sync; reboot; }").Run(); err != nil {
			s.Fatal("Failed to perform warm boot: ", err)
		}

		if err := testing.Poll(ctx, func(ctx context.Context) error {
			return dut.WaitConnect(ctx)
		}, &testing.PollOptions{Timeout: 1 * time.Minute}); err != nil {
			s.Fatal("Failed to reconnect to DUT: ", err)
		}

		// Read lsmod logs
		out, err := dut.Conn().CommandContext(ctx, "lsmod").Output()
		if err != nil {
			s.Fatal("Failed to read lsmod: ", err)
		}
		lsmodOutput := string(out)

		// Check for the required patterns
		for _, pattern := range requiredPatterns {
			if !pattern.MatchString(lsmodOutput) {
				s.Fatalf("Iteration %d: Required pattern %q not found in lsmod logs", i+1, pattern.String())
			}
		}

		s.Logf("Iteration %d: lsmod logs checked successfully", i+1)
	}
}
