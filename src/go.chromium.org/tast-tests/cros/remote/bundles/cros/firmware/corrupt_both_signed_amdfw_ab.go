// Copyright 2022 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"context"
	"os"
	"strconv"
	"time"

	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/signedamdfw"
	fwUtils "go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	pb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
)

func init() {
	testing.AddTest(&testing.Test{
		Func:         CorruptBothSignedAMDFWAB,
		Desc:         "Servo based both A and B signed AMDFW corruption test. This test requires a USB disk with ChromeOS test image plugged-in. This test corrupts both A and B SIGNED_AMDFW FMAP section. On next reboot, the firmware verification fails and enters recovery mode. This test then checks the success of the recovery boot",
		Contacts:     []string{"chromeos-faft@google.com", "kramasub@google.com"},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		Attr:         []string{"group:firmware", "firmware_bios"},
		Requirements: []string{"sys-fw-0021-v01", "sys-fw-0024-v01", "sys-fw-0025-v01"},
		HardwareDeps: hwdep.D(hwdep.ChromeEC()),
		Timeout:      50 * time.Minute,
		Vars:         []string{"firmware.skipFlashUSB"},
		SoftwareDeps: []string{"crossystem", "flashrom", "amd_cpu"},
		ServiceDeps:  []string{"tast.cros.firmware.BiosService", "tast.cros.firmware.UtilsService"},
		Params: []testing.Param{
			{
				Name:    "normal_mode",
				Fixture: fixture.NormalMode,
				Val:     "normal",
			},
			{
				Name:    "dev_mode",
				Fixture: fixture.DevMode,
				Val:     "developer",
			},
		},
	})
}

func CorruptBothSignedAMDFWAB(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to init servo: ", err)
	}

	if err := h.RequireBiosServiceClient(ctx); err != nil {
		s.Fatal("Requiring BiosServiceClient: ", err)
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}

	if ok := signedamdfw.CheckForSignedAMDFWSection(ctx, s, h); !ok {
		s.Log("Skipping test since SIGNED_AMDFW* section doesnot exist")
		return
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 15*time.Minute)
	defer cancel()

	s.Log("Backup SIGNED_AMDFW_A section")
	AMDFWABkp, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{Section: pb.ImageSection_SignedAMDFWAImageSection, Programmer: pb.Programmer_BIOSProgrammer})
	if err != nil {
		s.Fatal("Failed to backup current SIGNED_AMDFW_A region: ", err)
	}
	defer func(ctx context.Context) {
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", AMDFWABkp.Path).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete SIGNED_AMDFW_A backup: ", err)
		}
	}(cleanupContext)

	s.Log("Backup SIGNED_AMDFW_B section")
	AMDFWBBkp, err := h.BiosServiceClient.BackupImageSection(ctx, &pb.FWSectionInfo{Section: pb.ImageSection_SignedAMDFWBImageSection, Programmer: pb.Programmer_BIOSProgrammer})
	if err != nil {
		s.Fatal("Failed to backup current SIGNED_AMDFW_B region: ", err)
	}
	defer func(ctx context.Context) {
		if _, err := h.DUT.Conn().CommandContext(ctx, "rm", AMDFWBBkp.Path).Output(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to delete SIGNED_AMDFW_B backup: ", err)
		}
	}(cleanupContext)

	s.Log("Copy backup files to the Host")
	AMDFWADst, err := os.CreateTemp("", "AMDFWABackup")
	if err != nil {
		s.Fatal("Failed to create temporary file for SIGNED_AMDFW_A backup")
	}
	defer func() {
		AMDFWADst.Close()
		os.Remove(AMDFWADst.Name())
	}()

	AMDFWBDst, err := os.CreateTemp("", "AMDFWBBackup")
	if err != nil {
		s.Fatal("Failed to create temporary file for SIGNED_AMDFW_B backup")
	}
	defer func() {
		AMDFWBDst.Close()
		os.Remove(AMDFWBDst.Name())
	}()

	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), AMDFWABkp.Path, AMDFWADst.Name(), linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy SIGNED_AMDFW_A backup to the Host")
	}
	if err := linuxssh.GetFile(ctx, s.DUT().Conn(), AMDFWBBkp.Path, AMDFWBDst.Name(), linuxssh.PreserveSymlinks); err != nil {
		s.Fatal("Failed to copy SIGNED_AMDFW_B backup to the Host")
	}

	s.Log("Setup USB Key")
	skipFlashUSB := false
	if skipFlashUSBStr, ok := s.Var("firmware.skipFlashUSB"); ok {
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

	// Restore SIGNED_AMDFW*
	defer func(ctx context.Context) {
		// Disable wp so backup can be restored.
		if err := fwUtils.SetFWWriteProtect(ctx, h, false); err != nil {
			s.Fatal("Failed to set FW write protect state: ", err)
		}

		if err := h.RequireServo(ctx); err != nil {
			s.Fatal("Failed to init servo: ", err)
		}

		s.Log("Syncing TAST File from HOST")
		if err := h.SyncTastFilesToDUT(ctx); err != nil {
			s.Log(err, "syncing Tast files to DUT after booting to recovery")
		}

		// Require again here since reboots in test cause nil pointer errors otherwise.
		if err := h.RequireBiosServiceClient(ctx); err != nil {
			s.Fatal("Requiring BiosServiceClient: ", err)
		}

		s.Log("Get back SIGNED_AMDFW* backup from host to DUT")
		if _, err := linuxssh.PutFiles(ctx, s.DUT().Conn(), map[string]string{AMDFWADst.Name(): AMDFWABkp.Path, AMDFWBDst.Name(): AMDFWBBkp.Path}, linuxssh.PreserveSymlinks); err != nil {
			s.Fatal("Failed to get backup files to DUT from Host")
		}

		s.Log("Restore firmware bodies")
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, AMDFWABkp); err != nil {
			s.Fatal("Failed to restore SIGNED_AMDFW_A: ", err)
		}
		if _, err := h.BiosServiceClient.RestoreImageSection(ctx, AMDFWBBkp); err != nil {
			s.Fatal("Failed to restore SIGNED_AMDFW_B: ", err)
		}

		bootModeName := s.Param().(string)

		if bootModeName == "developer" {
			if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AssumeRecoveryMode, firmware.ExpectDevModeAfterReboot); err != nil {
				s.Fatal("Failed to perform mode aware reboot: ", err)
			}
		} else {
			if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AssumeRecoveryMode); err != nil {
				s.Fatal("Failed to perform mode aware reboot: ", err)
			}
		}

		s.Log(ctx, "Reestablishing connection to DUT")
		connectCtx, cancel := context.WithTimeout(ctx, 10*time.Minute)
		defer cancel()
		if err := h.WaitConnect(connectCtx); err != nil {
			s.Fatal("Failed to reconnect to DUT after booting to recovery mode: ", err)
		}

		if mainFWType, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwType); err != nil {
			s.Fatal("Failed to get crossystem mainfw_type: ", err)
		} else if mainFWType != bootModeName {
			s.Fatalf("Failed to match mainfw_type: got %q, want %q", mainFWType, bootModeName)
		}

	}(cleanupContext)

	s.Log("Corrupt SIGNED_AMDFW_A")
	if _, err := h.BiosServiceClient.CorruptFWSection(ctx, &pb.FWSectionInfo{Section: pb.ImageSection_SignedAMDFWAImageSection, Programmer: pb.Programmer_BIOSProgrammer}); err != nil {
		s.Fatal("Failed to corrupt SIGNED_AMDFW_A section: ", err)
	}

	s.Log("Corrupt SIGNED_AMDFW_B")
	if _, err := h.BiosServiceClient.CorruptFWSection(ctx, &pb.FWSectionInfo{Section: pb.ImageSection_SignedAMDFWBImageSection, Programmer: pb.Programmer_BIOSProgrammer}); err != nil {
		s.Fatal("Failed to corrupt SIGNED_AMDFW_B section: ", err)
	}

	s.Log("Copy TAST Files from DUT")
	if err := h.CopyTastFilesFromDUT(ctx); err != nil {
		s.Fatal(err, "copying Tast files from DUT to test server")
	}

	if err := h.DUT.Conn().CommandContext(ctx, "sync").Run(ssh.DumpLogOnError); err != nil {
		s.Fatal("Failed to sync DUT: ", err)
	}

	s.Log("Check recovery reason")
	if err := fwUtils.CheckRecReason(ctx, h, ms, []reporters.RecoveryReason{reporters.RecoveryReasonROInvalidRW, reporters.RecoveryReasonRWVendorBlob}); err != nil {
		s.Fatal("Failed when checking recovery reason: ", err)
	}
}
