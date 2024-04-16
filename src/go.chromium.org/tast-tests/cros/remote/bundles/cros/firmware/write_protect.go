// Copyright 2021 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"fmt"
	"os"
	"time"

	"go.chromium.org/tast-tests/cros/common/flashrom"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: WriteProtect,
		Desc: "Verify enabling and disabling write protect works as expected",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable"},
		SoftwareDeps: []string{"crossystem", "flashrom"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      20 * time.Minute,
		LacrosStatus: testing.LacrosVariantUnneeded,
		Params: []testing.Param{
			{
				Name:    "ec",
				Fixture: fixture.NormalMode,
				Val:     targetEC,
			},
			{
				Name:    "ec_dev",
				Fixture: fixture.DevModeGBB,
				Val:     targetEC,
			},
			{
				Name:    "ap",
				Fixture: fixture.NormalMode,
				Val:     targetBIOS,
			},
			{
				Name:    "ap_dev",
				Fixture: fixture.DevModeGBB,
				Val:     targetBIOS,
			},
		},
	})
}

type wpTarget string

const (
	targetBIOS wpTarget = "bios"
	targetEC   wpTarget = "ec"
)

var wpTargetToProg = map[wpTarget]pb.Programmer{
	targetBIOS: pb.Programmer_BIOSProgrammer,
	targetEC:   pb.Programmer_ECProgrammer,
}

var wpTargetToRegion = map[wpTarget]pb.ImageSection{
	targetBIOS: pb.ImageSection_FWSignBImageSection,
	// In EC, WP only protects RO sections (with flashwp enable).
	// In devices with CBI in flash (eg rex, brox), CBI is part of RO.
	// Corrupting entire RO might messup CBI and pstate flags which may cause prevent booting,
	// so we on only corrupt a small section of the RO.
	targetEC: pb.ImageSection_ROFRIDImageSection,
}

func WriteProtect(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	target := s.Param().(wpTarget) // Targets either EC or AP for the rest of test.

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to require configs: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Failed requiring BiosServiceClient: ", err)
	}
	hostBackup, err := os.CreateTemp("", "fwBackup")
	if err != nil {
		s.Fatal("Failed to create temporary file for host backup")
	}
	defer func() {
		hostBackup.Close()
		os.Remove(hostBackup.Name())
	}()

	testing.ContextLog(ctx, "Read current fw image")
	roBefore, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Section:    wpTargetToRegion[target],
		Programmer: wpTargetToProg[target],
		Path:       "/mnt/stateful_partition",
	})
	if err != nil {
		s.Fatal("Failed to save initial fw image: ", err)
	}
	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 10*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		s.Log("Deleting saved fw")
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", roBefore.Path).Output(ssh.DumpLogOnError); err != nil {
			// Might be deleted already so don't fail for this.
			testing.ContextLog(ctx, "Failed to delete backup files: ", err)
		}
	}(cleanupContext)

	s.Log("Copying fw back up to host")
	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), roBefore.Path, hostBackup.Name(), linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy fw backup to the host")
	}

	// Only restore firmware if it was unexpectedly corrupted.
	needsRestore := false
	defer func(ctx context.Context) {
		testing.ContextLog(ctx, "Make sure DUT is connected before cleanup")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to connect to the DUT: ", err)
		}

		testing.ContextLog(ctx, "Disable Write Protect")
		if err := setWriteProtect(ctx, h, target, false); err != nil {
			s.Error("Failed to disable FW write protect state: ", err)
		}

		if needsRestore {
			testing.ContextLog(ctx, "Fw may have been modified, restore original fw from backup: ", roBefore.Path)
			if _, err := h.BiosServiceClient.RestoreImageSection(ctx, roBefore); err != nil {
				s.Fatal("Failed to restore fw image: ", err)
			}
		}
	}(cleanupContext)

	testing.ContextLog(ctx, "Enable Write Protect")
	if err := setWriteProtect(ctx, h, target, true); err != nil {
		s.Fatal("Failed to set FW write protect state: ", err)
	}

	needsRestore = true // In case flashrom completes a partial write but still has errors.
	testing.ContextLog(ctx, "Attempt to overwrite fw with write protect enabled")
	if _, err := h.BiosServiceClient.CorruptFWSection(ctx, &pb.FWSectionInfo{
		Section:    wpTargetToRegion[target],
		Programmer: wpTargetToProg[target],
		Path:       "/mnt/stateful_partition",
	}); err == nil {
		// Flashrom command sometimes prints "SUCCESS" (the criteria for a successful write) even if the write didn't succeed.
		// Which means we cannot rely on the error/lack of error from the CorruptFWSection to determine if the write was successful.
		testing.ContextLog(ctx, "Expected flashrom write to fail since wp is enabled")
	}

	testing.ContextLog(ctx, "Disable Write Protect")
	if err := setWriteProtect(ctx, h, target, false); err != nil {
		s.Fatal("Failed to disable FW write protect state: ", err)
	}

	testing.ContextLog(ctx, "Read fw, make sure write didn't succeed with wp enabled")
	roAfter, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Section:    wpTargetToRegion[target],
		Programmer: wpTargetToProg[target],
		Path:       "/mnt/stateful_partition",
	})
	if err != nil {
		s.Fatal("Failed to save current fw image: ", err)
	}

	s.Log("Sync initial fw backup from host to DUT")
	if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(), map[string]string{
		hostBackup.Name(): roBefore.Path,
	}, linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to get backup files to DUT from host")
	}

	// Fw attempted to be overwritten with wp enabled, verify it's still original roBefore fw.
	s.Logf("Compare earlier file %v to current fw %v to verify write failed", roBefore.Path, roAfter.Path)
	if out, err := h.DUT.Conn().CommandContext(ctx, "cmp", roBefore.Path, roAfter.Path).CombinedOutput(ssh.DumpLogOnError); err != nil {
		// Cmp error code 0 == files match, 1 == files differ, 2 == error in running cmp.
		if errCode, ok := testexec.ExitCode(err); !ok || errCode == 2 {
			s.Fatalf("Failed to compare %q and %q using 'cmp': %v", roBefore.Path, roAfter.Path, err)
		} else if errCode == 1 && ok {
			s.Fatalf("Files %q and %q were not identical, so either write protect or read failed: %s, got error: %v", roBefore.Path, roAfter.Path, string(out), err)
		} else {
			s.Fatalf("Unexpected error using 'cmp', error code %v with output: %v", errCode, string(out))
		}
	}

	// Fw was not corrupted as expected, restore not necessary.
	needsRestore = false
}

// setWriteProtect uses gsc_ecrst_pulse after setting hw wp, which deletes all temp data/files.
func setWriteProtect(ctx context.Context, h *firmware.Helper, target wpTarget, enable bool) (retErr error) {
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		return errors.Wrap(err, "failed to create mode switcher")
	}

	// Make sure hardware wp is disabled for now so flashrom cmd can run.
	if out, err := h.Servo.GetString(ctx, servo.FWWPState); err != nil || out != string(servo.FWWPStateOff) {
		// If fw wp is enabled or unknown, disable and reboot so ap wp can be changed.
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOff); err != nil {
			return errors.Wrap(err, "failed to disable firmware write protect")
		}

		testing.ContextLog(ctx, "Performing mode aware reboot")
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, firmware.AllowGBBForce); err != nil {
			return errors.Wrap(err, "failed to perform mode aware reboot")
		}
	}

	enableStr := "enable"
	if !enable {
		enableStr = "disable"
	}

	if target == targetBIOS {
		var flashromConfig flashrom.Config
		flash, ctx, shutdown, _, err := flashromConfig.
			FlashromInit(flashrom.VerbosityDebug).
			ProgrammerInit(flashrom.ProgrammerHost, "").
			SetDut(h.DUT).
			Probe(ctx)
		defer func() {
			if err := shutdown(); err != nil {
				if retErr == nil {
					retErr = errors.Wrap(err, "failed to shutdown flashromInstance")
				} else {
					testing.ContextLog(ctx, "Failed to shutdown flashromInstance: ", err)
				}
			}
		}()
		if err != nil {
			return errors.Wrap(err, "flashrom probe failed, unable to build flashrom instance")
		}
		apWPFunc := flash.SoftwareWriteProtectEnable
		if !enable {
			apWPFunc = flash.SoftwareWriteProtectDisable
		}

		testing.ContextLogf(ctx, "Set AP wp to %s", enableStr)
		if _, err := apWPFunc(ctx); err != nil {
			return errors.Wrapf(err, "failed to set AP wp to %s", enableStr)
		}

	} else {
		if err := h.Servo.RunECCommand(ctx, fmt.Sprintf("flashwp %v", enableStr)); err != nil {
			return errors.Wrapf(err, "failed to %s flashwp", enableStr)
		}
	}

	if enable {
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
			return errors.Wrapf(err, "failed to %s firmware write protect", enableStr)
		}
	}

	testing.ContextLog(ctx, "Performing mode aware reboot")
	if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, firmware.AllowGBBForce); err != nil {
		return errors.Wrap(err, "failed to perform mode aware reboot")
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		testing.ContextLog(ctx, "Failed requiring BiosServiceClient: ", err)
	}
	return nil
}
