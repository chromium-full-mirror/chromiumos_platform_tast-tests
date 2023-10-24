// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"strconv"
	"strings"
	"time"

	common "go.chromium.org/tast-tests/cros/common/firmware"
	fwCommon "go.chromium.org/tast-tests/cros/common/firmware"
	"go.chromium.org/tast-tests/cros/common/testexec"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FWCorruptRecoveryCache,
		Desc: "Corrupt recovery cache and then check it's rebuilt",
		Contacts: []string{
			"chromeos-faft@google.com",
			"tij@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO: When stable, change firmware_unstable to a different attr.
		Attr:         []string{"group:firmware", "firmware_unstable", "firmware_usb"},
		Timeout:      20 * time.Minute,
		Vars:         []string{"firmware.skipFlashUSB"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		SoftwareDeps: []string{"has_recovery_mrc_cache"},
		Params: []testing.Param{
			{
				Name:    "normal",
				Fixture: fixture.NormalMode,
				Val:     common.BootModeNormal,
			},
			{
				Name:    "dev",
				Fixture: fixture.DevMode,
				Val:     common.BootModeDev,
			},
		},
	})
}

// FWCorruptRecoveryCache checks for RECOVERY_MRC_CACHE, voids it
// and then goes back to recovery mode to see if it's rebuilt again.
func FWCorruptRecoveryCache(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	bootMode := s.Param().(common.BootMode)

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to connect to servo: ", err)
	}

	s.Log("Verifying RECOVERY_MRC_CACHE section exists")
	err := h.DUT.Conn().CommandContext(ctx, "futility", "read", "-r", "RECOVERY_MRC_CACHE", "/dev/null").Run()
	if err != nil {
		s.Fatal("Failed to run cmd on DUT: ", err)
	} else if errCode, ok := testexec.ExitCode(err); !ok || errCode != 0 {
		s.Fatal("Failed to find RECOVERY_MRC_CACHE section: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Failed to create new boot mode switcher: ", err)
	}

	s.Log("Setup USB Key")
	skipFlashUSB := false
	if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
		var err error
		skipFlashUSB, err = strconv.ParseBool(skipFlashUSBStr)
		if err != nil {
			s.Fatalf("Invalid value for var firmware.skipFlashUSB: got %q, want true/false", skipFlashUSBStr)
		}
	}
	cs := s.CloudStorage()
	if skipFlashUSB {
		cs = nil
	}
	if err := h.SetupUSBKey(ctx, cs); err != nil {
		s.Fatal("USBKey not working: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Failed to require BiosServiceClient: ", err)
	}

	mrcHostBackup, err := os.CreateTemp("", "mrcHostBackup")
	if err != nil {
		s.Fatal("Failed to create temporary dir for Recovery MRC Cache backup")
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 5*time.Minute)
	defer cancel()
	defer func(ctx context.Context) {
		mrcHostBackup.Close()
		if err := os.Remove(mrcHostBackup.Name()); err != nil {
			s.Log("Failed to delete MRC cache back up dir from host")
		}
	}(cleanupContext)

	s.Log("Backing up current RECOVERY_MRC_CACHE section for safety")
	mrcPath, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{
		Programmer: pb.Programmer_BIOSProgrammer,
		Section:    pb.ImageSection_RECOVERYMRCCACHEImageSection,
	})
	if err != nil {
		s.Fatal("Failed to backup current RECOVERY_MRC_CACHE region: ", err)
	}
	s.Log("RECOVERY_MRC_CACHE region backup is stored at: ", mrcPath.Path)

	defer func(ctx context.Context) {
		h.DisconnectDUT(ctx)
		s.Log("Reconnecting to BiosService on DUT")
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Failed to reconnect to BiosServiceClient on DUT: ", err)
		}
		s.Log("Restoring RECOVERY_MRC_CACHE image")
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, mrcPath); err != nil {
			s.Error("Failed to restore MRC_RECOVERY_CACHE image: ", err)
		}

		s.Log("Removing RECOVERY_MRC_CACHE image backups from DUT")
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", mrcPath.Path).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete RECOVERY_MRC_CACHE image from DUT: ", err)
		}

		s.Log("Rebooting out of recovery")
		if err := ms.RebootToMode(ctx, bootMode, firmware.AssumeGBBFlagsCorrect); err != nil {
			s.Fatalf("Failed to reboot into %v mode: %v", bootMode, err)
		}
	}(cleanupContext)

	if !h.DoesServerHaveTastHostFiles() {
		if err := h.CopyTastFilesFromDUT(ctx); err != nil {
			s.Fatal("Copying Tast files to Host failed: ", err)
		}
	}
	s.Log("Copying kernel back up to host")
	if err := linuxssh.GetFile(ctx, h.DUT.Conn(), mrcPath.Path, mrcHostBackup.Name(), linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy mrc backup to the host")
	}

	defer func(ctx context.Context) {
		s.Log("Wait for DUT to reconnect")
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Fatal("Failed to ensure the DUT is booted: ", err)
		}

		if err := h.SyncTastFilesToDUT(ctx); err != nil {
			s.Fatal("Copying Tast files to DUT failed: ", err)
		}

		s.Log("Sync MRC backup from host to DUT")
		if _, err := linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{
			mrcHostBackup.Name(): mrcPath.Path,
		}, linuxssh.DereferenceSymlinks); err != nil {
			s.Fatal("Failed to get backup files to DUT from host")
		}
	}(cleanupContext)

	s.Log("Corrupting RECOVERY_MRC_CACHE section")
	if _, err := h.BiosServiceClient.CorruptFWSection(ctx,
		&pb.FWSectionInfo{
			Section:    pb.ImageSection_RECOVERYMRCCACHEImageSection,
			Programmer: pb.Programmer_BIOSProgrammer,
		}); err != nil {
		s.Fatal("Failed to corrupt RECOVERY_MRC_CACHE section: ", err)
	}

	s.Log("Rebooting into recovery mode to rebuild RECOVERY_MRC_CACHE")
	if err := ms.RebootToMode(ctx, fwCommon.BootModeRecovery); err != nil {
		s.Fatal("Failed to reboot into recovery mode: ", err)
	}
	h.DisconnectDUT(ctx)

	s.Log("Reconnecting to DUT")
	if err := h.WaitConnect(ctx); err != nil {
		s.Fatal("Failed to reconnect to DUT: ", err)
	}

	s.Log("Checking if recovery MRC cache has been rebuilt")
	const cbmemCheckCommand = `cbmem -1 | grep -e MRC -e APOB`
	out, err := h.DUT.Conn().CommandContext(ctx, "bash", "-c", cbmemCheckCommand).Output()
	if err != nil {
		s.Fatalf("Failed to grep for MRC from cbmem, got output %v with error: %v", string(out), err)
	}
	outstr := string(out)
	s.Log("Got output from cbmem: ", outstr)
	if !strings.Contains(outstr, "MRC: cache data 'RECOVERY_MRC_CACHE' needs update.") &&
		!strings.Contains(outstr, "APOB RAM hash differs from flash") {
		s.Fatal("Output from cbmem did not contain expected message: ", err)
	}
}
