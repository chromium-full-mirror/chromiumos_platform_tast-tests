// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"strings"
	"time"

	golangSSH "golang.org/x/crypto/ssh"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

const (
	// A region of flash that wont be disturbed.
	region = "RW_SECTION_B"
)

// This is also tested as part of other tests: FAFT 'WriteProtect' and
// FlashromTester. None of those tests are suitable for CQ, this test is faster
// and simpler, and will be stabilised and promoted to CQ.

func init() {
	testing.AddTest(&testing.Test{
		Func:         WriteProtectProtectsFlash,
		Desc:         "Verify that enabled hardware and software write protect prevent flash being written to",
		Contacts:     []string{"cros-flashrom-team@google.com", "nartemiev@google.com"},
		Attr:         []string{}, // test disabled https://buganizer.corp.google.com/issues/255617349
		BugComponent: "b:750299",
		SoftwareDeps: []string{"crossystem", "flashrom"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		// 10 minutes for the test, 21 minutes for cleanup.
		Timeout:      31 * time.Minute,
		Vars:         []string{"servo"},
		LacrosStatus: testing.LacrosVariantUnneeded,
	})
}

func WriteProtectProtectsFlash(ctx context.Context, s *testing.State) {
	servoSpec, _ := s.Var("servo")
	h := firmware.NewHelper(s.DUT(), nil, "", servoSpec, s.DUT().HostName(), "", "", "")
	cleanupContext := ctx
	// 10 minute timeout taken from firmware helper fixture Teardown.
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()
	defer h.Close(cleanupContext)

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to require servo: ", err)
	}

	// Might potentially fix issues with servod stopping during execution.
	if err := h.Servo.RemoveCCDWatchdogs(ctx); err != nil {
		s.Fatal("Failed to remove main watchdog: ", err)
	}

	var flashromConfig flashrom.Config
	flash, ctx, shutdown, _, err := flashromConfig.
		FlashromInit(flashrom.VerbosityDebug).
		ProgrammerInit(flashrom.ProgrammerHost, "").
		SetDut(h.DUT).
		Probe(ctx)
	defer func() {
		if err := shutdown(); err != nil {
			s.Error("Failed to shutdown flashromInstance: ", err)
		}
	}()

	if err != nil {
		s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
	}

	ctx, restore, originalFirmware, err := utils.BackupAndRestoreAPFirmwareAndWriteProtect(ctx, h.DUT, h.Servo, flash)
	if err != nil {
		s.Fatal("Firmware backup failed: ", err)
	}
	defer restore(s)

	// Hardware write protect needs to be disabled so that software write protect range can be modified
	s.Log("Disabling hardware write protect")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		s.Fatal("Failed to enable hardware write protect: ", err)
	}

	s.Log("Enabling software and hardware write protect and rebooting")
	if out, err := flash.SoftwareWriteProtectEnable(ctx); err != nil {
		s.Log(out)
		s.Fatal("Failed to enable software write protect: ", err)
	}

	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
		s.Fatal("Failed to enable hardware write protect: ", err)
	}

	// One board (asurada) cannot enable hardware write protect without a reboot.
	if err = h.DUT.Reboot(ctx); err != nil {
		s.Fatal("Failed to reboot: ", err)
	}

	if err = utils.CheckCrossystemWPSW(ctx, h, 1); err != nil {
		s.Fatal("Hardware write protect is not enabled: ", err)
	}

	flashsize, out, err := flash.Size(ctx)
	if err != nil {
		s.Log(out)
		s.Fatal("Failed to read flash size: ", err)
	}

	randomDataFileStdout, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-t", "tast.firmware.APFW.random.XXXXXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to create a temp file: ", err)
	}
	randomDataFile := strings.TrimSpace(string(randomDataFileStdout))

	cleanupContext = ctx
	ctx, cancel = ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		h.DUT.Conn().CommandContext(ctx, "rm", randomDataFile).Output(ssh.DumpLogOnError)
	}(cleanupContext)

	_, err = h.DUT.Conn().CommandContext(ctx, "dd", "if=/dev/random", fmt.Sprintf("of=%s", randomDataFile), fmt.Sprintf("bs=%d", flashsize), "count=1").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to write random data to temp file: ", err)
	}

	// Only the RW_SECTION_B section is written to. This reduces the risk of a
	// repair being required if this test crashes. This also avoids any regions
	// that may be concurrently written to by co-processors. A failure to find
	// the region will sneak through as an error here, but will be caught by the
	// verify step at the end of this test.
	s.Log("Attempting to flash AP, this should fail")
	out, err = flash.Write(ctx, randomDataFile, true, true, "", []string{region})
	if err == nil {
		s.Log(string(out))
		s.Fatal("Failed: flash was not protected by write protect")
	}
	var flashromExitError *golangSSH.ExitError
	if !errors.As(err, &flashromExitError) {
		s.Log(string(out))
		s.Fatal("Failed: expected ExitError but got something else: ", err)
	}
	if flashromExitError.Waitmsg.ExitStatus() != 2 {
		s.Log(string(out))
		s.Fatal("Failed: expected flashrom to exit(2): ", err)
	}

	// Flashrom claimed to fail, but we check that it did not write anything at all.
	out, err = flash.Verify(ctx, *originalFirmware, region)
	if err != nil {
		s.Log(string(out))
		s.Fatal("Failed: firmware verify failed: ", err)
	}
}
