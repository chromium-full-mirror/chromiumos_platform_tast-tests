// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"strings"
	"time"

	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type corruptSingleSectionVals struct {
	sectionA bios.ImageSection
	sectionB bios.ImageSection
	bodyA    bios.ImageSection
	bodyB    bios.ImageSection
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
		TestBedDeps:  tbdep.ServoPresentAndWorking,
		Attr:         []string{"group:firmware", "firmware_bios", "firmware_stressed", "firmware_bios_ro", "firmware_bios_rw"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC(), hwdep.NoVbootCbfsIntegration()),
		Timeout:      25 * time.Minute,
		Params: []testing.Param{
			{
				Name:      "body_normal",
				ExtraAttr: []string{"firmware_meets_kpi"},
				Fixture:   fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val: &corruptSingleSectionVals{
					bios.FWBodyAImageSection, bios.FWBodyBImageSection, bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
			{
				Name:    "body_dev",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevModeGBB),
				Val: &corruptSingleSectionVals{
					bios.FWBodyAImageSection, bios.FWBodyBImageSection, bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
			{
				Name:      "sig_normal",
				ExtraAttr: []string{"firmware_meets_kpi"},
				Fixture:   fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
				Val: &corruptSingleSectionVals{
					bios.FWSignAImageSection, bios.FWSignBImageSection, bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
			{
				Name:    "sig_dev",
				Fixture: fixture.BootModeFixtureWithAPBackup(fixture.DevModeGBB),
				Val: &corruptSingleSectionVals{
					bios.FWSignAImageSection, bios.FWSignBImageSection, bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
		},
	})
}

func CorruptFW(ctx context.Context, s *testing.State) {
	val := s.Param().(*corruptSingleSectionVals)
	backupManager := s.FixtValue().(*fixture.Value).BackupManager
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

	restoreFirmware := func(ctx context.Context) {
		s.Log("Restoring AP firmware via servo")

		out, err := h.ServoProxy.OutputCommand(ctx, false, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwservoXXXXXX")
		if err != nil {
			s.Fatal("Failed to create servo temp dir")
		}
		servoTempDir := strings.TrimSuffix(string(out), "\n")
		defer func() {
			if err := h.ServoProxy.RunCommand(cleanupContext, false, "rm", "-rf", servoTempDir); err != nil {
				s.Fatal("Failed deleting servo temp dir: ", err)
			}
		}()

		backupOnServoProxy := fmt.Sprintf("%s/bios_backup.bin", servoTempDir)
		if err := backupManager.CopyBackupToServoProxy(ctx, h.ServoProxy, fixture.FirmwareAP, backupOnServoProxy); err != nil {
			s.Fatal("Failed to copy AP firmware backup to ServoProxy: ", err)
		}

		if err := h.ServoProxy.RunCommand(ctx, true, "futility", "update", "--servo", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()),
			"--mode=recovery", "--wp=1", "--host_only", "-i", backupOnServoProxy); err != nil {
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

	out, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwimgXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed creating remote temp dir: ", err)
	}
	dutTempDir := strings.TrimSuffix(string(out), "\n")
	defer func() {
		if err := h.DUT.Conn().CommandContext(cleanupContext, "rm", "-rf", dutTempDir).Run(ssh.DumpLogOnError); err != nil {
			s.Log("Failed to delete temp dir on DUT: ", err)
		}
	}()

	backupOnDut := fmt.Sprintf("%s/bios_backup.bin", dutTempDir)
	if err := backupManager.CopyBackupToDut(ctx, h.DUT, fixture.FirmwareAP, backupOnDut); err != nil {
		s.Fatal("Failed to copy backup formware image to DUT: ", err)
	}

	s.Log("Corrupting FW bodies")
	// - Get the body sizes
	futilityInstance, err := futility.NewLocalBuilder(h.DUT).Debug(true).Build()
	if err != nil {
		s.Fatal("Failed to setup futility instance: ", err)
	}
	sections, _, err := futilityInstance.DumpFmap(ctx, backupOnDut, []string{string(val.bodyA), string(val.bodyB)})
	if err != nil {
		s.Fatal("Failed to get firmware sections: ", err)
	}

	// - Create corrupt bodies for A & B
	for _, section := range sections {
		_, err = h.DUT.Conn().CommandContext(ctx, "dd", fmt.Sprintf("of=%s/%s_corrupt.bin", dutTempDir, section.Name), "if=/dev/random", fmt.Sprintf("bs=%d", section.Size), "count=1").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed creating corrupt file: ", err)
		}
	}
	// - Generate a new image that contains those bodies
	corruptBodiesImage := fmt.Sprintf("%s/corrupt_bodies.bin", dutTempDir)
	out, err = futilityInstance.LoadFmap(ctx, backupOnDut, corruptBodiesImage, map[string]string{
		string(val.bodyA): fmt.Sprintf("%s/%s_corrupt.bin", dutTempDir, string(val.bodyA)),
		string(val.bodyB): fmt.Sprintf("%s/%s_corrupt.bin", dutTempDir, string(val.bodyB)),
	})
	if err != nil {
		s.Fatal("Failed to load corrupt sections into image: ", err, "\nOutput:\n", out)
	}

	s.Log("Signing corrupt image")
	// - Sign it.
	if out, err = futilityInstance.SignBIOS(ctx, futility.NewSignBIOSOptions(corruptBodiesImage)); err != nil {
		s.Fatal("Failed to sign firmware with corrupt bodies: ", err, "\nOutput:\n", out, "\n\n")
	}

	// - Extract the sections we want to test
	if _, err = futilityInstance.DumpFmapExtract(ctx, corruptBodiesImage, map[string]string{
		string(val.sectionA): fmt.Sprintf("%s/%s.bin", dutTempDir, string(val.sectionA)),
		string(val.sectionB): fmt.Sprintf("%s/%s.bin", dutTempDir, string(val.sectionB)),
	}); err != nil {
		s.Fatal("Failed to extract corrupt sections: ", err)
	}

	flashSectionAndReboot := func(ctx context.Context, oneSection bios.ImageSection, expectedBootCopy fwCommon.RWSection) {
		// - Create yet another image that contains the one section
		corruptOne := fmt.Sprintf("%s/corrupt_one.bin", dutTempDir)
		if _, err := futilityInstance.LoadFmap(ctx, backupOnDut, corruptOne, map[string]string{
			string(oneSection): fmt.Sprintf("%s/%s.bin", dutTempDir, string(oneSection)),
		}); err != nil {
			s.Fatal("Failed to load section into image: ", err)
		}

		s.Logf("Flashing corrupt section: %q", oneSection)
		// - Flash it.
		shouldRestoreFirmware = true
		err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/corrupt_one.bin", dutTempDir)).Run(ssh.DumpLogOnError)
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
	err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/bios_backup.bin", dutTempDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed restoring fw: ", err)
	}

	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
		s.Fatal("Failed to reboot after restoring: ", err)
	}
	shouldRestoreFirmware = false
}
