// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package intel

import (
	"context"
	"fmt"
	"io/ioutil"
	"os"
	"strconv"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/powercontrol"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         SuspendStress,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Desc:         "Verifies suspend stress test",
		Contacts:     []string{"intel.chrome.automation.team@intel.com", "ambalavanan.m.m@intel.com"},
		BugComponent: "b:157291", // ChromeOS > External > Intel
		SoftwareDeps: []string{"chrome", "reboot"},
		VarDeps:      []string{"servo"},
		Params: []testing.Param{{
			Name:    "quick",
			Val:     2,
			Timeout: 5 * time.Minute,
		}, {
			Name:      "bronze",
			Val:       500,
			Timeout:   250 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-bronze"},
		}, {
			Name:      "silver",
			Val:       1000,
			Timeout:   500 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-silver"},
		}, {
			Name:      "gold",
			Val:       2500,
			Timeout:   1250 * time.Minute,
			ExtraAttr: []string{"group:intel-stability-gold"},
		}}})
}

func SuspendStress(ctx context.Context, s *testing.State) {
	ctxForCleanUp := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 2*time.Minute)
	defer cancel()

	dut := s.DUT()

	logPath, err := ioutil.TempDir("", "temp")
	if err != nil {
		s.Fatal("Failed to create temp directory: ", err)
	}
	defer os.RemoveAll(logPath)

	servoSpec := s.RequiredVar("servo")
	pxy, err := servo.NewProxy(ctx, servoSpec, dut.KeyFile(), dut.KeyDir())
	if err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	defer pxy.Close(ctxForCleanUp)

	defer func(ctx context.Context) {
		if !dut.Connected(ctx) {
			if err := powercontrol.PowerOntoDUT(ctx, pxy, dut); err != nil {
				s.Fatal("Failed to power-on DUT at cleanup: ", err)
			}
		}

	}(ctxForCleanUp)

	const (
		prematureWakePattern    = "Premature wakes: 0"
		suspendFailurePattern   = "Suspend failures: 0"
		firmwareLogErrorPattern = "Firmware log errors: 0"
		s0ixErrorPattern        = "s0ix errors: 0"
		s2idleErrorPattern      = "s2idle errors: 0"
	)

	suspendErrors := []string{prematureWakePattern, suspendFailurePattern, firmwareLogErrorPattern}
	checkSuspendStates := []string{s0ixErrorPattern, s2idleErrorPattern}

	// Checks poll until no premature wake and/or suspend failures occurs with given poll timeout.
	if testing.Poll(ctx, func(ctx context.Context) error {
		stressOut, err := dut.Conn().CommandContext(ctx, "suspend_stress_test", "-c", "1").Output(ssh.DumpLogOnError)
		if err != nil {
			return errors.Wrap(err, "failed to execute suspend_stress_test command")
		}
		for _, errMsg := range suspendErrors {
			if !strings.Contains(string(stressOut), errMsg) {
				return errors.Errorf("failed was expecting %q, but got failures %s", errMsg, string(stressOut))
			}
		}
		// depending upon the suspend state, will have different output, so we check for one
		for _, errMsg := range checkSuspendStates {
			s.Logf("Checking for %s", errMsg)
			if strings.Contains(string(stressOut), errMsg) {
				break
			}
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 15 * time.Second,
	}); err != nil {
		s.Fatal("Failed to perform suspend_stress_test: ", err)
	}

	counter := strconv.Itoa(s.Param().(int))
	s.Logf("Execute: suspend_stress_test for %d counters", s.Param().(int))
	stressOut, err := dut.Conn().CommandContext(ctx, "suspend_stress_test", "-c", counter,
		fmt.Sprintf("--record_dmesg_dir=%s", logPath), "--suspend_min=15", "--suspend_max=20").Output()
	if err != nil {
		s.Fatal("Failed to execute suspend_stress_test command: ", err, string(stressOut))
	}

	for _, want := range suspendErrors {
		if got := string(stressOut); !strings.Contains(got, want) {
			s.Fatalf("Failed: got %s failures; want %q match", got, want)
		}
	}

}
