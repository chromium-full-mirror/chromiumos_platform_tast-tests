// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast-tests/cros/local/chrome"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HdmiSuspendResume,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies HDMI native port functionality with suspend-resume cycles",
		BugComponent: "b:157291", // ChromeOS > External > Intel
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		Attr:         []string{"group:intel-hdmi"},
		SoftwareDeps: []string{"chrome"},
		Pre:          chrome.LoggedIn(),
		Timeout:      8 * time.Minute,
	})
}

func HdmiSuspendResume(ctx context.Context, s *testing.State) {
	var (
		C10PkgPattern         = regexp.MustCompile(`C10 : ([A-Za-z0-9]+)`)
		SuspndFailurePattern  = regexp.MustCompile("Suspend failures: 0")
		FrmwreLogErrorPattern = regexp.MustCompile("Firmware log errors: 0")
		S0ixErrorPattern      = regexp.MustCompile("s0ix errors: 0")
	)
	const (
		SlpS0Cmd         = "cat /sys/kernel/debug/pmc_core/slp_s0_residency_usec"
		PkgCstateCmd     = "cat /sys/kernel/debug/pmc_core/package_cstate_show"
		SuspendStressCmd = "suspend_stress_test -c 10"
	)
	numberOfDisplay := 1
	spec := usbutils.DisplaySpec{
		NumberOfDisplays: &numberOfDisplay,
		DisplayType:      usbutils.NativeHDMI,
	}
	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
		s.Fatal("Failed to check for connected external HDMI display: ", err)
	}

	cmdOutput := func(cmd string) string {
		s.Logf("Executing command: %s", cmd)
		out, err := testexec.CommandContext(ctx, "sh", "-c", cmd).Output()
		if err != nil {
			s.Fatalf("Failed to execute %s command: %v", cmd, err)
		}
		return string(out)
	}
	slpOpSetPre := cmdOutput(SlpS0Cmd)
	pkgOpSetOutput := cmdOutput(PkgCstateCmd)
	matchSetPre := (C10PkgPattern).FindStringSubmatch(pkgOpSetOutput)
	if matchSetPre == nil {
		s.Fatal("Failed to match pre PkgCstate value: ", pkgOpSetOutput)
	}
	pkgOpSetPre := matchSetPre[1]
	stressOut := cmdOutput(SuspendStressCmd)

	suspendErrors := []*regexp.Regexp{SuspndFailurePattern, FrmwreLogErrorPattern, S0ixErrorPattern}
	for _, errmsg := range suspendErrors {
		if !(errmsg).MatchString(string(stressOut)) {
			s.Fatalf("Failed expected%q failures are non-zero", errmsg)
		}
	}

	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
		s.Fatal("Failed to check for connected external HDMI display: ", err)
	}

	slpOpSetPost := cmdOutput(SlpS0Cmd)
	if slpOpSetPre == slpOpSetPost {
		s.Fatalf("Failed SLP counter value must be different than the value %q noted most recently %q", slpOpSetPre, slpOpSetPost)
	}
	if slpOpSetPost == "0" {
		s.Fatal("Failed SLP counter value must be non-zero, noted is: ", slpOpSetPost)
	}
	pkgOpSetPostOutput := cmdOutput(PkgCstateCmd)
	matchSetPost := (C10PkgPattern).FindStringSubmatch(pkgOpSetPostOutput)
	if matchSetPost == nil {
		s.Fatal("Failed to match post PkgCstate value: ", pkgOpSetPostOutput)
	}
	pkgOpSetPost := matchSetPost[1]
	if pkgOpSetPre == pkgOpSetPost {
		s.Fatalf("Failed Package C10 value %q must be different than value noted earlier %q", pkgOpSetPre, pkgOpSetPost)
	}
	if pkgOpSetPost == "0x0" || pkgOpSetPost == "0" {
		s.Fatal("Failed Package C10 should be non-zero")
	}
	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
		s.Fatal("Failed to check for connected external HDMI display: ", err)
	}
}
