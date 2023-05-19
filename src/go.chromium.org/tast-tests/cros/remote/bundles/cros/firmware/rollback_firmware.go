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
	"go.chromium.org/tast-tests/cros/common/flashrom"

	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"

	"go.chromium.org/tast-tests/cros/common/firmware/bios"

	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
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
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		Params: []testing.Param{{
			Name:    "normal",
			Fixture: fixture.NormalMode,
		}, {
			Name:    "dev",
			Fixture: fixture.DevModeGBB,
		},
		},
	})
}

func RollbackFirmware(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	var cutoffEvent reporters.Event

	func() {
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Failed to require BiosServiceClient: ", err)
		}

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

		remoteTmpFile, err = h.ServoProxy.OutputCommand(ctx, false, "mktemp", "-d", "-p", "/var/tmp", "-t", "fwservoXXXXXX")
		if err != nil {
			s.Fatal("Failed to create remote temp dir")
		}
		servoTempDir := strings.TrimSuffix(string(remoteTmpFile), "\n")

		cleanupContext := ctx
		ctx, closeFunc := ctxutil.Shorten(ctx, 1*time.Minute)
		defer closeFunc()
		defer func(ctx context.Context) {
			s.Log("Cleaning up")
			if err := h.RequireServo(ctx); err != nil {
				s.Fatal("Failed to require servo: ", err)
			}
			if shouldRestoreFirmware {
				func() {
					var flashromConfig flashrom.Config
					flashromInstance, ctx, shutdown, _, err := flashromConfig.
						FlashromInit("").
						SetServoProxy(h.ServoProxy).
						Probe(ctx)
					defer func() {
						if err := shutdown(); err != nil {
							s.Error("Failed to shutdown flashromInstance: ", err)
						}
					}()
					if err != nil {
						s.Error("Failed to create flashrom instance: ", err)
						return
					}
					if out, err := flashromInstance.Write(ctx, "", true /*noVerifyAll=*/, false /*noverify=*/, "", []string{
						fmt.Sprintf("%s:%s", bios.FWSignAImageSection, fmt.Sprintf("%s/vb.a", servoTempDir)),
						fmt.Sprintf("%s:%s", bios.FWSignBImageSection, fmt.Sprintf("%s/vb.b", servoTempDir)),
					}); err != nil {
						s.Errorf("Failed to restore A/B signatures: %v output = %s", err, string(out))
					}
				}()
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

		s.Log("Backing up AP firmware")
		backupInfo, err := h.BiosServiceClient.BackupImageSection(ctx, &fwpb.FWSectionInfo{
			Section:    fwpb.ImageSection_EmptyImageSection,
			Programmer: fwpb.Programmer_BIOSProgrammer,
			Path:       remoteTempDir,
		})
		if err != nil {
			s.Fatal("Failed to backup current AP firmware: ", err)
		}
		s.Log("Backup on DUT written to ", backupInfo.Path)

		// TODO(b/276861597): Use futility library.
		if err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", backupInfo.Path, "-x",
			fmt.Sprintf("%s:%s/vb.a", bios.FWSignAImageSection, remoteTempDir), fmt.Sprintf("VBLOCK_B:%s/vb.b", remoteTempDir),
			fmt.Sprintf("FW_MAIN_A:%s/fw.a", remoteTempDir), fmt.Sprintf("FW_MAIN_B:%s/fw.b", remoteTempDir),
		).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to extract sections: ", err)
		}
		s.Log("Downloading files to ", localTempDir)
		if err := linuxssh.GetFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/vb.a", remoteTempDir), fmt.Sprintf("%s/vb.a", localTempDir), linuxssh.DereferenceSymlinks); err != nil {
			s.Fatal("Failed to download vb.a: ", err)
		}
		if err := linuxssh.GetFile(ctx, h.DUT.Conn(), fmt.Sprintf("%s/vb.b", remoteTempDir), fmt.Sprintf("%s/vb.b", localTempDir), linuxssh.DereferenceSymlinks); err != nil {
			s.Fatal("Failed to download vb.b: ", err)
		}
		s.Log("Copying files to servohost ", servoTempDir)
		if err := h.ServoProxy.PutFiles(ctx, false, map[string]string{
			fmt.Sprintf("%s/vb.a", localTempDir): fmt.Sprintf("%s/vb.a", servoTempDir),
			fmt.Sprintf("%s/vb.b", localTempDir): fmt.Sprintf("%s/vb.b", servoTempDir),
		}); err != nil {
			s.Fatal("Failed to copy files to servo host: ", err)
		}
		// Change the firmware version to 0 and resign. The normal firmware version is 1 or more, so 0 will be a rollback.
		resignedVBlockA := fmt.Sprintf("%s/resigned-vb.a", remoteTempDir)
		resignedVBlockB := fmt.Sprintf("%s/resigned-vb.b", remoteTempDir)
		// TODO(b/276861597): Use futility library.
		if err := h.DUT.Conn().CommandContext(ctx, "futility", "vbutil_firmware", "--vblock", resignedVBlockA,
			"--fv", fmt.Sprintf("%s/fw.a", remoteTempDir), "--version", "0",
			"--keyblock", "/usr/share/vboot/devkeys/firmware.keyblock", "--signprivate", "/usr/share/vboot/devkeys/firmware_data_key.vbprivk",
			"--kernelkey", "/usr/share/vboot/devkeys/kernel_subkey.vbpubk",
		).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to sign vb.a: ", err)
		}
		// TODO(b/276861597): Use futility library.
		if err := h.DUT.Conn().CommandContext(ctx, "futility", "vbutil_firmware", "--vblock", resignedVBlockB,
			"--fv", fmt.Sprintf("%s/fw.b", remoteTempDir), "--version", "0",
			"--keyblock", "/usr/share/vboot/devkeys/firmware.keyblock", "--signprivate", "/usr/share/vboot/devkeys/firmware_data_key.vbprivk",
			"--kernelkey", "/usr/share/vboot/devkeys/kernel_subkey.vbpubk",
		).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to sign vb.b: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "truncate", "-r", fmt.Sprintf("%s/vb.a", remoteTempDir), resignedVBlockA).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to extend vb.a: ", err)
		}
		if err := h.DUT.Conn().CommandContext(ctx, "truncate", "-r", fmt.Sprintf("%s/vb.b", remoteTempDir), resignedVBlockB).Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to extend vb.b: ", err)
		}

		activeFW, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
		if err != nil {
			s.Fatal("Failed to get active FW: ", err)
		}
		var flashOrder []bios.ImageSection
		var flashPaths []string
		if activeFW == string(fwCommon.RWSectionA) {
			flashOrder = []bios.ImageSection{bios.FWSignAImageSection, bios.FWSignBImageSection}
			flashPaths = []string{resignedVBlockA, resignedVBlockB}
		} else {
			flashOrder = []bios.ImageSection{bios.FWSignBImageSection, bios.FWSignAImageSection}
			flashPaths = []string{resignedVBlockB, resignedVBlockA}
		}

		var flashromConfig flashrom.Config
		flashromInstance, ctx, shutdown, _, err := flashromConfig.
			FlashromInit("").
			SetDut(h.DUT).
			ProgrammerInit(flashrom.ProgrammerHost, "").
			Probe(ctx)
		defer func() {
			if err := shutdown(); err != nil {
				s.Error("Failed to shutdown flashromInstance: ", err)
			}
		}()
		if err != nil {
			s.Fatal("Flashrom probe failed, unable to build flashrom instance: ", err)
		}

		shouldRestoreFirmware = true
		s.Log("Rolling back ", flashOrder[0])
		if out, err := flashromInstance.Write(ctx, "", true /*noVerifyAll=*/, false /*noverify=*/, "", []string{
			fmt.Sprintf("%s:%s", flashOrder[0], flashPaths[0]),
		}); err != nil {
			s.Errorf("Failed to flash: %v output = %s", err, string(out))
		}
		ms, err := firmware.NewModeSwitcher(ctx, h)
		if err != nil {
			s.Fatal("Creating mode switcher: ", err)
		}
		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset); err != nil {
			s.Fatal("Failed to reboot after rolling back 1 firmware: ", err)
		}
		newFW, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
		if err != nil {
			s.Error("Failed to get active FW: ", err)
		}
		if activeFW == newFW {
			s.Errorf("Booted to wrong FW, got %q, want !%q", newFW, activeFW)
		}

		oldEvents, err := h.Reporter.EventlogList(ctx)
		if err != nil {
			s.Fatal("Finding last event: ", err)
		}
		if len(oldEvents) > 0 {
			cutoffEvent = oldEvents[len(oldEvents)-1]
		}

		s.Log("Rolling back ", flashOrder[1])
		if out, err := flashromInstance.Write(ctx, "", true /*noVerifyAll=*/, false /*noverify=*/, "", []string{
			fmt.Sprintf("%s:%s", flashOrder[1], flashPaths[1]),
		}); err != nil {
			s.Errorf("Failed to flash: %v output = %s", err, string(out))
		}
		if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.SkipWaitConnect); err != nil {
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
