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
	"go.chromium.org/tast-tests/cros/common/servo"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"
	"go.chromium.org/tast-tests/cros/common/firmware/futility"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: RollbackFirmware,
		Desc: "Verifies that a firmware which has been rolled back to an earlier security version will not boot",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@chromium.org",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"flashrom"},
		LacrosStatus: testing.LacrosVariantUnneeded,
		Timeout:      25 * time.Minute,
		Params: []testing.Param{{
			Name:    "normal",
			Fixture: fixture.NormalMode,
		}, {
			Name:    "dev",
			Fixture: fixture.DevModeGBB,
			// On elm, the event log doesn't contain the correct message.
			ExtraHardwareDeps: hwdep.D(hwdep.SkipOnModel("elm")),
		}},
	})
}

func RollbackFirmware(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	var cutoffEvent reporters.Event

	func() {
		shouldRestoreFirmware := false

		localTempDir, err := os.MkdirTemp("", "fwlocal*")
		if err != nil {
			s.Fatal("Failed to create local temp dir")
		}
		remoteTmpFile, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwdutXXXXXX").Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed to create remote temp dir")
		}
		remoteTempDir := strings.TrimSuffix(string(remoteTmpFile), "\n")

		servoTmpFile, err := h.ServoProxy.OutputCommand(ctx, false, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwservoXXXXXX")
		if err != nil {
			s.Fatal("Failed to create remote temp dir")
		}
		servoTempDir := strings.TrimSuffix(string(servoTmpFile), "\n")

		cleanupContext := ctx
		ctx, closeFunc := ctxutil.Shorten(ctx, 10*time.Minute)
		defer closeFunc()
		defer func(ctx context.Context) {
			if err := h.RequireServo(ctx); err != nil {
				s.Fatal("Failed to require servo: ", err)
			}
			if shouldRestoreFirmware {
				s.Log("Restoring AP firmware via servo")
				if err := h.ServoProxy.RunCommand(ctx, true, "futility", "update", "--servo", fmt.Sprintf("--servo_port=%d", h.ServoProxy.GetPort()),
					"--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/backup.bin", servoTempDir)); err != nil {
					s.Error("Failed restoring firmware via servo: ", err)
				}
				if err := h.Servo.SetPowerState(ctx, servo.PowerStateReset); err != nil {
					s.Error("Hard reset failed: ", err)
				}
				if err := h.WaitConnect(ctx); err != nil {
					s.Error("Failed to connect to DUT: ", err)
				}
			}
			s.Logf("Deleting backups on servohost at %q", servoTempDir)
			if err := h.ServoProxy.RunCommand(ctx, false, "rm", "-rf", servoTempDir); err != nil {
				s.Error("Failed to delete firmware backups: ", err)
			}

			if err := os.RemoveAll(localTempDir); err != nil {
				s.Error("Failed to delete local firmware backups: ", err)
			}
			if err := h.EnsureDUTBooted(ctx); err != nil {
				s.Fatal("DUT is down during cleanup: ", err)
			}
			s.Logf("Deleting backups on DUT at %q", remoteTempDir)
			if err := h.DUT.Conn().CommandContext(ctx, "rm", "-rf", remoteTempDir).Run(ssh.DumpLogOnError); err != nil {
				s.Error("Failed to delete firmware backups: ", err)
			}
		}(cleanupContext)

		activeFW, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
		if err != nil {
			s.Fatal("Failed to get active FW: ", err)
		}
		s.Log("Current FW is ", activeFW)

		if activeFW == string(fwCommon.RWSectionA) {
			if err := firmware.SetFWTries(ctx, h.DUT, fwCommon.RWSectionB, 3); err != nil {
				s.Fatal("Failed to set FW tries to B: ", err)
			}
		} else {
			if err := firmware.SetFWTries(ctx, h.DUT, fwCommon.RWSectionA, 3); err != nil {
				s.Fatal("Failed to set FW tries to A: ", err)
			}
		}
		oldEvents, err := h.Reporter.EventlogList(ctx)
		if err != nil {
			s.Fatal("Finding last event: ", err)
		}
		if len(oldEvents) > 0 {
			cutoffEvent = oldEvents[len(oldEvents)-1]
		}

		ms, err := firmware.NewModeSwitcher(ctx, h)
		if err != nil {
			s.Fatal("Creating mode switcher: ", err)
		}

		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AllowGBBForce); err != nil {
			s.Fatal("Failed to reboot on alternate firmware: ", err)
		}
		newFW, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
		if err != nil {
			s.Error("Failed to get active FW: ", err)
		}
		s.Log("Current FW is ", newFW)
		if activeFW == newFW {
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
				s.Error("Gathering events: ", err)
			}
			s.Fatalf("Failed to boot to alternate firmware. Only %s is working. Run chromeos-firmwareupdate --mode=recovery --force: %+v", activeFW, events)
		}
		// Since we switched, the active is now different.
		activeFW = newFW

		s.Log("Backing up AP firmware")
		remoteBackupFile := fmt.Sprintf("%s/bios_backup.bin", remoteTempDir)
		if err := h.DUT.Conn().CommandContext(ctx, "futility", "read", remoteBackupFile).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed taking bios backup: ", err)
		}
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Failed to require BiosServiceClient: ", err)
		}

		s.Log("Backup on DUT written to ", remoteBackupFile)

		// Extract the signature blocks (FWSign?ImageSection).
		activeSignSection := bios.FWSignAImageSection
		inactiveSignSection := bios.FWSignBImageSection
		if activeFW == string(fwCommon.RWSectionB) {
			activeSignSection = bios.FWSignBImageSection
			inactiveSignSection = bios.FWSignAImageSection
		}
		activeSignFile := fmt.Sprintf("%s/%s.bin", remoteTempDir, activeSignSection)
		inactiveSignFile := fmt.Sprintf("%s/%s.bin", remoteTempDir, inactiveSignSection)
		// TODO(b/276861597): Use futility library.
		if err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", remoteBackupFile, "-x",
			fmt.Sprintf("%s:%s", activeSignSection, activeSignFile),
			fmt.Sprintf("%s:%s", inactiveSignSection, inactiveSignFile),
		).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to extract sections: ", err)
		}

		s.Log("Downloading files to ", localTempDir)
		localBackupFile := fmt.Sprintf("%s/backup.bin", localTempDir)
		if err := linuxssh.GetFile(ctx, h.DUT.Conn(), remoteBackupFile, localBackupFile, linuxssh.DereferenceSymlinks); err != nil {
			s.Fatal("Failed to download backup: ", err)
		}
		s.Log("Copying files to ", servoTempDir)
		if err := h.ServoProxy.PutFiles(ctx, false, map[string]string{
			localBackupFile: fmt.Sprintf("%s/backup.bin", servoTempDir),
		}); err != nil {
			s.Fatal("Failed to copy files to servo host: ", err)
		}

		futilityInstance, err := futility.NewLocalBuilder(h.DUT).Build()
		if err != nil {
			s.Fatal("Failed to get futility instance: ", err)
		}

		// Change the firmware version to 0 and resign. The normal firmware version is 1 or more, so 0 will be a rollback.
		rollbackOutputFile := remoteBackupFile + ".ver0.bin"
		signOptions := futility.NewSignBIOSOptions(remoteBackupFile).WithVersion(0).WithOutputFile(rollbackOutputFile)
		if _, err = futilityInstance.SignBIOS(ctx, signOptions); err != nil {
			s.Fatal("Failed to re-sign firmware: ", err)
		}

		activeRollbackSignFile := fmt.Sprintf("%s/%s.ver0.bin", remoteTempDir, activeSignSection)
		inactiveRollbackSignFile := fmt.Sprintf("%s/%s.ver0.bin", remoteTempDir, inactiveSignSection)
		// TODO(b/276861597): Use futility library.
		if err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", rollbackOutputFile, "-x",
			fmt.Sprintf("%s:%s", activeSignSection, activeRollbackSignFile),
			fmt.Sprintf("%s:%s", inactiveSignSection, inactiveRollbackSignFile),
		).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to extract sections: ", err)
		}

		shouldRestoreFirmware = true
		s.Log("Rolling back ", activeSignSection)
		err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/corrupt.bin", remoteTempDir), remoteBackupFile,
			fmt.Sprintf("%s:%s", activeSignSection, activeRollbackSignFile),
		).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed futility load_fmap: ", err)
		}
		err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--force", "--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/corrupt.bin", remoteTempDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed flashing corrupt fw: ", err)
		}
		ms, err = firmware.NewModeSwitcher(ctx, h)
		if err != nil {
			s.Fatal("Creating mode switcher: ", err)
		}
		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AllowGBBForce); err != nil {
			s.Fatal("Failed to reboot after rolling back 1 firmware: ", err)
		}
		newFW, err = h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
		if err != nil {
			s.Error("Failed to get active FW: ", err)
		}
		if activeFW == newFW {
			s.Errorf("Booted to wrong FW, got %q, want !%q", newFW, activeFW)
		}

		oldEvents, err = h.Reporter.EventlogList(ctx)
		if err != nil {
			s.Fatal("Finding last event: ", err)
		}
		if len(oldEvents) > 0 {
			cutoffEvent = oldEvents[len(oldEvents)-1]
		}

		s.Log("Rolling back ", inactiveSignSection)
		err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", "-o", fmt.Sprintf("%s/corrupt.bin", remoteTempDir), remoteBackupFile,
			fmt.Sprintf("%s:%s", activeSignSection, activeRollbackSignFile),
			fmt.Sprintf("%s:%s", inactiveSignSection, inactiveRollbackSignFile),
		).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed futility load_fmap: ", err)
		}
		err = h.DUT.Conn().CommandContext(ctx, "futility", "update", "--force", "--mode=recovery", "--wp=1", "--host_only", "-i", fmt.Sprintf("%s/corrupt.bin", remoteTempDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatal("Failed flashing corrupt fw: ", err)
		}

		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.SkipWaitConnect, firmware.AllowGBBForce); err != nil {
			s.Fatal("Failed to reboot after rolling back both firmwares: ", err)
		}
		waitContext, cancel := context.WithTimeout(ctx, h.Config.DelayRebootToPing)
		defer cancel()
		s.Logf("Waiting %s(DelayRebootToPing) for DUT not to boot", h.Config.DelayRebootToPing)
		if err := h.WaitConnect(waitContext); err == nil {
			s.Error("DUT is unexpectedly up, rollback prevention failed")
		}
	}()
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
	// I don't know which boards return "RW firmware failed signature check", but it was in the autotest also.
	re := regexp.MustCompile(`(RW firmware version rollback detected|RW firmware failed signature check)`)
	for _, event := range events {
		if re.FindString(event.Message) != "" {
			found = true
			break
		}
	}
	if !found {
		s.Error("Did not find expected recovery reason in event log: ", events)
	}
}
