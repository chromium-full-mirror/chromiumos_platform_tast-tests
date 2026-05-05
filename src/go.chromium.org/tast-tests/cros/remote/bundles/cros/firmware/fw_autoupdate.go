// Copyright 2025 The ChromiumOS Authors
// Use of this source code is governed by a BSD-style license that can be
// found in the LICENSE file.

package firmware

import (
	"bytes"
	"context"
	"fmt"
	"os"
	"os/exec"
	"path"
	"strconv"
	"strings"
	"time"

	"github.com/golang/protobuf/ptypes/empty"
	"go.chromium.org/chromiumos/config/go/api"
	"go.chromium.org/tast-tests/cros/common/perf"
	"go.chromium.org/tast-tests/cros/common/servo"
	"go.chromium.org/tast-tests/cros/common/tbdep"
	"go.chromium.org/tast-tests/cros/remote/bundles/cros/firmware/utils"
	"go.chromium.org/tast-tests/cros/remote/firmware"
	"go.chromium.org/tast-tests/cros/remote/firmware/fixture"
	"go.chromium.org/tast-tests/cros/remote/firmware/reporters"
	fwpb "go.chromium.org/tast-tests/cros/services/cros/firmware"
	"go.chromium.org/tast/core/ctxutil"
	"go.chromium.org/tast/core/errors"
	"go.chromium.org/tast/core/ssh"
	"go.chromium.org/tast/core/ssh/linuxssh"
	"go.chromium.org/tast/core/testing"
	"go.chromium.org/tast/core/testing/hwdep"
	"gopkg.in/yaml.v2"
)

type testMode struct {
	Charging     bool
	WriteProtect bool
}

func init() {
	testing.AddTest(&testing.Test{
		Func: FWAutoupdate,
		Desc: "Verify autoupdate from and rollback to old firmware builds",
		Contacts: []string{
			"chromeos-faft@google.com",
			"jbettis@google.com",
		},
		BugComponent: "b:792402", // ChromeOS > Platform > Enablement > Firmware > FAFT
		// TODO(b/427195218): Add servo-exists + servo_state:WORKING after bug resolved.
		TestBedDeps:  []string{tbdep.ServoPresent},
		Vars:         []string{"firmware.apro", "firmware.aprw", "firmware.ecro", "firmware.ecrw"},
		Timeout:      2 * time.Hour,
		Fixture:      fixture.NormalMode,
		HardwareDeps: hwdep.D(),
		ServiceDeps:  []string{"tast.cros.firmware.UtilsService"},
		SoftwareDeps: []string{"crossystem", "chrome"},
		Params: []testing.Param{
			{
				Name: "ac_rw",
				Val: &testMode{
					Charging:     true,
					WriteProtect: true,
				},
			},
			{
				Name: "ac_ro",
				Val: &testMode{
					Charging:     true,
					WriteProtect: false,
				},
			},
			{
				Name: "battery_rw",
				Val: &testMode{
					Charging:     false,
					WriteProtect: true,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
			},
			{
				Name: "battery_ro",
				Val: &testMode{
					Charging:     false,
					WriteProtect: false,
				},
				ExtraHardwareDeps: hwdep.D(hwdep.Battery()),
			},
		},
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

const minChargePercent = 30

// The firmware update should take about 5 minutes.
const firmwareUpdateTimeout = 20 * time.Minute

// Other various commands get less time
const execTimeout = 2 * time.Minute

// FWAutoupdate expects the DUT to have the released RO/RW installed on the DUT as a precondition (--mode=recovery),
// then it will autoupdate to the version under test which is provided in the command line vars.
// Next it will attempt to downgrade back to the prior version.
// The command line flags accept any of:
// - a gs:// path to a directory of model tar files (gs://firmware-image-archive/firmware-rex-15709.B/15709.234.0/)
// - a gs:// path to a large board tar file (gs://chromeos-image-archive/firmware-rex-15709.B-branch/R122-15709.234.0-1-8717810714641823665/rex/firmware_from_source.tar.bz2)
// - a local path to a model tar file (karis.15709.234.0.tar.bz2 or FIXME.tbz2)
// - a local path to a large board tar file (15709.234.0.tar.bz2) WARNING the large board tar file from zephyr builds might pick the wrong ec binary.
// - a local path to a image.bin file.
func FWAutoupdate(ctx context.Context, s *testing.State) {
	h := s.FixtValue().(*fixture.Value).Helper
	if err := h.RequireConfig(ctx); err != nil {
		s.Fatal("Failed to get config: ", err)
	}

	if err := h.RequireServo(ctx); err != nil {
		s.Fatal("Failed to RequireServo: ", err)
	}

	if err := utils.EnableSoftwareSync(ctx, h); err != nil {
		s.Fatal("Software sync is required for this test: ", err)
	}

	hasBattery := true
	switch s.Features("").GetHardware().GetHardwareFeatures().GetFormFactor().GetFormFactor() {
	case api.HardwareFeatures_FormFactor_CHROMEBASE, api.HardwareFeatures_FormFactor_CHROMEBOX, api.HardwareFeatures_FormFactor_CHROMEBIT:
		hasBattery = false
	case api.HardwareFeatures_FormFactor_FORM_FACTOR_UNKNOWN:
		s.Fatal("Unknown formfactor")
	}
	if hasBattery {
		chargeTimeout := 30 * time.Minute
		testing.ContextLogf(ctx, "Wait for DUT to reach %d%% charged state up to %s minutes", minChargePercent, chargeTimeout)
		if err := testing.Poll(ctx, func(ctx context.Context) error {
			battery, err := firmware.GetECBatteryStatus(ctx, h)
			if err != nil {
				return errors.Wrap(err, "error getting battery status")
			}
			testing.ContextLogf(ctx, "Current charge: %v, status: %v", battery.Charge, battery.Status)
			if battery.Charge < minChargePercent {
				if err := firmware.PollToSetChargerStatus(ctx, h, true); err != nil {
					return errors.Wrap(err, "error connecting charger")
				}
				return errors.New("battery level too low")
			}
			return nil
		}, &testing.PollOptions{Timeout: chargeTimeout, Interval: time.Minute}); err != nil {
			s.Fatal("Failed to poll for battery level: ", err)
		}
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

	tryUntarFiles := func(ctx context.Context, archivePath, outputDir string, candidateFilenames, extraFilenames []string) (string, error) {
		// Run a single tar command to extract all the files, and then see which ones got extracted.
		// Running tar multiple times is very very slow.
		args := append([]string{
			"-x", "-j", "-f", archivePath, "-C", outputDir,
		}, candidateFilenames...)
		args = append(args, extraFilenames...)
		testing.ContextLog(ctx, "Trying to extract files from local archive: tar ", args)
		shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		out, tarErr := exec.CommandContext(shortCtx, "tar", args...).CombinedOutput()
		for _, filename := range candidateFilenames {
			binFile := path.Join(outputDir, filename)
			if _, err = os.Stat(binFile); err == nil {
				testing.ContextLog(ctx, "Found ", filename)
				return binFile, nil
			}
		}
		return "", errors.Wrapf(tarErr, "none of %v found in %q: %s", candidateFilenames, archivePath, string(out))
	}

	extractAPFirmwareImage := func(ctx context.Context, artifactUrl, destFile string) error {
		if strings.HasPrefix(artifactUrl, "gs://") {
			_, err := h.RPCUtils.ExtractAPFirmwareImage(ctx, &fwpb.ExtractFirmwareImageRequest{
				PreferredDevserver: s.CloudStorage().Devservers(),
				BackupDevserver:    redirectedDevServers,
				Dest:               destFile,
				Url:                artifactUrl,
				BuildTargets:       fwTargets,
				Board:              h.Board,
				Model:              h.Model,
			})
			if err != nil {
				return errors.Wrap(err, "RPCUtils.ExtractAPFirmwareImage failed")
			}
			return nil
		}
		if _, err := os.Stat(artifactUrl); err != nil {
			return errors.Wrapf(err, "failed to stat %q", artifactUrl)
		}
		if strings.HasSuffix(artifactUrl, ".tar.bz2") || strings.HasSuffix(artifactUrl, ".tbz2") {
			tempDir, err := os.MkdirTemp("", "firmware-extract-*")
			if err != nil {
				return errors.Wrap(err, "failed to create tmp dir")
			}
			defer os.RemoveAll(tempDir)

			var filenames []string
			if fwTargets.CorebootName != "" {
				filenames = append(filenames, fmt.Sprintf("image-%v.bin", fwTargets.CorebootName))
			}
			if h.Model != "" {
				filenames = append(filenames, fmt.Sprintf("image-%v.bin", h.Model))
			}
			if h.Board != "" {
				filenames = append(filenames, fmt.Sprintf("image-%v.bin", h.Board))
			}
			filenames = append(filenames, "image.bin")
			filenames = append(filenames, "bios.bin")
			binFile, err := tryUntarFiles(ctx, artifactUrl, tempDir, filenames, nil)
			if err != nil {
				return errors.Wrap(err, "tryUntarFiles failed")
			}
			artifactUrl = binFile
			// Continue on with handling of .bin files
		}
		_, err = linuxssh.PutFiles(ctx, h.DUT.Conn(), map[string]string{
			artifactUrl: destFile,
		}, linuxssh.DereferenceSymlinks)
		if err != nil {
			return errors.Wrap(err, "copy file to dut failed")
		}
		return nil
	}
	extractECFirmwareImage := func(ctx context.Context, artifactUrl, destFile string) error {
		if strings.HasPrefix(artifactUrl, "gs://") {
			_, err := h.RPCUtils.ExtractECFirmwareImage(ctx, &fwpb.ExtractFirmwareImageRequest{
				PreferredDevserver: s.CloudStorage().Devservers(),
				BackupDevserver:    redirectedDevServers,
				Dest:               destFile,
				Url:                artifactUrl,
				BuildTargets:       fwTargets,
				Board:              h.Board,
				Model:              h.Model,
			})
			if err != nil {
				return errors.Wrap(err, "RPCUtils.ExtractECFirmwareImage failed")
			}
			return nil
		}
		if _, err := os.Stat(artifactUrl); err != nil {
			return errors.Wrapf(err, "failed to stat %q", artifactUrl)
		}
		if strings.HasSuffix(artifactUrl, ".tar.bz2") || strings.HasSuffix(artifactUrl, ".tbz2") {
			tempDir, err := os.MkdirTemp("", "firmware-extract-*")
			if err != nil {
				return errors.Wrap(err, "failed to create tmp dir")
			}
			defer os.RemoveAll(tempDir)

			var filenames []string
			var extraFilenames []string
			if fwTargets.LegacyEcName != "" {
				filenames = append(filenames, path.Join(fwTargets.LegacyEcName, "ec.bin"))
				extraFilenames = append(extraFilenames, path.Join(fwTargets.LegacyEcName, "ec.config"))
			}
			if fwTargets.StandaloneEcName != "" {
				filenames = append(filenames, path.Join(fwTargets.StandaloneEcName, "ec.bin"))
				extraFilenames = append(extraFilenames, path.Join(fwTargets.StandaloneEcName, "ec.config"))
			}
			if h.Model != "" {
				filenames = append(filenames, path.Join(h.Model, "ec.bin"))
				extraFilenames = append(extraFilenames, path.Join(h.Model, "ec.config"))
			}
			if h.Board != "" {
				filenames = append(filenames, path.Join(h.Board, "ec.bin"))
				extraFilenames = append(extraFilenames, path.Join(h.Board, "ec.config"))
			}
			filenames = append(filenames, "ec.bin")
			extraFilenames = append(extraFilenames, "ec.config")
			binFile, err := tryUntarFiles(ctx, artifactUrl, tempDir, filenames, extraFilenames)
			if err != nil {
				return errors.Wrap(err, "tryUntarFiles failed")
			}
			artifactUrl = binFile
			// Continue on with handling of .bin files
		}
		files := map[string]string{
			artifactUrl: destFile,
		}
		ecConfigFile := strings.Replace(artifactUrl, ".bin", ".config", 1)
		if _, err := os.Stat(ecConfigFile); err == nil {
			files[ecConfigFile] = strings.Replace(destFile, ".bin", ".config", 1)
		}
		testing.ContextLog(ctx, "Copying files to dut: ", files)
		_, err = linuxssh.PutFiles(ctx, h.DUT.Conn(), files, linuxssh.DereferenceSymlinks)
		if err != nil {
			return errors.Wrap(err, "copy file to dut failed")
		}
		return nil
	}

	// Make a temp dir on the DUT
	shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
	defer cancel()
	tmpDirOut, err := h.DUT.Conn().CommandContext(shortCtx, "mktemp", "-d", "/usr/local/tmp/tast.firmware.FWAutoupdate.XXXXXXXXXX").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatal("Failed to create a temp dir: ", err)
	}
	tmpDir := strings.TrimSpace(string(tmpDirOut))

	defer func(ctx context.Context) {
		if err := h.EnsureDUTBooted(ctx); err != nil {
			s.Error("Failed to boot dut: ", err)
		}
		shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err := h.DUT.Conn().CommandContext(shortCtx, "rm", "-rf", tmpDir).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Error("Failed to delete temp dir: ", err)
		}
	}(cleanupContext)

	// Download all the necessary images
	apROFile := fmt.Sprintf("%s/apro.bin", tmpDir)
	err = extractAPFirmwareImage(ctx, apROURL, apROFile)
	if err != nil {
		s.Fatalf("Failed to extract AP RO from %q: %+v", apROURL, err)
	}
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err := h.DUT.Conn().CommandContext(shortCtx, "futility", "update", "--manifest", "--image", apROFile).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility update --manifest --image %q: %+v", apROFile, err)
	}
	expectedVersions := versionJSON{}
	err = yaml.Unmarshal(out, &expectedVersions)
	if err != nil {
		s.Fatalf("Failed to parse futility manifest: %q %+v", out, err)
	}
	expectedAPROVersion := expectedVersions["default"].AP.Versions.RO
	expectedAPRWVersion := expectedVersions["default"].AP.Versions.RW
	expectedECRWVersion := expectedVersions["default"].AP.Versions.ECRW
	s.Logf("Extracted AP RO: %q (%s)", apROFile, expectedAPROVersion)
	apRWFile := ""
	if apROURL != apRWURL {
		apRWFile = fmt.Sprintf("%s/aprw.bin", tmpDir)
		err = extractAPFirmwareImage(ctx, apRWURL, apRWFile)
		if err != nil {
			s.Fatalf("Failed to extract AP RW from %q: %+v", apRWURL, err)
		}
		shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		out, err = h.DUT.Conn().CommandContext(shortCtx, "futility", "update", "--manifest", "--image", apRWFile).Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to futility update --manifest --image %q: %+v", apRWFile, err)
		}
		expectedVersions := versionJSON{}
		err = yaml.Unmarshal(out, &expectedVersions)
		if err != nil {
			s.Fatalf("Failed to parse futility manifest: %q %+v", out, err)
		}
		expectedAPRWVersion = expectedVersions["default"].AP.Versions.RW
		expectedECRWVersion = expectedVersions["default"].AP.Versions.ECRW
	}
	s.Logf("Extracted AP RW: %q (%s)", apRWFile, expectedAPRWVersion)

	ecROFile := fmt.Sprintf("%s/ecro.bin", tmpDir)
	err = extractECFirmwareImage(ctx, ecROURL, ecROFile)
	if err != nil {
		s.Fatalf("Failed to extract EC RO from %q: %+v", ecROURL, err)
	}
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err = h.DUT.Conn().CommandContext(shortCtx, "futility", "update", "--manifest", "--ec_image", ecROFile).Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility update --manifest --ec_image %q: %+v", ecROFile, err)
	}
	expectedVersions = versionJSON{}
	err = yaml.Unmarshal(out, &expectedVersions)
	if err != nil {
		s.Fatalf("Failed to parse futility manifest: %q %+v", out, err)
	}
	expectedECROVersion := expectedVersions["default"].EC.Versions.RO
	s.Logf("Extracted EC RO: %q (%s)", ecROFile, expectedECROVersion)

	ecRWFile := ""
	if ecRWURL != apRWURL {
		ecRWFile = fmt.Sprintf("%s/ecrw.bin", tmpDir)
		if ecROURL == ecRWURL {
			ecRWFile = ecROFile
		} else {
			err = extractECFirmwareImage(ctx, ecRWURL, ecRWFile)
			if err != nil {
				s.Fatalf("Failed to extract EC RW from %q: %+v", ecRWURL, err)
			}
		}
		shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		out, err = h.DUT.Conn().CommandContext(shortCtx, "futility", "update", "--manifest", "--ec_image", ecRWFile).Output(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to futility update --manifest --ec_image %q: %+v", ecRWFile, err)
		}
		expectedVersions := versionJSON{}
		err = yaml.Unmarshal(out, &expectedVersions)
		if err != nil {
			s.Fatalf("Failed to parse futility manifest: %q %+v", out, err)
		}
		expectedECRWVersion = expectedVersions["default"].EC.Versions.RW
	}
	s.Logf("Extracted EC RW: %q (%s)", ecRWFile, expectedECRWVersion)

	// Much of this logic is copied from src/platform/firmware/pack_firmware.py

	s.Log("Repacking shellball with new FW")
	// Create directory structure expected by chromeos-firmwareupdate --repack
	newMergedDir := fmt.Sprintf("%s/new", tmpDir)
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "mkdir", newMergedDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to mkdir a %q: %+v", newMergedDir, err)
	}
	newTargetDir := fmt.Sprintf("%s/%s", newMergedDir, fwTargets.FirmwareManifestKey)
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "mkdir", newTargetDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to mkdir a %q: %+v", newTargetDir, err)
	}
	// Copy RO files to newMergedDir
	mergedAPFile := fmt.Sprintf("%s/image-%s.bin", newMergedDir, fwTargets.FirmwareManifestKey)
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "cp", apROFile, mergedAPFile).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to cp apro to %q: %+v", newMergedDir, err)
	}
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "cp", ecROFile, fmt.Sprintf("%s/ec.bin", newTargetDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to cp ecro to %q: %+v", newTargetDir, err)
	}
	// Pack AP-RW into merged.bin (RW_LEGACY & RW_MISC are optional)
	if apRWFile != "" {
		shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err := h.DUT.Conn().CommandContext(shortCtx, "futility", "dump_fmap", apRWFile, "-x", fmt.Sprintf("RW_SECTION_A:%s/a.bin", tmpDir), "-x", fmt.Sprintf("RW_SECTION_B:%s/b.bin", tmpDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to futility dump_fmap: %+v", err)
		}
		shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err = h.DUT.Conn().CommandContext(shortCtx, "futility", "load_fmap", mergedAPFile, fmt.Sprintf("RW_SECTION_A:%s/a.bin", tmpDir), fmt.Sprintf("RW_SECTION_B:%s/b.bin", tmpDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to futility load_fmap: %+v", err)
		}
		// RW_LEGACY is optional
		shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err = h.DUT.Conn().CommandContext(shortCtx, "futility", "dump_fmap", apRWFile, "-x", fmt.Sprintf("RW_LEGACY:%s/legacy.bin", tmpDir)).Run(ssh.DumpLogOnError)
		if err == nil {
			shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
			defer cancel()
			err := h.DUT.Conn().CommandContext(shortCtx, "futility", "load_fmap", mergedAPFile, fmt.Sprintf("RW_LEGACY:%s/legacy.bin", tmpDir)).Run(ssh.DumpLogOnError)
			if err != nil {
				s.Fatalf("Failed to futility load_fmap: %+v", err)
			}
		}
		// RW_MISC is optional
		shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err = h.DUT.Conn().CommandContext(shortCtx, "futility", "dump_fmap", apRWFile, "-x", fmt.Sprintf("RW_MISC:%s/misc.bin", tmpDir)).Run(ssh.DumpLogOnError)
		if err == nil {
			shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
			defer cancel()
			err := h.DUT.Conn().CommandContext(shortCtx, "futility", "load_fmap", mergedAPFile, fmt.Sprintf("RW_MISC:%s/misc.bin", tmpDir)).Run(ssh.DumpLogOnError)
			if err != nil {
				s.Fatalf("Failed to futility load_fmap: %+v", err)
			}
		}
	}
	// Pack EC-RW into merged.bin
	if ecRWFile != "" {
		args := []string{"-i", mergedAPFile, "-e", ecRWFile}
		configFile := strings.Replace(ecRWFile, ".bin", ".config", 1)
		shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err := h.DUT.Conn().CommandContext(shortCtx, "test", "-f", configFile).Run(ssh.DumpLogOnError)
		if err != nil {
			args = append(args, "--ec_config", configFile)
		}
		shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err = h.DUT.Conn().CommandContext(shortCtx, "/usr/share/vboot/bin/swap_ec_rw", args...).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to swap_ec_rw: %+v", err)
		}
	}
	// Pack EC-RW into ec.bin
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err = h.DUT.Conn().CommandContext(shortCtx, "futility", "dump_fmap", ecROFile, "-p").Output(ssh.DumpLogOnError)
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
		shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err := h.DUT.Conn().CommandContext(shortCtx, "futility", "dump_fmap", ecRWFile, "-x", fmt.Sprintf("EC_RW:%s/ecrw.raw", tmpDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to futility dump_fmap: %+v", err)
		}
	} else {
		shortCtx, cancel := context.WithTimeout(ctx, execTimeout)
		defer cancel()
		err := h.DUT.Conn().CommandContext(shortCtx, "cbfstool", mergedAPFile, "extract", "-r", "FW_MAIN_A", "-n", "ecrw", "-f", fmt.Sprintf("%s/ecrw.raw", tmpDir)).Run(ssh.DumpLogOnError)
		if err != nil {
			s.Fatalf("Failed to cbfstool extract: %+v", err)
		}
	}
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "futility", "load_fmap", fmt.Sprintf("%s/ec.bin", newTargetDir), fmt.Sprintf("EC_RW:%s/ecrw.raw", tmpDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility load_fmap: %+v", err)
	}
	// Repack
	shellBallNew := fmt.Sprintf("%s/chromeos-firmwareupdate-new", tmpDir)
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "cp", "/usr/sbin/chromeos-firmwareupdate", shellBallNew).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to copy chromeos-firmwareupdate: %+v", err)
	}
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, shellBallNew, "--repack", newMergedDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to repack chromeos-firmwareupdate-new: %+v", err)
	}

	// Now repack chromeos-firmwareupdate-old
	s.Log("Repacking shellball with old FW")
	oldMergedDir := fmt.Sprintf("%s/old", tmpDir)
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "mkdir", oldMergedDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to mkdir a %q: %+v", oldMergedDir, err)
	}
	oldTargetDir := fmt.Sprintf("%s/%s", oldMergedDir, fwTargets.FirmwareManifestKey)
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "mkdir", oldTargetDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to mkdir a %q: %+v", oldTargetDir, err)
	}
	// Read current AP FW
	oldAPFile := fmt.Sprintf("%s/image-%s.bin", oldMergedDir, fwTargets.FirmwareManifestKey)
	shortCtx, cancel = context.WithTimeout(ctx, firmwareUpdateTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "futility", "read", oldAPFile).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility read to %q: %+v", oldAPFile, err)
	}
	// Copy the active section to the backup section, released images will have the same firmware in A & B.
	activeFw, err := h.Reporter.CrossystemParam(ctx, reporters.CrossystemParamMainfwAct)
	inactiveFw := "B"
	if activeFw == "B" {
		inactiveFw = "A"
	}
	activeSection := "RW_SECTION_" + activeFw
	inactiveSection := "RW_SECTION_" + inactiveFw
	s.Logf("Copying section %s to %s for chromeos-firmwareupdate-old", activeSection, inactiveSection)
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "futility", "dump_fmap", oldAPFile, "-x", fmt.Sprintf("%s:%s/active_rw.bin", activeSection, tmpDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility dump_fmap: %+v", err)
	}
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "futility", "load_fmap", oldAPFile, fmt.Sprintf("%s:%s/active_rw.bin", inactiveSection, tmpDir)).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to futility load_fmap: %+v", err)
	}

	// Read current EC FW
	oldECFile := fmt.Sprintf("%s/%s/ec.bin", oldMergedDir, fwTargets.FirmwareManifestKey)
	shortCtx, cancel = context.WithTimeout(ctx, firmwareUpdateTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "flashrom", "-p", "ec", "-r", oldECFile).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to flashrom read ec to %q: %+v", oldECFile, err)
	}
	// Repack
	shellBallOld := fmt.Sprintf("%s/chromeos-firmwareupdate-old", tmpDir)
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, "cp", "/usr/sbin/chromeos-firmwareupdate", shellBallOld).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to copy chromeos-firmwareupdate: %+v", err)
	}
	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	err = h.DUT.Conn().CommandContext(shortCtx, shellBallOld, "--repack", oldMergedDir).Run(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to repack chromeos-firmwareupdate-old: %+v", err)
	}

	// That was all setup, start testing now
	if hasBattery {
		if err := firmware.PollToSetChargerStatus(ctx, h, s.Param().(*testMode).Charging); err != nil {
			s.Fatal("Error connecting charger: ", err)
		}
		defer func() {
			if err := firmware.PollToSetChargerStatus(cleanupContext, h, true); err != nil {
				s.Error("Failed to connect charger: ", err)
			}
		}()
	}

	ms, err := firmware.NewModeSwitcher(ctx, h)
	if err != nil {
		s.Fatal("Creating mode switcher: ", err)
	}
	if s.Param().(*testMode).WriteProtect {
		s.Log("Enabling write protect")
		shortCtx, cancel := context.WithTimeout(ctx, firmwareUpdateTimeout)
		defer cancel()
		if err := h.DUT.Conn().CommandContext(shortCtx, "futility", "flash", "--wp-enable").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to enable software write protect: ", err)
		}
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOn); err != nil {
			s.Fatal("Failed to enable hardware write protect: ", err)
		}
		if err := h.GSCResetAfterWPEnable(ctx, s.Features("")); err != nil {
			s.Fatal("Failed to reset GSC after write protect enable: ", err)
		}
		testing.ContextLog(ctx, "Rebooting the DUT")
		// The EC has to reboot to pick up the new WP state
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, firmware.AllowGBBForce); err != nil {
			s.Fatal("Failed to perform mode aware reboot: ", err)
		}
	} else {
		s.Log("Disabling write protect")
		if err := h.Servo.SetFWWPState(ctx, servo.FWWPStateForceOff); err != nil {
			s.Fatal("Failed to disable hardware write protect: ", err)
		}
		testing.ContextLog(ctx, "Rebooting the DUT")
		// The EC has to reboot to pick up the new WP state
		if err := ms.ModeAwareReboot(ctx, firmware.ColdReset, firmware.AllowGBBForce); err != nil {
			s.Fatal("Failed to perform mode aware reboot: ", err)
		}
		shortCtx, cancel := context.WithTimeout(ctx, firmwareUpdateTimeout)
		defer cancel()
		if err := h.DUT.Conn().CommandContext(shortCtx, "futility", "flash", "--wp-disable").Run(ssh.DumpLogOnError); err != nil {
			s.Fatal("Failed to enable software write protect: ", err)
		}
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
	s.Logf("Before autoupdate: APRO:%s APRW:%s ECRO:%s ECRW:%s", initialVersions[reporters.CrossystemParamRoFwid], initialVersions[reporters.CrossystemParamFwid], initialECRO, initialECRW)
	if initialVersions[reporters.CrossystemParamMainfwType] != "normal" {
		s.Errorf("Expected to be in normal mode, got %q", initialVersions[reporters.CrossystemParamMainfwType])
	}

	pv := perf.NewValues()
	defer func() {
		if err := pv.Save(s.OutDir()); err != nil {
			s.Error("Failed saving perf data: ", err)
		}
	}()
	baselineSpeedMetric, err := runSpeedTest(ctx, h)
	if err != nil {
		s.Error("Failed to get baseline speed metric: ", err)
	} else {
		pv.Set(perf.Metric{
			Name:      "baseline_speedometer_metric",
			Unit:      "None",
			Direction: perf.BiggerIsBetter,
			Multiple:  false,
		}, baselineSpeedMetric)
	}

	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err = h.DUT.Conn().CommandContext(shortCtx, shellBallNew, "--manifest").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to chromeos-firmwareupdate-new --manifest: %+v", err)
	}
	expectedVersions = versionJSON{}
	err = yaml.Unmarshal(out, &expectedVersions)
	if err != nil {
		s.Fatalf("Failed to parse futility manifest: %q %+v", out, err)
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RO != expectedAPROVersion {
		s.Errorf("Failed to pack chromeos-firmwareupdate-new AP RO got %s, want %s", expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RO, expectedAPROVersion)
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW != expectedAPRWVersion {
		s.Errorf("Failed to pack chromeos-firmwareupdate-new AP RW got %s, want %s", expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW, expectedAPRWVersion)
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RO != expectedECROVersion {
		s.Errorf("Failed to pack chromeos-firmwareupdate-new EC RO got %s, want %s", expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RO, expectedECROVersion)
	}
	if expectedECRWVersion != "" && expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW != expectedECRWVersion {
		s.Errorf("Failed to pack chromeos-firmwareupdate-new EC RW got %s, want %s", expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW, expectedECRWVersion)
	}

	s.Log("Updating firmware to new version")
	shortCtx, cancel = context.WithTimeout(ctx, firmwareUpdateTimeout)
	defer cancel()
	out, err = h.DUT.Conn().CommandContext(shortCtx, shellBallNew, "--mode=autoupdate").CombinedOutput(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to chromeos-firmwareupdate-new --mode=autoupdate: %+v", err)
	}
	s.Logf("%s --mode=autoupdate: %s", shellBallNew, string(out))
	testing.ContextLog(ctx, "Rebooting the DUT")
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AllowGBBForce, firmware.WaitSoftwareSync); err != nil {
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
	s.Logf("After autoupdate: APRO:%s APRW:%s ECRO:%s ECRW:%s", updatedVersions[reporters.CrossystemParamRoFwid], updatedVersions[reporters.CrossystemParamFwid], updatedECRO, updatedECRW)

	// Assertions
	if updatedVersions[reporters.CrossystemParamMainfwType] != "normal" {
		s.Errorf("Expected to be in normal mode, got %q", updatedVersions[reporters.CrossystemParamMainfwType])
	}
	if s.Param().(*testMode).WriteProtect {
		if initialVersions[reporters.CrossystemParamRoFwid] != updatedVersions[reporters.CrossystemParamRoFwid] {
			s.Errorf("Expected AP RO unchanged, got %q want %q", updatedVersions[reporters.CrossystemParamRoFwid], initialVersions[reporters.CrossystemParamRoFwid])
		}
		if initialECRO != updatedECRO {
			s.Errorf("Expected EC RO unchanged, got %q want %q", updatedECRO, initialECRO)
		}
	} else {
		if expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RO != updatedVersions[reporters.CrossystemParamRoFwid] {
			s.Errorf("Expected AP RO updated, got %q want %q", updatedVersions[reporters.CrossystemParamRoFwid], expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RO)
		}
		if expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RO != updatedECRO {
			s.Errorf("Expected EC RO updated, got %q want %q", updatedECRO, expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RO)
		}
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW != updatedVersions[reporters.CrossystemParamFwid] {
		s.Errorf("Expected AP RW updated, got %q want %q", updatedVersions[reporters.CrossystemParamFwid], expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW)
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW != updatedECRW {
		s.Errorf("Expected EC RW updated, got %q want %q", updatedECRW, expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW)
	}

	speedMetric, err := runSpeedTest(ctx, h)
	if err != nil {
		s.Error("Failed to get speed metric: ", err)
	} else {
		pv.Set(perf.Metric{
			Name:      "speedometer_metric",
			Unit:      "None",
			Direction: perf.BiggerIsBetter,
			Multiple:  false,
		}, speedMetric)
	}
	if speedMetric < baselineSpeedMetric*0.95 {
		s.Errorf("Speedometer metric has degraded by >5%% - Latest: %f Baseline: %f", speedMetric, baselineSpeedMetric)
	} else {
		s.Logf("Speedometer metric is acceptable - Latest: %f Baseline: %f", speedMetric, baselineSpeedMetric)
	}

	shortCtx, cancel = context.WithTimeout(ctx, execTimeout)
	defer cancel()
	out, err = h.DUT.Conn().CommandContext(shortCtx, shellBallOld, "--manifest").Output(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to chromeos-firmwareupdate-old --manifest: %+v", err)
	}
	expectedVersions = versionJSON{}
	err = yaml.Unmarshal(out, &expectedVersions)
	if err != nil {
		s.Fatalf("Failed to parse futility manifest: %q %+v", out, err)
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RO != initialVersions[reporters.CrossystemParamRoFwid] {
		s.Errorf("Failed to pack chromeos-firmwareupdate-old AP RO got %s, want %s", expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RO, initialVersions[reporters.CrossystemParamRoFwid])
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW != initialVersions[reporters.CrossystemParamFwid] {
		s.Errorf("Failed to pack chromeos-firmwareupdate-old AP RW got %s, want %s", expectedVersions[fwTargets.FirmwareManifestKey].AP.Versions.RW, initialVersions[reporters.CrossystemParamFwid])
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RO != initialECRO {
		s.Errorf("Failed to pack chromeos-firmwareupdate-old EC RO got %s, want %s", expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RO, initialECRO)
	}
	if expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW != initialECRW {
		s.Errorf("Failed to pack chromeos-firmwareupdate-old EC RW got %s, want %s", expectedVersions[fwTargets.FirmwareManifestKey].EC.Versions.RW, initialECRW)
	}

	s.Log("Rolling back firmware to old version")
	shortCtx, cancel = context.WithTimeout(ctx, firmwareUpdateTimeout)
	defer cancel()
	out, err = h.DUT.Conn().CommandContext(shortCtx, shellBallOld, "--mode=autoupdate").CombinedOutput(ssh.DumpLogOnError)
	if err != nil {
		s.Fatalf("Failed to chromeos-firmwareupdate-old --mode=autoupdate: %+v", err)
	}
	s.Logf("%s --mode=autoupdate: %s", shellBallOld, string(out))
	testing.ContextLog(ctx, "Rebooting the DUT")
	if err := ms.ModeAwareReboot(ctx, firmware.WarmReset, firmware.AllowGBBForce, firmware.WaitSoftwareSync); err != nil {
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
	s.Logf("After rollback: APRO:%s APRW:%s ECRO:%s ECRW:%s", rollbackVersions[reporters.CrossystemParamRoFwid], rollbackVersions[reporters.CrossystemParamFwid], rollbackECRO, rollbackECRW)

	// Assertions
	if rollbackVersions[reporters.CrossystemParamMainfwType] != "normal" {
		s.Errorf("Expected to be in normal mode, got %q", rollbackVersions[reporters.CrossystemParamMainfwType])
	}
	if initialVersions[reporters.CrossystemParamRoFwid] != rollbackVersions[reporters.CrossystemParamRoFwid] {
		s.Errorf("Expected AP RO at initial version, got %q want %q", rollbackVersions[reporters.CrossystemParamRoFwid], initialVersions[reporters.CrossystemParamRoFwid])
	}
	if initialECRO != rollbackECRO {
		s.Errorf("Expected EC RO at initial version, got %q want %q", rollbackECRO, initialECRO)
	}
	if initialVersions[reporters.CrossystemParamTpmFwVer] != updatedVersions[reporters.CrossystemParamTpmFwVer] {
		s.Logf("Caution! New firmware sets anti-rollback version (%s). Expect rollback to fail", updatedVersions[reporters.CrossystemParamTpmFwVer])
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

const (
	// speedometerTime sets the timeout for Speedometer test.
	maxSpeedometerTime = 10 * time.Minute
)

// runSpeedTest performs the speedometer2 test.
func runSpeedTest(ctx context.Context, h *firmware.Helper) (float64, error) {
	speedometerCtx, cancelSpeedometerCtx := context.WithTimeout(ctx, maxSpeedometerTime)
	defer cancelSpeedometerCtx()

	if err := h.RequireRPCClient(ctx); err != nil {
		return 0.0, errors.Wrap(err, "failed to start rpc client")
	}

	speedometerService := fwpb.NewUtilsServiceClient(h.RPCClient.Conn)
	if _, err := speedometerService.NewChrome(speedometerCtx, &empty.Empty{}); err != nil {
		return 0.0, errors.Wrap(err, "failed to initiate a chrome sesion")
	}
	defer func() error {
		if _, err := speedometerService.CloseChrome(speedometerCtx, &empty.Empty{}); err != nil {
			return errors.Wrap(err, "failed to close the chrome sesion")
		}
		return nil
	}()

	testing.ContextLog(speedometerCtx, "Sleep 120 seconds before running Speedometer")
	// GoBigSleepLint: Running speedometer right after boot gets weird results.
	testing.Sleep(speedometerCtx, 120*time.Second)

	testing.ContextLog(speedometerCtx, "Running speedometer test")
	sptest, err := speedometerService.PerformSpeedometerTest(speedometerCtx, &empty.Empty{})
	if err != nil {
		return 0.0, errors.Wrap(err, "failed while performing the Speedometer benchmark")
	}

	// Parse the output of the test as a float for later math operations.
	result, err := strconv.ParseFloat(sptest.Result, 64)
	if err != nil {
		return 0.0, errors.Wrap(err, "failed to convert the result into float")
	}
	testing.ContextLogf(speedometerCtx, "Speedometer Result: %f", result)
	return result, nil
}
