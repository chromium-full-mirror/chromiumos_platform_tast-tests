// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"fmt"
	"strings"
	"time"

	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"gopkg.in/yaml.v2"
)

func init() {
	testing.AddTest(&testing.Test{
		Func: FWAutoupdate,
		Desc: "Verify autoupdate from and rollback to old firmware builds",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		TestBedDeps:  []string{tbdep.ServoStateWorking},
		Vars:         []string{"firmware.apro", "firmware.aprw", "firmware.ecro", "firmware.ecrw"},
		Timeout:      2 * time.Hour,
		// Fixture:      fixture.BootModeFixtureWithAPBackup(fixture.NormalMode),
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(),
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		SoftwareDeps: []string{"crossystem"},
	})
}

type firmwareVersions struct {
	AP struct {
		Versions struct {
			RO string `yaml:"ro"`
			RW string `yaml:"rw"`
			// The EC version embedded within the AP-RW image.
			ECRW string `yaml:"ecrw"`
		} `yaml:"versions"`
	} `yaml:"host"`
	EC struct {
		Versions struct {
			RO string `yaml:"ro"`
			RW string `yaml:"rw"`
		} `yaml:"versions"`
	} `yaml:"ec"`
}
type versionJSON map[string]firmwareVersions

// FWAutoupdate expects the DUT to have the released RO/RW installed on the DUT as a precondition (--mode=recovery),
// then it will autoupdate to the version under test which is provided in the command line vars.
// Next it will attempt to downgrade back to the prior version.
func FWAutoupdate(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	cleanupContext := ctx
	ctx, cancel := ctxutil.Shorten(ctx, 1*time.Minute)
	defer cancel()

	apROURL := s.RequiredVar("firmware.apro")
	apRWURL, ok := s.Var("firmware.aprw")
	if !ok {
		apRWURL = apROURL
	}
	ecROURL, ok := s.Var("firmware.ecro")
	if !ok {
		ecROURL = apROURL
	}
	ecRWURL, ok := s.Var("firmware.ecrw")
	if !ok {
		ecRWURL = apRWURL
	}

	if err := h.RequirePlatform(ctx); err != nil {
		s.Fatal("Requiring platform: ", err)
	}
	if err := h.RequireRPCUtils(ctx); err != nil {
		s.Fatal("Requiring RPC utils: ", err)
	}

	// Create a SSH redirect for the ephemeral devserver. If running from a workstation, this will be the only devserver.
	var redirectedDevServers []string
	for _, devserver := range s.CloudStorage().Devservers() {
		if strings.HasPrefix(devserver, "http://127.0.0.1:") {
			localHostPort := strings.TrimPrefix(devserver, "http://")
			remoteHostPort, err := h.DUT.Conn().ForwardRemoteToLocal("tcp", "127.0.0.1:0", localHostPort, nil)
			if err != nil {
				s.Fatal("Failed to forward port to DUT: ", err)
			}
			defer remoteHostPort.Close()
			redirectedDevServers = append(redirectedDevServers, "http://"+remoteHostPort.ListenAddr().String())
		}
	}

	// Read the config from the DUT to determine which images to download.
	fwTargets, err := h.RPCUtils.FirmwareBuildTargets(ctx, &fwpb.FirmwareBuildTargetsRequest{})
	if err != nil {
		s.Fatal("FirmwareBuildTargets: ", err)
	}

	s.Logf("Found the FW targets: %s", fwTargets.String())

	// Make a temp dir on the DUT
	tmpDirOut, err := h.DUT.Conn().CommandContext(ctx, "mktemp", "-d", "/usr/local/tmp/tast.firmware.FWAutoupdate.XXXXXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to create a temp dir: ", err)
	}
	tmpDir := strings.TrimSpace(string(tmpDirOut))

	defer func(ctx context.Context) {
		err := h.DUT.Conn().CommandContext(ctx, "rm", "-rf", tmpDir).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Error("Failed to delete temp dir: ", err)
		}
	}(cleanupContext)

	// Download all the necessary images
	apROFile := fmt.Sprintf("%s/apro.bin", tmpDir)
	_, err = h.RPCUtils.ExtractAPFirmwareImage(ctx, &fwpb.ExtractFirmwareImageRequest{
		PreferredDevserver: s.CloudStorage().Devservers(),
		BackupDevserver:    redirectedDevServers,
		Dest:               apROFile,
		Url:                apROURL,
		BuildTargets:       fwTargets,
		Board:              h.Board,
		Model:              h.Model,
	})
	if err != nil {
		s.Fatalf("Failed to extract AP RO from %q: %+v", apROURL, err)
	}
	s.Logf("Extracted AP RO: %s", apROFile)
	apRWFile := ""
	if apROURL != apRWURL {
		apRWFile = fmt.Sprintf("%s/aprw.bin", tmpDir)
		_, err = h.RPCUtils.ExtractAPFirmwareImage(ctx, &fwpb.ExtractFirmwareImageRequest{
			PreferredDevserver: s.CloudStorage().Devservers(),
			BackupDevserver:    redirectedDevServers,
			Dest:               apRWFile,
			Url:                apRWURL,
			BuildTargets:       fwTargets,
			Board:              h.Board,
			Model:              h.Model,
		})
		if err != nil {
			s.Fatalf("Failed to extract AP RW from %q: %+v", apRWURL, err)
		}
		s.Logf("Extracted AP RW: %s", apRWFile)
	}

	ecROFile := fmt.Sprintf("%s/ecro.bin", tmpDir)
	_, err = h.RPCUtils.ExtractECFirmwareImage(ctx, &fwpb.ExtractFirmwareImageRequest{
		PreferredDevserver: s.CloudStorage().Devservers(),
		BackupDevserver:    redirectedDevServers,
		Dest:               ecROFile,
		Url:                ecROURL,
		BuildTargets:       fwTargets,
		Board:              h.Board,
		Model:              h.Model,
	})
	if err != nil {
		s.Fatalf("Failed to extract EC RO from %q: %+v", ecROURL, err)
	}
	s.Logf("Extracted EC RO: %s", ecROFile)

	ecRWFile := ""
	if ecRWURL != apRWURL {
		ecRWFile = fmt.Sprintf("%s/ecrw.bin", tmpDir)
		if ecROURL == ecRWURL {
			ecRWFile = ecROFile
		} else {
			_, err = h.RPCUtils.ExtractECFirmwareImage(ctx, &fwpb.ExtractFirmwareImageRequest{
				PreferredDevserver: s.CloudStorage().Devservers(),
				BackupDevserver:    redirectedDevServers,
				Dest:               ecRWFile,
				Url:                ecRWURL,
				BuildTargets:       fwTargets,
				Board:              h.Board,
				Model:              h.Model,
			})
			if err != nil {
				s.Fatalf("Failed to extract EC RW from %q: %+v", ecRWURL, err)
			}
		}
		s.Logf("Extracted EC RW: %s", ecRWFile)
	}

	// Much of this logic is copied from src/platform/firmware/pack_firmware.py

	s.Log("Repacking shellball with new FW")
	// Create directory structure expected by chromeos-firmwareupdate --repack
	newMergedDir := fmt.Sprintf("%s/new", tmpDir)
	err = h.DUT.Conn().CommandContext(ctx, "mkdir", newMergedDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to mkdir a %q: %+v", newMergedDir, err)
	}
	newTargetDir := fmt.Sprintf("%s/%s", newMergedDir, fwTargets.FirmwareManifestKey)
	err = h.DUT.Conn().CommandContext(ctx, "mkdir", newTargetDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to mkdir a %q: %+v", newTargetDir, err)
	}
	// Copy RO files to newMergedDir
	mergedAPFile := fmt.Sprintf("%s/image-%s.bin", newMergedDir, fwTargets.FirmwareManifestKey)
	err = h.DUT.Conn().CommandContext(ctx, "cp", apROFile, mergedAPFile).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to cp apro to %q: %+v", newMergedDir, err)
	}
	err = h.DUT.Conn().CommandContext(ctx, "cp", ecROFile, fmt.Sprintf("%s/ec.bin", newTargetDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to cp ecro to %q: %+v", newTargetDir, err)
	}
	// Pack AP-RW into merged.bin (RW_LEGACY & RW_MISC are optional)
	if apRWFile != "" {
		err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", apRWFile, "-x", fmt.Sprintf("RW_SECTION_A:%s/a.bin", tmpDir), "-x", fmt.Sprintf("RW_SECTION_B:%s/b.bin", tmpDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to futility dump_fmap: %+v", err)
		}
		err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", mergedAPFile, fmt.Sprintf("RW_SECTION_A:%s/a.bin", tmpDir), fmt.Sprintf("RW_SECTION_B:%s/b.bin", tmpDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to futility load_fmap: %+v", err)
		}
		// RW_LEGACY is optional
		err = h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", apRWFile, "-x", fmt.Sprintf("RW_LEGACY:%s/legacy.bin", tmpDir)).Run(ssh.DumpLogOnError)
		if err == nil {
			err := h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", mergedAPFile, fmt.Sprintf("RW_LEGACY:%s/legacy.bin", tmpDir)).Run(ssh.DumpLogOnError)
			if err != nil {
				s.Fatalf("Failed to futility load_fmap: %+v", err)
			}
		}
		// RW_MISC is optional
		err = h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", apRWFile, "-x", fmt.Sprintf("RW_MISC:%s/misc.bin", tmpDir)).Run(ssh.DumpLogOnError)
		if err == nil {
			err := h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", mergedAPFile, fmt.Sprintf("RW_MISC:%s/misc.bin", tmpDir)).Run(ssh.DumpLogOnError)
			if err != nil {
				s.Fatalf("Failed to futility load_fmap: %+v", err)
			}
		}
	}
	// Pack EC-RW into merged.bin
	if ecRWFile != "" {
		args := []string{"-i", mergedAPFile, "-e", ecRWFile}
		configFile := strings.Replace(ecRWFile, ".bin", ".config", 1)
		err := h.DUT.Conn().CommandContext(ctx, "test", "-f", configFile).Run(ssh.DumpLogOnError)
		if err != nil {
			args = append(args, "--ec_config", configFile)
		}
		err = h.DUT.Conn().CommandContext(ctx, "/usr/share/vboot/bin/swap_ec_rw", args...).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to swap_ec_rw: %+v", err)
		}
	}
	// Pack EC-RW into ec.bin
	out, err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", ecROFile, "-p").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility dump_fmap: %+v", err)
	}
	ecRWSection := ""
	lines := bytes.Split(out, []byte("\n"))

	for _, line := range lines {
		pieces := bytes.SplitN(line, []byte(" "), 2)
		if string(pieces[0]) == "RW_FW" || string(pieces[0]) == "EC_RW" {
			ecRWSection = string(pieces[0])
			break
		}
	}
	if ecRWSection == "" {
		s.Fatalf("Did not locate EC RW FMAP section in: %s", string(out))
	}
	if ecRWFile != "" {
		err := h.DUT.Conn().CommandContext(ctx, "futility", "dump_fmap", ecRWFile, "-x", fmt.Sprintf("EC_RW:%s/ecrw.raw", tmpDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to futility dump_fmap: %+v", err)
		}
	} else {
		err := h.DUT.Conn().CommandContext(ctx, "cbfstool", mergedAPFile, "extract", "-r", "FW_MAIN_A", "-n", "ecrw", "-f", fmt.Sprintf("%s/ecrw.raw", tmpDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to cbfstool extract: %+v", err)
		}
	}
	err = h.DUT.Conn().CommandContext(ctx, "futility", "load_fmap", fmt.Sprintf("%s/ec.bin", newTargetDir), fmt.Sprintf("EC_RW:%s/ecrw.raw", tmpDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility load_fmap: %+v", err)
	}
	// Repack
	shellBallNew := fmt.Sprintf("%s/chromeos-firmwareupdate-new", tmpDir)
	err = h.DUT.Conn().CommandContext(ctx, "cp", "/usr/sbin/chromeos-firmwareupdate", shellBallNew).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to copy chromeos-firmwareupdate: %+v", err)
	}
	err = h.DUT.Conn().CommandContext(ctx, shellBallNew, "--repack", newMergedDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to repack chromeos-firmwareupdate-new: %+v", err)
	}

	// Now repack chromeos-firmwareupdate-old
	s.Log("Repacking shellball with old FW")
	oldMergedDir := fmt.Sprintf("%s/old", tmpDir)
	err = h.DUT.Conn().CommandContext(ctx, "mkdir", oldMergedDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to mkdir a %q: %+v", oldMergedDir, err)
	}
	oldTargetDir := fmt.Sprintf("%s/%s", oldMergedDir, fwTargets.FirmwareManifestKey)
	err = h.DUT.Conn().CommandContext(ctx, "mkdir", oldTargetDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to mkdir a %q: %+v", oldTargetDir, err)
	}
	// Read current AP FW
	oldAPFile := fmt.Sprintf("%s/image-%s.bin", oldMergedDir, fwTargets.FirmwareManifestKey)
	err = h.DUT.Conn().CommandContext(ctx, "futility", "read", oldAPFile).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility read to %q: %+v", oldAPFile, err)
	}
	// Read current EC FW
	oldECFile := fmt.Sprintf("%s/%s/ec.bin", oldMergedDir, fwTargets.FirmwareManifestKey)
	err = h.DUT.Conn().CommandContext(ctx, "flashrom", "-p", "ec", "-r", oldECFile).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to flashrom read ec to %q: %+v", oldECFile, err)
	}
	// Repack
	shellBallOld := fmt.Sprintf("%s/chromeos-firmwareupdate-old", tmpDir)
	err = h.DUT.Conn().CommandContext(ctx, "cp", "/usr/sbin/chromeos-firmwareupdate", shellBallOld).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to copy chromeos-firmwareupdate: %+v", err)
	}
	err = h.DUT.Conn().CommandContext(ctx, shellBallOld, "--repack", oldMergedDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to repack chromeos-firmwareupdate-old: %+v", err)
	}

	// That was all setup, start testing now

	s.Log("Enabling hardware write protect")
	if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateOn); err != nil {
		s.Fatal("Failed to enable hardware write protect: ", err)
	}
	testing.ContextLog(ctx, "Rebooting the DUT")
	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AllowGBBForce); err != nil {
		s.Fatal("Failed to perform mode aware reboot: ", err)
	}

	getCrossystemParams := func(ctx context.Context) (result map[reporters.CrossystemParam]string, retErr error) {
		retErr = testing.Poll(ctx, func(ctx context.Context) error {
			result, err = h.Reporter.Crossystem(ctx, reporters.CrossystemParamRoFwid, reporters.CrossystemParamFwid, reporters.CrossystemParamMainfwAct,
				reporters.CrossystemParamMainfwType, reporters.CrossystemParamTpmFwVer, reporters.CrossystemParamFWResult)
			if err != nil {
				return err
			}
			if result[reporters.CrossystemParamFWResult] == "trying" {
				return errors.New("firmware not ready, fw_result = trying")
			}
			return nil
		}, &testing.PollOptions{Timeout: 90 * time.Second, Interval: 5 * time.Second})
		return
	}
	initialVersions, err := getCrossystemParams(ctx)
	if err != nil {
		s.Fatalf("Failed to call crossystem: %+v", err)
	}
	ectool := firmware.NewECTool(h.DUT, firmware.ECToolNameMain)
	initialECRO, initialECRW, err := ectool.RORWVersion(ctx)
	if err != nil {
		s.Fatal("Failed to read ectool version: ", err)
	}
	s.Logf("Before autoupdate: %+v ECRO:%s ECRW:%s", initialVersions, initialECRO, initialECRW)
	if initialVersions[reporters.CrossystemParamMainfwType] != "normal" {
		s.Errorf("Expected to be in normal mode, got %q", initialVersions[reporters.CrossystemParamMainfwType])
	}

	out, err = h.DUT.Conn().CommandContext(ctx, shellBallNew, "--manifest").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to chromeos-firmwareupdate-new --mode=manifest: %+v", err)
	}
	expectedVersions := versionJSON{}
	err = yaml.Unmarshal(out, &expectedVersions)
	if err != nil {
		s.Fatalf("Failed to parse futility manifest: %q %+v", out, err)
	}
	s.Logf("Autoupdate expected versions: %+v", expectedVersions)

	out, err = h.DUT.Conn().CommandContext(ctx, shellBallNew, "--mode=autoupdate").CombinedOutput(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to chromeos-firmwareupdate-new --mode=autoupdate: %+v", err)
	}
	s.Logf("%s --mode=autoupdate: %s", shellBallNew, string(out))
	testing.ContextLog(ctx, "Rebooting the DUT")
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AllowGBBForce); err != nil {
		s.Fatal("Failed to perform mode aware reboot: ", err)
	}

	updatedVersions, err := getCrossystemParams(ctx)
	if err != nil {
		s.Fatalf("Failed to call crossystem: %+v", err)
	}
	updatedECRO, updatedECRW, err := ectool.RORWVersion(ctx)
	if err != nil {
		s.Fatal("Failed to read ectool version: ", err)
	}
	s.Logf("After autoupdate: %+v ECRO:%s ECRW:%s", updatedVersions, updatedECRO, updatedECRW)

	// Assertions
	if updatedVersions[reporters.CrossystemParamMainfwType] != "normal" {
		s.Errorf("Expected to be in normal mode, got %q", updatedVersions[reporters.CrossystemParamMainfwType])
	}
	if initialVersions[reporters.CrossystemParamRoFwid] != updatedVersions[reporters.CrossystemParamRoFwid] {
		s.Errorf("Expected AP RO unchanged, got %q want %q", updatedVersions[reporters.CrossystemParamRoFwid], initialVersions[reporters.CrossystemParamRoFwid])
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW != updatedVersions[reporters.CrossystemParamFwid] {
		s.Errorf("Expected AP RW updated, got %q want %q", updatedVersions[reporters.CrossystemParamFwid], expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW)
	}
	if initialECRO != updatedECRO {
		s.Errorf("Expected EC RO unchanged, got %q want %q", updatedECRO, initialECRO)
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW != updatedECRW {
		s.Errorf("Expected EC RW updated, got %q want %q", updatedECRW, expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW)
	}

	out, err = h.DUT.Conn().CommandContext(ctx, shellBallOld, "--manifest").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to chromeos-firmwareupdate-old --mode=manifest: %+v", err)
	}
	expectedVersions = versionJSON{}
	err = yaml.Unmarshal(out, &expectedVersions)
	if err != nil {
		s.Fatalf("Failed to parse futility manifest: %q %+v", out, err)
	}
	s.Logf("Rollback expected versions: %+v", expectedVersions)

	out, err = h.DUT.Conn().CommandContext(ctx, shellBallOld, "--mode=autoupdate").CombinedOutput(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to chromeos-firmwareupdate-old --mode=autoupdate: %+v", err)
	}
	s.Logf("%s --mode=autoupdate: %s", shellBallOld, string(out))
	testing.ContextLog(ctx, "Rebooting the DUT")
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AllowGBBForce); err != nil {
		s.Fatal("Failed to perform mode aware reboot: ", err)
	}

	rollbackVersions, err := getCrossystemParams(ctx)
	if err != nil {
		s.Fatalf("Failed to call crossystem: %+v", err)
	}
	rollbackECRO, rollbackECRW, err := ectool.RORWVersion(ctx)
	if err != nil {
		s.Fatal("Failed to read ectool version: ", err)
	}
	s.Logf("After rollback: %+v ECRO:%s ECRW:%s", rollbackVersions, rollbackECRO, rollbackECRW)

	// Assertions
	if rollbackVersions[reporters.CrossystemParamMainfwType] != "normal" {
		s.Errorf("Expected to be in normal mode, got %q", rollbackVersions[reporters.CrossystemParamMainfwType])
	}
	if initialVersions[reporters.CrossystemParamRoFwid] != rollbackVersions[reporters.CrossystemParamRoFwid] {
		s.Errorf("Expected AP RO unchanged, got %q want %q", rollbackVersions[reporters.CrossystemParamRoFwid], initialVersions[reporters.CrossystemParamRoFwid])
	}
	if initialECRO != rollbackECRO {
		s.Errorf("Expected EC RO unchanged, got %q want %q", rollbackECRO, initialECRO)
	}
	if initialVersions[reporters.CrossystemParamTpmFwVer] != updatedVersions[reporters.CrossystemParamTpmFwVer] {
		s.Log("Caution! New firmware sets anti-rollback version. Expect rollback to fail")
		if updatedVersions[reporters.CrossystemParamFwid] != rollbackVersions[reporters.CrossystemParamFwid] {
			s.Errorf("Expected AP RW unchanged, got %q want %q", rollbackVersions[reporters.CrossystemParamFwid], updatedVersions[reporters.CrossystemParamFwid])
		}
		if updatedECRW != rollbackECRW {
			s.Errorf("Expected EC RW unchanged, got %q want %q", rollbackECRW, updatedECRW)
		}
	} else {
		if expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW != rollbackVersions[reporters.CrossystemParamFwid] {
			s.Errorf("Expected AP RW reverted, got %q want %q", rollbackVersions[reporters.CrossystemParamFwid], expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW)
		}
		if initialVersions[reporters.CrossystemParamFwid] != rollbackVersions[reporters.CrossystemParamFwid] {
			s.Errorf("Expected AP RW reverted, got %q want %q", rollbackVersions[reporters.CrossystemParamFwid], initialVersions[reporters.CrossystemParamFwid])
		}
		if expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW != rollbackECRW {
			s.Errorf("Expected EC RW reverted, got %q want %q", rollbackECRW, expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW)
		}
		if initialECRW != rollbackECRW {
			s.Errorf("Expected EC RW reverted, got %q want %q", rollbackECRW, initialECRW)
		}
	}
}
