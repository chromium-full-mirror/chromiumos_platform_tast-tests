// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"regexp"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type corruptSingleSectionVals struct {
	sectionA bios.ImageSection
	sectionB bios.ImageSection
	bodyA bios.ImageSection
	bodyB bios.ImageSection
}

func init() {
	testing.AddTest(&testing.Test{
		Func: CorruptFW,
		Desc: "Corrupt a single section of RW A AP firmware, reboot, verify alternate firware booted, restore, repeat for B",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      25 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "body_normal",
				Fixture: fixture.NormalMode,
				Val: &corruptSingleSectionVals{
					bios.FWBodyAImageSection, bios.FWBodyBImageSection, bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
			{
				Name:    "body_dev",
				Fixture: fixture.DevModeGBB,
				Val: &corruptSingleSectionVals{
					bios.FWBodyAImageSection, bios.FWBodyBImageSection, bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
			{
				Name:    "sig_normal",
				Fixture: fixture.NormalMode,
				Val: &corruptSingleSectionVals{
					bios.FWSignAImageSection, bios.FWSignBImageSection, bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
			{
				Name:    "sig_dev",
				Fixture: fixture.DevModeGBB,
				Val: &corruptSingleSectionVals{
					bios.FWSignAImageSection, bios.FWSignBImageSection, bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
		},
	})
}

func CorruptFW(ctx context.Context, s *testing.State) {
	val := s.Param().(*corruptSingleSectionVals)
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	shouldRestoreFirmware := false
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()

	s.Log("Boot to firmware B")
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if err := fwUtils.ChangeFWVariant(ctx, h, ms, fwCommon.RWSectionB); err != nil {
		s.Fatal("Failed to change FW variant: ", err)
	}
	s.Log("Boot to firmware A")
	if err := fwUtils.ChangeFWVariant(ctx, h, ms, fwCommon.RWSectionA); err != nil {
		s.Fatal("Failed to change FW variant: ", err)
	}

	s.Log("Backup AP firmware")
	out, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed creating remote temp dir: ", err)
	}
	remoteTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		err := h.DUT.Conn().CommandContext(cleanupContext, "rm", "-rf", remoteTempDir).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed deleting remote temp dir: ", err)
		}
	}()
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "read", fmt.Sprintf("%s/bios_backup.bin", remoteTempDir)).Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed taking bios backup: ", err)
	}
	out, err = h.ServoProxy.OutputCommand(ctx, false, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwservoXXXXXX")
	if err != nil {
		s.Fatal("Failed to create servo temp dir")
	}
	servoTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		if err := h.ServoProxy.RunCommand(cleanupContext, false, "rm", "-rf", servoTempDir); err != nil {
			s.Fatal("Failed deleting servo temp dir: ", err)
		}
	}()
	localTempDir, err := os.MkdirTemp("", "fwlocal*")
	if err != nil {
		s.Fatal("Failed to create local temp dir")
	}

	s.Log("Downloading backup to ", localTempDir)
	if err := linuxssh.GetFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/bios_backup.bin", remoteTempDir), fmt.Sprintf("%s/bios_backup.bin", localTempDir), linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to download: ", err)
	}
	s.Log("Copying file to servohost ", servoTempDir)
	if err := h.ServoProxy.PutFiles(ctx, false, map[string]string{
		fmt.Sprintf("%s/bios_backup.bin", localTempDir): fmt.Sprintf("%s/bios_backup.bin", servoTempDir),
	}); err != nil {
		s.Fatal("Failed to copy files to servo host: ", err)
	}

	restoreFirmware := func(ctx context.Context) {
		s.Log("Restoring AP firmware via servo")

		if err := h.ServoProxy.RunCommand(ctx, true, "futility", "update", "--servo", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()),
			"--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/bios_backup.bin", servoTempDir)); err != nil {
			s.Fatal("Failed restoring firmware via servo: ", err)
		}
		if err := h.WaitConnect(ctx); err != nil {
			s.Error("Failed to connect to DUT: ", err)
		}
		shouldRestoreFirmware = false
	}
	defer func() {
		if shouldRestoreFirmware {
			restoreFirmware(cleanupContext)
		}
	}()

	// futility won't flash an image that has an invalid signature blocks, so to corrupt the data we need to:
	// - Get the body sizes
	// - Create corrupt bodies for A & B
	// - Generate a new image that contains those bodies
	// - Sign it.
	// - Extract the sections we want to test
	// - Create yet another image that contains those sections
	// - Flash it.

	s.Log("Corrupting FW bodies")
	// - Get the body sizes
	out, err = h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", "-p", fmt.Sprintf("%s/bios_backup.bin", remoteTempDir), string(val.bodyA), string(val.bodyB)).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed getting section sizes: ", err)
	}
	fmapRe := regexp.MustCompile(`(?m)^(\S+) \d+ (\d+)`)
	matches := fmapRe.FindAllSubmatch(out, -1)
	if matches == nil {
		s.Fatal("Output doesn't match regex: ", string(out))
	}
	// - Create corrupt bodies for A & B
	for _, m := range matches {
		out, err = h.DUT.Conn().CommandContext(ctx, "dd", fmt.Sprintf("of=%s/%s_corrupt.bin", remoteTempDir, string(m[1])), "if=/dev/random", fmt.Sprintf("bs=%s", string(m[2])), "count=1").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed creating corrupt file: ", err)
		}
	}
	// - Generate a new image that contains those bodies
	err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/corrupt_bodies.bin", remoteTempDir), fmt.Sprintf("%s/bios_backup.bin", remoteTempDir),
		fmt.Sprintf("%s:%s/%s_corrupt.bin", string(val.bodyA), remoteTempDir, string(val.bodyA)),
		fmt.Sprintf("%s:%s/%s_corrupt.bin", string(val.bodyB), remoteTempDir, string(val.bodyB)),
	).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed futility load_fmap: ", err)
	}

	s.Log("Signing corrupt image")
	// - Sign it.
	err = h.DUT.Conn().CommandContext(ctx, "futility", "sign", "--type", "bios", fmt.Sprintf("%s/corrupt_bodies.bin", remoteTempDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed futility sign: ", err)
	}

	// - Extract the sections we want to test
	err = h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", "-x", fmt.Sprintf("%s/corrupt_bodies.bin", remoteTempDir),
		fmt.Sprintf("%s:%s/%s.bin", val.sectionA, remoteTempDir, val.sectionA),
		fmt.Sprintf("%s:%s/%s.bin", val.sectionB, remoteTempDir, val.sectionB),
	).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed getting corrupt sections: ", err)
	}

	flashSectionAndReboot := func(ctx context.Context, oneSection bios.ImageSection, expectedBootCopy fwCommon.RWSection) {
		// - Create yet another image that contains the one section
		err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/corrupt_one.bin", remoteTempDir), fmt.Sprintf("%s/bios_backup.bin", remoteTempDir),
			fmt.Sprintf("%s:%s/%s.bin", oneSection, remoteTempDir, oneSection),
		).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed futility load_fmap: ", err)
		}

		s.Logf("Flashing corrupt section: %q", oneSection)
		// - Flash it.
		shouldRestoreFirmware = true
		err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/corrupt_one.bin", remoteTempDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed flashing corrupt fw: ", err)
		}

		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			s.Fatal("Failed to reboot after corrupting: ", err)
		}
		s.Log("Check the firmware version")
		curr, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
		if err != nil {
			s.Fatal("Failed to check firmware version: ", err)
		} else if curr != string(expectedBootCopy) {
			s.Errorf("Incorrect active firmware. got=%q, want=%q", curr, expectedBootCopy)
		}
	}

	flashSectionAndReboot(ctx, val.sectionA, fwCommon.RWSectionB)
	flashSectionAndReboot(ctx, val.sectionB, fwCommon.RWSectionA)

	s.Log("Restoring backup")
	err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/bios_backup.bin", remoteTempDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed restoring fw: ", err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot after restoring: ", err)
	}
	shouldRestoreFirmware = false
}
