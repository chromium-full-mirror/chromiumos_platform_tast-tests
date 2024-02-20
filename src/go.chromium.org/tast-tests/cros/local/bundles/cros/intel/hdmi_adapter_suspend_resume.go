// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"io/ioutil"
	"regexp"
	"time"

	"go.chromium.org/tast-tests/cros/common/cswitch"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/common/usbutils"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/testing"
)

var (
	c10PackagesRe       = regexp.MustCompile(`C10 : ([A-Za-z0-9]+)`)
	suspendFailuresRe   = regexp.MustCompile("Suspend failures: 0")
	firmwareLogErrorsRe = regexp.MustCompile("Firmware log errors: 0")
	s0ixErrorsRe        = regexp.MustCompile("s0ix errors: 0")
)

const (
	slpS0Files         = "/sys/kernel/debug/pmc_core/slp_s0_residency_usec"
	packageCstateFiles = "/sys/kernel/debug/pmc_core/package_cstate_show"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         HdmiAdapterSuspendResume,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies USB type-C single port adapter functionality with suspend-resume cycles",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "pathan.jilani@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome"},
		Attr:         []string{"group:intel-cswitch-set1"},
		Vars: []string{
			"intel.cSwitchPort",
			"intel.domainIP",
		},
		Fixture: "chromeLoggedIn",
		Timeout: 8 * time.Minute,
	})
}

func HdmiAdapterSuspendResume(ctx context.Context, s *testing.State) {
	cleanupCtx := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 3*time.Second)
	defer cancel()

	const cSwitchOFF = "0"
	// The Type-C HDMI is connected to C-Switch in P1 as per the intel_cswitch_set1 suite setup.
	cswitchVar := "1"
	if cswitchON, ok := s.Var("intel.cSwitchPort"); ok {
		cswitchVar = cswitchON
	}
	// IP address of Tqc server hosting device.
	domainIP := s.RequiredVar("intel.domainIP")

	// Create C-Switch session that performs hot plug-unplug on USB4 device.
	sessionID, err := cswitch.CreateSession(ctx, domainIP)
	if err != nil {
		s.Fatal("Failed to create session: ", err)
	}

	if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cswitchVar, domainIP); err != nil {
		s.Fatal("Failed to enable c-switch port: ", err)
	}

	defer func(ctx context.Context) {
		if err := closeCswitchConnection(ctx, cSwitchOFF, sessionID, domainIP); err != nil {
			s.Fatal("Failed to close cswitch: ", err)
		}
	}(cleanupCtx)
	numberOfDisplay := 1
	spec := usbutils.DisplaySpec{
		NumberOfDisplays: &numberOfDisplay,
		DisplayType:      usbutils.TypeCHDMI,
	}
	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
		s.Fatal("Failed to check for connected external HDMI display: ", err)
	}

	cmdOutput := func(ctx context.Context, file string) string {
		out, err := ioutil.ReadFile(file)
		if err != nil {
			s.Fatalf("Failed to read %q file: %v", file, err)
		}
		return string(out)
	}

	slpOpSetPre := cmdOutput(ctx, slpS0Files)
	pkgOpSetOutput := cmdOutput(ctx, packageCstateFiles)
	matchSetPre := c10PackagesRe.FindStringSubmatch(pkgOpSetOutput)
	if matchSetPre == nil {
		s.Fatal("Failed to match pre PkgCstate value: ", pkgOpSetOutput)
	}
	pkgOpSetPre := matchSetPre[1]

	// GoBigSleepLint:Screen visible in extenal is taking time due to which Sleep is required before suspend stress.
	if err := testing.Sleep(ctx, 20*time.Second); err != nil {
		s.Fatal("Failed to sleep: ", err)
	}

	testing.ContextLog(ctx, "Executing suspend_stress_test for 10 cycles")
	stressOut, err := testexec.CommandContext(ctx, "suspend_stress_test", "-c", "10").Output()
	if err != nil {
		s.Fatal("Failed to execute suspend_stress_test command: ", err)
	}

	suspendErrors := []*regexp.Regexp{suspendFailuresRe, firmwareLogErrorsRe, s0ixErrorsRe}
	for _, errmsg := range suspendErrors {
		if !(errmsg.MatchString(string(stressOut))) {
			s.Fatalf("Failed expected %q, but failures are non-zero", errmsg)
		}
	}

	if err := assertSLPCounter(ctx, slpOpSetPre); err != nil {
		s.Fatal("Failed asserting SLP Counter: ", err)
	}

	if err := assertPackageCStates(ctx, pkgOpSetPre); err != nil {
		s.Fatal("Asserting Package C-State failed: ", err)
	}

	if err := usbutils.ExternalDisplayDetectionForLocal(ctx, spec); err != nil {
		s.Fatal("Failed to check for connected external HDMI display: ", err)
	}
}

func assertSLPCounter(ctx context.Context, slpOpSetPre string) error {
	slpOpSetPost, err := ioutil.ReadFile(slpS0Files)
	if err != nil {
		return errors.Wrapf(err, "failed to read %q file", slpS0Files)
	}
	if slpOpSetPre == string(slpOpSetPost) {
		return errors.Errorf("failed SLP counter value must be different than the value %q noted most recently %q", slpOpSetPre, slpOpSetPost)
	}
	if string(slpOpSetPost) == "0" {
		return errors.Errorf("failed SLP counter value must be non-zero, noted is: %q", slpOpSetPost)
	}
	return nil
}

func assertPackageCStates(ctx context.Context, pkgOpSetPre string) error {
	pkgOpSetPostOutput, err := ioutil.ReadFile(packageCstateFiles)
	if err != nil {
		return errors.Wrapf(err, "failed to read %q file", packageCstateFiles)
	}
	matchSetPost := c10PackagesRe.FindStringSubmatch(string(pkgOpSetPostOutput))
	if matchSetPost == nil {
		return errors.Errorf("failed to match post PkgCstate value: %q", pkgOpSetPostOutput)
	}
	pkgOpSetPost := matchSetPost[1]
	if pkgOpSetPre == pkgOpSetPost {
		return errors.Errorf("failed Package C10 value %q must be different than value %q noted most recently", pkgOpSetPre, pkgOpSetPost)
	}
	if pkgOpSetPost == "0x0" || pkgOpSetPost == "0" {
		return errors.Errorf("failed Package C10 = want non-zero, got %s", pkgOpSetPost)
	}
	return nil
}

func closeCswitchConnection(ctx context.Context, cSwitchOFF, sessionID, domainIP string) error {
	if err := testing.Poll(ctx, func(ctx context.Context) error {
		if err := cswitch.ToggleCSwitchPort(ctx, sessionID, cSwitchOFF, domainIP); err != nil {
			return errors.Wrap(err, "failed to disable c-switch port")
		}

		if err := cswitch.CloseSession(ctx, sessionID, domainIP); err != nil {
			return errors.Wrap(err, "failed to close session")
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 10 * time.Second,
	}); err != nil {
		return errors.Wrap(err, "failed to close cswitch connection")
	}
	return nil
}
