// Copyright 2023 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

type corruptTestVal struct {
	sectionA bios.ImageSection
	sectionB bios.ImageSection
}

func init() {
	testing.AddTest(&testing.Test{
		Func: CorruptFWBothAB,
		Desc: "Corrupt both copies of AP firmware, verify broken screen, restore backup via servo",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_level3"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      25 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:              "body_normal",
				Fixture:           fixture.NormalMode,
				ExtraAttr:         []string{"firmware_bios"},
				ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
				Val: &corruptTestVal{
					bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
			{
				Name:              "body_dev",
				Fixture:           fixture.DevModeGBB,
				ExtraAttr:         []string{"firmware_bios"},
				ExtraRequirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
				Val: &corruptTestVal{
					bios.FWBodyAImageSection, bios.FWBodyBImageSection,
				},
			},
			{
				Name:      "sig_normal",
				Fixture:   fixture.NormalMode,
				ExtraAttr: []string{"firmware_unstable"},
				Val: &corruptTestVal{
					bios.FWSignAImageSection, bios.FWSignBImageSection,
				},
			},
			{
				Name:      "sig_dev",
				Fixture:   fixture.DevModeGBB,
				ExtraAttr: []string{"firmware_unstable"},
				Val: &corruptTestVal{
					bios.FWSignAImageSection, bios.FWSignBImageSection,
				},
			},
		},
	})
}

func CorruptFWBodySection(ctx context.Context, s *testing.State, h *firmware.Helper, corruptBiosRemoteImage, backupBiosRemoteImage, remoteTempDir string) error {
	s.Log("Corrupting FW bodies")
	// Copy BIOS image to corrupt it
	out, err := h.DUT.Conn().CommandContext(ctx, "cp", backupBiosRemoteImage, corruptBiosRemoteImage).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Error("Failed to copy image for corruption: ", out)
		return err
	}

	for _, section := range []string{string(bios.FWBodyAImageSection), string(bios.FWBodyBImageSection)} {
		_, err := h.DUT.Conn().CommandContext(ctx, "cbfstool", corruptBiosRemoteImage, "remove", "-r", section, "-n", "fallback/payload").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Error("Failed to corrupt ", section, " section: ", err)
			return err
		}
	}
	return nil
}

func CorruptFWBothAB(ctx context.Context, s *testing.State) {
	corruptFWSectionTest(ctx, s, CorruptFWBodySection, "RW firmware unable to verify firmware body")
}

func corruptFWSectionTest(ctx context.Context, s *testing.State, corruptFMAPSection func(context.Context, *testing.State, *firmware.Helper, string, string, string) error, failureReason string) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	sectionA := string(s.Param().(*corruptTestVal).sectionA)
	sectionB := string(s.Param().(*corruptTestVal).sectionB)
	shouldRestoreFirmware := false
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()

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

	backupBiosRemoteImage := fmt.Sprintf("%s/bios_backup.bin", remoteTempDir)
	if err := h.DUT.Conn().CommandContext(ctx, "futility", "read", backupBiosRemoteImage).Run(ssh.DumpLogOnError); err != nil {
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
	if err := linuxssh.GetFile(ctx, h.DUT.Conn(), backupBiosRemoteImage, fmt.Sprintf("%s/bios_backup.bin", localTempDir), linuxssh.DereferenceSymlinks); err != nil {
		s.Fatal("Failed to download: ", err)
	}
	s.Log("Copying file to servohost ", servoTempDir)
	if err := h.ServoProxy.PutFiles(ctx, false, map[string]string{
		fmt.Sprintf("%s/bios_backup.bin", localTempDir): fmt.Sprintf("%s/bios_backup.bin", servoTempDir),
	}); err != nil {
		s.Fatal("Failed to copy files to servo host: ", err)
	}

	var cutoffEvent reporters.Event
	oldEvents, err := h.Reporter.EventlogList(ctx)
	if err != nil {
		s.Fatal("Finding last event: ", err)
	}
	if len(oldEvents) > 0 {
		cutoffEvent = oldEvents[len(oldEvents)-1]
	}

	restoreFirmware := func(ctx context.Context) {
		s.Log("Restoring AP firmware via servo")

		if err := h.ServoProxy.RunCommand(ctx, true, "futility", "update", "--servo", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()),
			"--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/bios_backup.bin", servoTempDir)); err != nil {
			s.Error("Failed restoring firmware via servo: ", err)
		}
		// In b/314059450 it was discovered that some devices don't come back on after futility update. Explicitly reset to prevent this problem.
		if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
			s.Error("Failed to reset after restoring firmware: ", err)
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
	// - Create copy of image and modify body by removing (or modifying) CBFS file "fallback/payload" to make firmware fail verification.
	// - Sign resulting image
	// - Extract the sections we want to test
	// - Create yet another image that contains those sections
	// - Flash it.

	corruptBiosRemoteImage := fmt.Sprintf("%s/corrupt_bodies.bin", remoteTempDir)
	if err := corruptFMAPSection(ctx, s, h, corruptBiosRemoteImage, backupBiosRemoteImage, remoteTempDir); err != nil {
		s.Fatal("Failed to corrupt FMAP sections: ", err)
	}

	s.Log("Signing corrupt image")
	// - Sign it.
	err = h.DUT.Conn().CommandContext(ctx, "futility", "sign", "--type", "bios", corruptBiosRemoteImage).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed futility sign: ", err)
	}

	// - Extract the sections we want to test
	err = h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", "-x", corruptBiosRemoteImage,
		fmt.Sprintf("%s:%s/%s.bin", sectionA, remoteTempDir, sectionA),
		fmt.Sprintf("%s:%s/%s.bin", sectionB, remoteTempDir, sectionB),
	).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed getting corrupt sections: ", err)
	}

	// - Create yet another image that contains those sections
	err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/corrupt.bin", remoteTempDir), backupBiosRemoteImage,
		fmt.Sprintf("%s:%s/%s.bin", sectionA, remoteTempDir, sectionA),
		fmt.Sprintf("%s:%s/%s.bin", sectionB, remoteTempDir, sectionB),
	).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed futility load_fmap: ", err)
	}

	s.Logf("Flashing corrupt sections: %q %q", sectionA, sectionB)
	// - Flash it.
	shouldRestoreFirmware = true
	err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/corrupt.bin", remoteTempDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed flashing corrupt fw: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.SkipWaitConnect); err != nil {
		s.Fatal("Failed to reboot after corrupting: ", err)
	}
	waitContext, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
	defer cancel()
	s.Logf("Waiting %s(DelayRebootToPing) for DUT not to boot", h.Config.DelayRebootToPing)
	if err := h.WaitConnect(waitContext); err == nil {
		s.Error("DUT is unexpectedly up, corruption failed")
	}
	restoreFirmware(ctx)

	s.Log("Checking eventlog for evidence of broken screen")

	// Sometimes events are missing if you check too quickly after boot.
	var events []reporters.Event
	if err := testing.Poll(ctx, func(context.Context) error {
		var err error
		events, err = h.Reporter.EventlogListAfter(ctx, cutoffEvent)
		if err != nil {
			return testing.PollBreak(err)
		}
		if len(events) == 0 {
			return errors.New("no new events found")
		}
		return nil
	}, &testing.PollOptions{
		Timeout: 1 * time.Minute, Interval: 5 * time.Second,
	}); err != nil {
		s.Fatal("Gathering events: ", err)
	}
	found := false
	for _, event := range events {
		if strings.Contains(event.Message, failureReason) {
			found = true
			break
		}
	}
	if !found {
		s.Error("Did not find expected recovery reason in event log: ", events)
	}
}
