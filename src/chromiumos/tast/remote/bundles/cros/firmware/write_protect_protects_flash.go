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

	"chromiumos/tast/common/servo"
	"chromiumos/tast/ctxutil"
	"chromiumos/tast/remote/bundles/cros/firmware/utils"
	"chromiumos/tast/remote/firmware"
	"chromiumos/tast/ssh"
	"chromiumos/tast/testing"
	"chromiumos/tast/testing/hwdep"
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
		Timeout: 31 * time.Minute,
		Vars:    []string{"servo"},
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
	if err := h.Servo.WatchdogRemove(ctx, servo.WatchdogMain); err != nil {
		s.Fatal("Failed to remove main watchdog: ", err)
	}

	ctx, restore, originalFirmware, err := utils.BackupAndRestoreAPFirmwareAndWriteProtect(ctx, h.DUT, h.Servo)
	if err != nil {
		s.Fatal("Firmware backup failed: ", err)
	}
	defer restore(s)

	// Hardware WP needs to be disabled so that APSoftwareWriteProtectEnable can
	// control the write protect range.
	s.Log("Disabling hardware write protect")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
		s.Fatal("Failed to enable hardware write protect: ", err)
	}

	s.Log("Enabling software and hardware write protect and rebooting")
	if err := utils.APSoftwareWriteProtectEnable(ctx, h.DUT.Conn()); err != nil {
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

	flashsize, err := utils.APFirmwareSize(ctx, h.DUT.Conn())
	if err != nil {
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
	cmd := h.DUT.Conn().CommandContext(ctx, "flashrom", "-p", "host", "--include", region, "-w", randomDataFile)
	err = cmd.Run()
	if err == nil {
		cmd.DumpLog(ctx)
		s.Fatal("Failed: flash was not protected by write protect")
	}
	flashromExitError, ok := err.(*golangSSH.ExitError)
	if !ok {
		cmd.DumpLog(ctx)
		s.Fatal("Failed: expected ExitError but got something else: ", err)
	}
	if flashromExitError.Waitmsg.ExitStatus() != 2 {
		s.Fatal("Failed: expected flashrom to exit(2): ", err)
	}

	// Flashrom claimed to fail, but we check that it did not write anything at all.
	err = utils.APFirmwareVerify(ctx, h.DUT.Conn(), *originalFirmware, region)
	if err != nil {
		s.Fatal("Failed: firmware verify failed: ", err)
	}
}
