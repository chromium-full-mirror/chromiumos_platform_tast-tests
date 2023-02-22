// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package power

import (
	"context"
	"regexp"
	"strconv"
	"time"

	"chromiumos/tast/common/testexec"
	"chromiumos/tast/local/chrome"
	"chromiumos/tast/testing"
)

var (
	powerItrVar = testing.RegisterVarString(
		"power.SuspendStressInGuestMode.itr",
		"2",
		"It takes number of iterations as input")
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendStressInGuestMode,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Suspend Stress test memory check in GuestMode",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		SoftwareDeps: []string{"chrome", "no_qemu"},
		Attr:         []string{"group:mainline", "informational"},
		Vars:         []string{"power.SuspendStressInGuestMode.itr"},
		Timeout:      5 * time.Hour,
	})
}

// SuspendStressInGuestMode Suspend Stress test memory check in GuestMode.
func SuspendStressInGuestMode(ctx context.Context, s *testing.State) {
	var (
		SuspendFailurePattern   = regexp.MustCompile("Suspend failures: 0")
		FirmwareLogErrorPattern = regexp.MustCompile("Firmware log errors: 0")
		S0ixErrorPattern        = regexp.MustCompile("s0ix errors: ([0-9]+)")
		S2IdleErrorPattern      = regexp.MustCompile("s2idle errors: ([0-9]+)")
	)
	s.Log("Iterations: ", powerItrVar.Value())
	opts := []chrome.Option{chrome.GuestLogin(), chrome.KeepState()}
	cr, err := chrome.New(ctx, opts...)
	if err != nil {
		s.Fatal("Failed to start Chrome: ", err)
	}
	defer cr.Close(ctx)
	out, err := testexec.CommandContext(ctx, "suspend_stress_test", "--memory_check", "--suspend_min=10", "--suspend_max=10", "-c", powerItrVar.Value()).Output()
	if err != nil {
		s.Fatal("Failed to execute suspend_stress_test command: ", err)
	}
	errorCodes := []*regexp.Regexp{SuspendFailurePattern, FirmwareLogErrorPattern}
	for _, errmsg := range errorCodes {
		if !errmsg.MatchString(string(out)) {
			s.Fatalf("Failed %q failures are non-zero", errmsg)
		}
	}

	// These optionally exist, so only check for non-zero if there's a subgroup match.
	errorCodes = []*regexp.Regexp{S0ixErrorPattern, S2IdleErrorPattern}
	for _, errmsg := range errorCodes {
		m := errmsg.FindStringSubmatch(string(out))
		if len(m) == 1 {
			if num, err := strconv.Atoi(m[0]); err != nil {
				s.Fatalf("Failed to parse number of errors in %q, %v", errmsg, err)
			} else if num != 0 {
				s.Fatalf("Failed %q failures are non-zero", errmsg)
			}
		}
	}
}
